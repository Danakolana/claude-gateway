package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/danakolana/claude-gateway/internal/observability"
	"github.com/danakolana/claude-gateway/internal/protocol/inbound/anthropic"
	"github.com/danakolana/claude-gateway/internal/routing"
	"github.com/danakolana/claude-gateway/pkg/api"
)

// Config for the local proxy.
type Config struct {
	Addr             string // default 127.0.0.1:0
	MaxBodyBytes     int64
	RequestTimeout   time.Duration
	AllowNonLoopback bool
	Logger           *slog.Logger
	OnRequest        func(req api.Request, resp api.Response) // optional history hook
}

// Server is the local Anthropic-compatible proxy.
type Server struct {
	cfg    Config
	engine *routing.Engine
	http   *http.Server
	ln     net.Listener
}

func New(cfg Config, engine *routing.Engine) *Server {
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:0"
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 8 << 20
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 120 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Server{cfg: cfg, engine: engine}
}

// Start binds and serves. Non-loopback requires AllowNonLoopback and logs WARN.
func (s *Server) Start() (string, error) {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return "", err
	}
	addr := ln.Addr().String()
	host, _, _ := net.SplitHostPort(addr)
	ip := net.ParseIP(host)
	if ip != nil && !ip.IsLoopback() {
		if !s.cfg.AllowNonLoopback {
			_ = ln.Close()
			return "", fmt.Errorf("non-loopback binding requires explicit allow")
		}
		s.cfg.Logger.Warn("proxy binding to non-loopback address; ensure firewall rules restrict access", "addr", addr)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v1/messages", s.handleMessages)
	mux.HandleFunc("/v1/models", s.handleModels)
	s.http = &http.Server{Handler: s.limit(mux), ReadHeaderTimeout: 10 * time.Second}
	s.ln = ln
	go func() { _ = s.http.Serve(ln) }()
	return addr, nil
}

