package openai

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/danakolana/claude-gateway/internal/provider"
	"github.com/danakolana/claude-gateway/pkg/api"
)

// Client is an OpenAI-compatible HTTP adapter.
type Client struct {
	NameStr    string
	BaseURL    string
	APIKey     string
	AuthScheme string // bearer | x-api-key
	Headers    map[string]string
	HTTP       *http.Client
	Caps       api.Capabilities
}

func NewClient(name, baseURL, apiKey, authScheme string, tlsVerify bool, headers map[string]string, timeout time.Duration) *Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if !tlsVerify {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12} //nolint:gosec
	} else {
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &Client{
		NameStr: name, BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey,
		AuthScheme: authScheme, Headers: headers,
		HTTP: &http.Client{Timeout: timeout, Transport: tr},
		Caps: api.Capabilities{
			Streaming: api.CapSupported, Tools: api.CapSupported,
			Vision: api.CapSupported, Reasoning: api.CapUnsupported, Context: 0,
		},
	}
}

func (c *Client) Name() string { return c.NameStr }

func (c *Client) Capabilities(context.Context, provider.Target) (api.Capabilities, error) {
	return c.Caps, nil
}

func (c *Client) Health(ctx context.Context) provider.HealthResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return provider.HealthResult{OK: false, Message: err.Error()}
	}
	c.applyAuth(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return provider.HealthResult{OK: false, Message: err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		return provider.HealthResult{OK: false, Message: fmt.Sprintf("status %d", res.StatusCode)}
	}
	return provider.HealthResult{OK: true, Message: "ok"}
}

func (c *Client) applyAuth(req *http.Request) {
	switch strings.ToLower(c.AuthScheme) {
	case "x-api-key":
		req.Header.Set("x-api-key", c.APIKey)
	default:
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Content-Type", "application/json")
}

func (c *Client) Send(ctx context.Context, req api.Request) (api.Response, error) {
	req.Stream = false
	body, err := EncodeRequest(req)
	if err != nil {
		return api.Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return api.Response{}, err
	}
	c.applyAuth(httpReq)
	res, err := c.HTTP.Do(httpReq)
	if err != nil {
		return api.Response{Error: &api.Error{Category: api.ErrProviderTransient, Message: err.Error(), Retryable: true}}, nil
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return api.Response{}, err
	}
	if res.StatusCode >= 400 {
		return api.Response{Error: mapHTTPError(res.StatusCode, string(raw))}, nil
	}
	return DecodeResponse(raw)
}

func (c *Client) Stream(ctx context.Context, req api.Request) (<-chan api.Event, error) {
	req.Stream = true
	body, err := EncodeRequest(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.applyAuth(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")
	res, err := c.HTTP.Do(httpReq)
	if err != nil {
		ch := make(chan api.Event, 1)
		ch <- api.Event{Type: api.EventError, Error: &api.Error{Category: api.ErrProviderTransient, Message: err.Error(), Retryable: true}}
		close(ch)
		return ch, nil
	}
	if res.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		_ = res.Body.Close()
		ch := make(chan api.Event, 1)
		ch <- api.Event{Type: api.EventError, Error: mapHTTPError(res.StatusCode, string(raw))}
		close(ch)
		return ch, nil
	}
	inner := DecodeStream(res.Body)
	out := make(chan api.Event)
	go func() {
		defer close(out)
		defer res.Body.Close()
		for {
			select {
			case <-ctx.Done():
				out <- api.Event{Type: api.EventError, Error: &api.Error{Category: api.ErrRequestCancelled, Message: "cancelled"}}
				return
			case e, ok := <-inner:
				if !ok {
					return
				}
				out <- e
				if e.Terminal() {
					// drain
					for range inner {
					}
					return
				}
			}
		}
	}()
	return out, nil
}

var _ provider.Adapter = (*Client)(nil)
