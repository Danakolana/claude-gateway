package fake

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/danakolana/claude-gateway/internal/provider"
	"github.com/danakolana/claude-gateway/pkg/api"
)

// Adapter is a deterministic in-memory provider.
type Adapter struct {
	FailAuth        bool
	Slow            time.Duration
	DisconnectAfter int // stream events before error; 0 = full success
	FailIfTarget    string
	HealthFail      bool
	HealthDelay     time.Duration
	Calls           []string
}

func (a *Adapter) Name() string { return "fake" }

func (a *Adapter) Capabilities(context.Context, provider.Target) (api.Capabilities, error) {
	return api.Capabilities{
		Streaming: api.CapSupported,
		Tools:     api.CapSupported,
		Vision:    api.CapUnsupported,
		Reasoning: api.CapUnsupported,
		Context:   128000,
	}, nil
}

func (a *Adapter) Health(ctx context.Context) provider.HealthResult {
	if a.HealthDelay > 0 {
		select {
		case <-ctx.Done():
			return provider.HealthResult{OK: false, Message: ctx.Err().Error()}
		case <-time.After(a.HealthDelay):
		}
	}
	if a.HealthFail {
		return provider.HealthResult{OK: false, Message: "fake health down"}
	}
	return provider.HealthResult{OK: true, Message: "fake ok"}
}

func (a *Adapter) Send(ctx context.Context, req api.Request) (api.Response, error) {
	a.Calls = append(a.Calls, req.TargetModel)
	if a.FailIfTarget != "" && req.TargetModel == a.FailIfTarget {
		return api.Response{}, &api.Error{Category: api.ErrProviderTransient, Message: "fake target down", Retryable: true}
	}
	if a.FailAuth {
		return api.Response{Error: &api.Error{Category: api.ErrProviderAuth, Message: "fake auth"}}, nil
	}
	select {
	case <-ctx.Done():
		return api.Response{Error: &api.Error{Category: api.ErrRequestCancelled, Message: ctx.Err().Error()}}, nil
	case <-time.After(a.Slow):
	}
	text := "fake:" + lastUserText(req)
	if hasTools(req) {
		return api.Response{
			ID: req.ID, Model: req.TargetModel,
			Content: []api.ContentBlock{{
				Type: api.BlockToolUse, ToolUseID: "call_1", ToolName: req.Tools[0].Name,
				ToolInput: map[string]any{"q": text},
			}},
			FinishReason: api.FinishToolUse,
			Usage:        api.Usage{InputTokens: 3, OutputTokens: 5},
		}, nil
	}
	return api.Response{
		ID: req.ID, Model: req.TargetModel,
		Content:      []api.ContentBlock{{Type: api.BlockText, Text: text}},
		FinishReason: api.FinishEndTurn,
		Usage:        api.Usage{InputTokens: 3, OutputTokens: 5},
	}, nil
}

func (a *Adapter) Stream(ctx context.Context, req api.Request) (<-chan api.Event, error) {
	ch := make(chan api.Event)
	go func() {
		defer close(ch)
		text := "fake:" + lastUserText(req)
		if hasTools(req) {
			args := `{"q":` + jsonQuote(text) + `}`
			select {
			case <-ctx.Done():
				ch <- api.Event{Type: api.EventError, Error: &api.Error{Category: api.ErrRequestCancelled, Message: "cancelled"}}
				return
			case ch <- api.Event{
				Type: api.EventToolCallDelta, ToolUseID: "call_1", ToolName: req.Tools[0].Name,
				ToolIndex: 0, ToolInputJSON: "",
			}:
			}
			select {
			case <-ctx.Done():
				ch <- api.Event{Type: api.EventError, Error: &api.Error{Category: api.ErrRequestCancelled, Message: "cancelled"}}
				return
			case ch <- api.Event{Type: api.EventToolCallDelta, ToolIndex: 0, ToolInputJSON: args}:
			}
			ch <- api.Event{Type: api.EventFinish, FinishReason: api.FinishToolUse}
			return
		}
		parts := []string{text[:len(text)/2], text[len(text)/2:]}
		if parts[0] == "" {
			parts = []string{text}
		}
		n := 0
		for _, p := range parts {
			n++
			if a.DisconnectAfter > 0 && n > a.DisconnectAfter {
				ch <- api.Event{Type: api.EventError, Error: &api.Error{
					Category: api.ErrProviderTransient, Message: "fake disconnect", Retryable: true,
				}}
				return
			}
			select {
			case <-ctx.Done():
				ch <- api.Event{Type: api.EventError, Error: &api.Error{
					Category: api.ErrRequestCancelled, Message: "cancelled",
				}}
				return
			case ch <- api.Event{Type: api.EventTextDelta, Text: p}:
			}
		}
		ch <- api.Event{Type: api.EventUsage, Usage: &api.Usage{InputTokens: 3, OutputTokens: 5}}
		ch <- api.Event{Type: api.EventFinish, FinishReason: api.FinishEndTurn}
	}()
	return ch, nil
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func lastUserText(req api.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role != api.RoleUser {
			continue
		}
		var b strings.Builder
		for _, c := range req.Messages[i].Content {
			if c.Type == api.BlockText {
				b.WriteString(c.Text)
			}
		}
		if b.Len() > 0 {
			return b.String()
		}
	}
	return "empty"
}

func hasTools(req api.Request) bool { return len(req.Tools) > 0 }

var _ provider.Adapter = (*Adapter)(nil)