func (s *Server) limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// Addr returns the bound address.
func (s *Server) Addr() string {
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Shutdown stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.http == nil {
		return nil
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "provider": s.engine.Provider})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	// Claude Desktop on 3P calls GET /v1/models for discovery.
	// Opaque (non-Claude) IDs are ignored unless they carry anthropic_family_tier.
	type modelObj struct {
		ID                  string `json:"id"`
		DisplayName         string `json:"display_name,omitempty"`
		AnthropicFamilyTier string `json:"anthropic_family_tier,omitempty"`
		IsFamilyDefault     bool   `json:"is_family_default,omitempty"`
	}
	var data []modelObj
	if s.engine != nil && s.engine.Registry != nil {
		for _, m := range s.engine.Registry.Models {
			if !m.Enabled {
				continue
			}
			tier := m.TierAlias
			if tier == "" || tier == "true" || tier == "false" {
				tier = "sonnet"
			}
			// Map our tier aliases to Claude family tiers Desktop understands.
			switch tier {
			case "fast":
				tier = "haiku"
			case "balanced":
				tier = "sonnet"
			case "premium":
				tier = "opus"
			}
			name := m.DisplayName
			if name == "" {
				name = m.ModelID
			}
			data = append(data, modelObj{
				ID: m.ModelID, DisplayName: name,
				AnthropicFamilyTier: tier, IsFamilyDefault: true,
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "object": "list"})
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	corr := observability.CorrelationID(r.Header.Get("X-Request-ID"))
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, corr, &api.Error{Category: api.ErrInvalidConfig, Message: "read body: " + err.Error()})
		return
	}
	req, err := anthropic.DecodeRequest(body)
	if err != nil {
		if ae, ok := err.(*api.Error); ok {
			writeErr(w, corr, ae)
			return
		}
		writeErr(w, corr, &api.Error{Category: api.ErrProviderProtocol, Message: err.Error()})
		return
	}
	req.ID = corr
	s.cfg.Logger.Info("request", "correlation_id", corr, "model", req.SourceModel, "stream", req.Stream)
	// Never log prompts/tools payloads.

	if req.Stream {
		s.stream(ctx, w, corr, req)
		return
	}
	resp, dec, err := s.engine.Send(ctx, req)
	if err != nil {
		if ae, ok := err.(*api.Error); ok {
			writeErr(w, corr, ae)
			return
		}
		writeErr(w, corr, &api.Error{Category: api.ErrInternal, Message: err.Error()})
		return
	}
	if resp.ID == "" {
		resp.ID = corr
	}
	if resp.Model == "" {
		resp.Model = dec.TargetModel
	}
	if s.cfg.OnRequest != nil {
		s.cfg.OnRequest(req, resp)
	}
	raw, err := anthropic.EncodeResponse(resp)
	if err != nil {
		writeErr(w, corr, &api.Error{Category: api.ErrInternal, Message: err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Correlation-ID", corr)
	w.Header().Set("X-Routed-Model", dec.TargetModel)
	_, _ = w.Write(raw)
}

func (s *Server) stream(ctx context.Context, w http.ResponseWriter, corr string, req api.Request) {
	ch, dec, err := s.engine.Stream(ctx, req)
	if err != nil {
		if ae, ok := err.(*api.Error); ok {
			writeErr(w, corr, ae)
			return
		}
		writeErr(w, corr, &api.Error{Category: api.ErrInternal, Message: err.Error()})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, corr, &api.Error{Category: api.ErrInternal, Message: "streaming unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Correlation-ID", corr)
	w.Header().Set("X-Routed-Model", dec.TargetModel)
	w.WriteHeader(http.StatusOK)

	writeSSE := func(event string, payload []byte) {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
		flusher.Flush()
	}
	// message_start
	start, _ := json.Marshal(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": corr, "type": "message", "role": "assistant", "model": dec.TargetModel,
			"content": []any{}, "usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	})
	writeSSE("message_start", start)
	blockStart, _ := json.Marshal(map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
	writeSSE("content_block_start", blockStart)

	var assembled strings.Builder
	for {
		select {
		case <-ctx.Done():
			ev := api.Event{Type: api.EventError, Error: &api.Error{Category: api.ErrRequestCancelled, Message: "cancelled"}}
			name, payload, _ := anthropic.EncodeEvent(ev)
			writeSSE(name, payload)
			if s.cfg.OnRequest != nil {
				s.cfg.OnRequest(req, api.Response{
					ID: corr, Model: dec.TargetModel, FinishReason: api.FinishCancelled,
					Content: []api.ContentBlock{{Type: api.BlockText, Text: assembled.String()}},
					Error:   ev.Error,
				})
			}
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if e.Type == api.EventTextDelta {
				assembled.WriteString(e.Text)
			}
			name, payload, term := anthropic.EncodeEvent(e)
			writeSSE(name, payload)
			if e.Type == api.EventFinish {
				stop, _ := json.Marshal(map[string]any{"type": "message_stop"})
				writeSSE("message_stop", stop)
				if s.cfg.OnRequest != nil {
					s.cfg.OnRequest(req, api.Response{
						ID: corr, Model: dec.TargetModel, FinishReason: e.FinishReason,
						Content: []api.ContentBlock{{Type: api.BlockText, Text: assembled.String()}},
					})
				}
				return
			}
			if term {
				if s.cfg.OnRequest != nil {
					s.cfg.OnRequest(req, api.Response{
						ID: corr, Model: dec.TargetModel, FinishReason: api.FinishError,
						Content: []api.ContentBlock{{Type: api.BlockText, Text: assembled.String()}},
						Error:   e.Error,
					})
				}
				return
			}
		}
	}
}

func writeErr(w http.ResponseWriter, corr string, e *api.Error) {
	e.Correlation = corr
	status := http.StatusBadRequest
	switch e.Category {
	case api.ErrProviderAuth:
		status = http.StatusUnauthorized
	case api.ErrProviderRateLimit:
		status = 429
	case api.ErrInternal, api.ErrProviderTransient:
		status = http.StatusBadGateway
	case api.ErrUnsupportedCapability:
		status = http.StatusUnprocessableEntity
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Correlation-ID", corr)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":  "error",
		"error": map[string]any{"type": string(e.Category), "message": e.Message},
	})
}
