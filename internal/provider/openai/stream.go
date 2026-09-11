package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/danakolana/claude-gateway/pkg/api"
)

// DecodeStream reads OpenAI SSE and emits canonical events. Exactly one terminal.
// Usage may arrive in a later chunk after finish_reason (OpenRouter); we defer
// Finish until [DONE] / EOF so accounting is not dropped.
func DecodeStream(r io.Reader) <-chan api.Event {
	ch := make(chan api.Event)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		terminal := false
		var pendingFinish api.FinishReason
		havePending := false
		var lastUsage *api.Usage
		var backend string

		emit := func(e api.Event) {
			if terminal {
				return
			}
			ch <- e
			if e.Terminal() {
				terminal = true
			}
		}
		flushTerminal := func(fallback api.FinishReason) {
			if lastUsage != nil {
				emit(api.Event{Type: api.EventUsage, Usage: lastUsage})
			}
			reason := fallback
			if havePending {
				reason = pendingFinish
			}
			emit(api.Event{Type: api.EventFinish, FinishReason: reason})
		}

		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				flushTerminal(api.FinishEndTurn)
				return
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content          string     `json:"content"`
						Reasoning        string     `json:"reasoning"`
						ReasoningDetails []any      `json:"reasoning_details"`
						ToolCalls        []ToolCall `json:"tool_calls"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
				Provider string     `json:"provider"`
				Usage    *chatUsage `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				emit(api.Event{Type: api.EventError, Error: &api.Error{
					Category: api.ErrProviderProtocol, Message: "bad SSE JSON",
				}})
				return
			}
			if chunk.Provider != "" {
				backend = chunk.Provider
				if lastUsage != nil {
					lastUsage.UpstreamBackend = backend
				}
			}
			if chunk.Usage != nil {
				u := chunk.Usage.toAPI()
				if backend != "" {
					u.UpstreamBackend = backend
				}
				lastUsage = &u
			} else if backend != "" && lastUsage == nil {
				lastUsage = &api.Usage{UpstreamBackend: backend}
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			d := chunk.Choices[0].Delta
			if d.Reasoning != "" {
				emit(api.Event{Type: api.EventThinkingDelta, Text: d.Reasoning})
			}
			for _, raw := range d.ReasoningDetails {
				m, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				if tx, ok := m["text"].(string); ok && tx != "" {
					emit(api.Event{Type: api.EventThinkingDelta, Text: tx})
				}
			}
			if d.Content != "" {
				emit(api.Event{Type: api.EventTextDelta, Text: d.Content})
			}
			for _, tc := range d.ToolCalls {
				emit(api.Event{
					Type: api.EventToolCallDelta, ToolUseID: tc.ID, ToolName: tc.Function.Name,
					ToolInputJSON: tc.Function.Arguments, ToolIndex: tc.Index,
				})
			}
			if chunk.Choices[0].FinishReason != nil && *chunk.Choices[0].FinishReason != "" {
				pendingFinish = mapFinish(*chunk.Choices[0].FinishReason)
				havePending = true
				// Keep reading for a trailing usage chunk / [DONE].
				continue
			}
		}
		if err := sc.Err(); err != nil {
			emit(api.Event{Type: api.EventError, Error: &api.Error{
				Category: api.ErrProviderTransient, Message: err.Error(), Retryable: true,
			}})
			return
		}
		if !terminal {
			if havePending || lastUsage != nil {
				flushTerminal(api.FinishEndTurn)
				return
			}
			emit(api.Event{Type: api.EventError, Error: &api.Error{
				Category: api.ErrProviderTransient, Message: "stream ended without terminal event", Retryable: true,
			}})
		}
	}()
	return ch
}

// StreamFromString is a test helper.
func StreamFromString(s string) <-chan api.Event {
	return DecodeStream(bytes.NewBufferString(s))
}
