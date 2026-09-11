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

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/guide"
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
	Harness          *HarnessStore
}

// Server is the local Anthropic-compatible proxy.
type Server struct {
	cfg     Config
	engine  *routing.Engine
	http    *http.Server
	ln      net.Listener
	harness *HarnessStore
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
	h := cfg.Harness
	if h == nil {
		h = NewHarnessStore(false)
	}
	return &Server{cfg: cfg, engine: engine, harness: h}
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
	mux.HandleFunc("/debug/harness", s.handleHarness)
	mux.Handle("/", guide.Handler())
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
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": true, "provider": s.engine.Provider,
		"inspect_prompts": s.harness != nil && s.harness.Enabled(),
		"guide":           "/",
	})
}

func (s *Server) handleHarness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	enabled := s.harness != nil && s.harness.Enabled()
	items := []HarnessItem{}
	if enabled {
		items = s.harness.List()
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"enabled": enabled,
		"items":   items,
		"hint":    "Set [proxy] inspect_prompts = true (or CLAUDE_GATEWAY_INSPECT_PROMPTS=1) and restart to capture system prompts + tool lists from Claude Desktop.",
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	// Anthropic-compatible Models list for Claude Desktop on 3P discovery.
	// Desktop filters opaque IDs unless anthropic_family_tier is set, or the
	// ID looks like a Claude model. We advertise both the real OpenRouter ID
	// and Claude-looking aliases that routing maps to the same target.
	type caps struct {
		Supported bool `json:"supported"`
	}
	type modelObj struct {
		ID                  string          `json:"id"`
		Type                string          `json:"type"`
		DisplayName         string          `json:"display_name"`
		CreatedAt           string          `json:"created_at"`
		AnthropicFamilyTier string          `json:"anthropic_family_tier,omitempty"`
		IsFamilyDefault     bool            `json:"is_family_default,omitempty"`
		MaxInputTokens      int             `json:"max_input_tokens"`
		MaxTokens           int             `json:"max_tokens"`
		Capabilities        map[string]caps `json:"capabilities"`
	}
	baseCaps := map[string]caps{
		"batch": {}, "citations": {}, "code_execution": {},
		"context_management": {}, "effort": {}, "image_input": {},
		"pdf_input": {}, "structured_outputs": {}, "thinking": {},
	}
	baseCaps["structured_outputs"] = caps{Supported: false}

	var data []modelObj
	add := func(id, display, tier string, tools, vision, thinking bool) {
		c := map[string]caps{}
		for k, v := range baseCaps {
			c[k] = v
		}
		// tool use is implied by Messages API; image/thinking flagged explicitly
		_ = tools
		c["image_input"] = caps{Supported: vision}
		c["thinking"] = caps{Supported: thinking}
		data = append(data, modelObj{
			ID: id, Type: "model", DisplayName: display,
			CreatedAt:           "2026-01-01T00:00:00Z",
			AnthropicFamilyTier: tier, IsFamilyDefault: true,
			MaxInputTokens: 200000, MaxTokens: 8192, Capabilities: c,
		})
	}

	if s.engine != nil && s.engine.Registry != nil {
		picker := config.DesktopPickerEntries(s.engine.Registry.Models)
		if len(picker) > 0 {
			for _, e := range picker {
				name := e.DesktopLabel
				objTier := e.DesktopTier
				if objTier == "" {
					objTier = "sonnet"
				}
				c := map[string]caps{}
				for k, v := range baseCaps {
					c[k] = v
				}
				c["image_input"] = caps{Supported: e.Vision}
				c["thinking"] = caps{Supported: e.Reasoning}
				maxIn := e.ContextLimit
				if maxIn <= 0 {
					maxIn = 200000
				}
				data = append(data, modelObj{
					ID: e.DesktopID, Type: "model", DisplayName: name,
					CreatedAt:           "2026-01-01T00:00:00Z",
					AnthropicFamilyTier: objTier, IsFamilyDefault: e.IsDefault,
					MaxInputTokens: maxIn, MaxTokens: 8192, Capabilities: c,
				})
			}
		} else {
			for _, m := range s.engine.Registry.Models {
				if !m.Enabled {
					continue
				}
				tier := m.TierAlias
				switch tier {
				case "fast", "haiku":
					tier = "haiku"
				case "premium", "opus":
					tier = "opus"
				default:
					tier = "sonnet"
				}
				name := m.DisplayName
				if name == "" {
					name = m.ModelID
				}
				add(m.ModelID, name, tier, m.ToolCalls, m.Vision, m.Reasoning)
			}
		}
	}
	if len(data) == 0 {
		add("claude-sonnet-4", "Default Sonnet alias", "sonnet", true, false, false)
	}
	w.Header().Set("Content-Type", "application/json")
	first, last := data[0].ID, data[len(data)-1].ID
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": data, "first_id": first, "last_id": last, "has_more": false,
	})
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
	// Never log prompts/tools payloads to the process log.

	if req.Stream {
		s.stream(ctx, w, corr, req)
		return
	}
	resp, dec, err := s.engine.Send(ctx, req)
	s.harness.Record(req, dec.TargetModel)
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
	// Echo the client-visible model ID; upstream route is in X-Routed-Model.
	resp.Model = req.SourceModel
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
	s.harness.Record(req, dec.TargetModel)
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
	enc := anthropic.NewStreamEncoder(corr, req.SourceModel)
	for _, fr := range enc.Begin() {
		writeSSE(fr.Event, fr.Data)
	}

	var assembled strings.Builder
	var thinking strings.Builder
	var lastUsage api.Usage
	haveUsage := false
	for {
		select {
		case <-ctx.Done():
			for _, fr := range enc.Push(api.Event{Type: api.EventError, Error: &api.Error{Category: api.ErrRequestCancelled, Message: "cancelled"}}) {
				writeSSE(fr.Event, fr.Data)
			}
			if s.cfg.OnRequest != nil {
				resp := api.Response{
					ID: corr, Model: req.SourceModel, FinishReason: api.FinishCancelled,
					Content: streamAssembledContent(thinking.String(), assembled.String()),
					Error:   &api.Error{Category: api.ErrRequestCancelled, Message: "cancelled"},
				}
				if haveUsage {
					resp.Usage = lastUsage
				}
				s.cfg.OnRequest(req, resp)
			}
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if e.Type == api.EventTextDelta {
				assembled.WriteString(e.Text)
			}
			if e.Type == api.EventThinkingDelta {
				thinking.WriteString(e.Text)
			}
			if e.Type == api.EventUsage && e.Usage != nil {
				lastUsage = *e.Usage
				haveUsage = true
			}
			frames := enc.Push(e)
			for _, fr := range frames {
				writeSSE(fr.Event, fr.Data)
			}
			if e.Type == api.EventFinish {
				if s.cfg.OnRequest != nil {
					resp := api.Response{
						ID: corr, Model: req.SourceModel, FinishReason: e.FinishReason,
						Content: streamAssembledContent(thinking.String(), assembled.String()),
					}
					if haveUsage {
						resp.Usage = lastUsage
					}
					s.cfg.OnRequest(req, resp)
				}
				return
			}
			if e.Terminal() {
				if s.cfg.OnRequest != nil {
					resp := api.Response{
						ID: corr, Model: req.SourceModel, FinishReason: api.FinishError,
						Content: streamAssembledContent(thinking.String(), assembled.String()),
						Error:   e.Error,
					}
					if haveUsage {
						resp.Usage = lastUsage
					}
					s.cfg.OnRequest(req, resp)
				}
				return
			}
		}
	}
}

func streamAssembledContent(thinking, text string) []api.ContentBlock {
	var out []api.ContentBlock
	if thinking != "" {
		out = append(out, api.ContentBlock{Type: api.BlockThinking, Text: thinking})
	}
	if text != "" {
		out = append(out, api.ContentBlock{Type: api.BlockText, Text: text})
	}
	return out
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
