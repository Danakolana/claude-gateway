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
func DecodeStream(r io.Reader) <-chan api.Event {
	ch := make(chan api.Event)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		terminal := false
		emit := func(e api.Event) {
			if terminal {
				return
			}
			ch <- e
			if e.Terminal() {
				terminal = true
			}
		}
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				emit(api.Event{Type: api.EventFinish, FinishReason: api.FinishEndTurn})
				return
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content   string     `json:"content"`
						ToolCalls []ToolCall `json:"tool_calls"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
				Usage *struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				emit(api.Event{Type: api.EventError, Error: &api.Error{
					Category: api.ErrProviderProtocol, Message: "bad SSE JSON",
				}})
				return
			}
			if chunk.Usage != nil {
				emit(api.Event{Type: api.EventUsage, Usage: &api.Usage{
					InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens,
				}})
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			d := chunk.Choices[0].Delta
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
				emit(api.Event{Type: api.EventFinish, FinishReason: mapFinish(*chunk.Choices[0].FinishReason)})
				return
			}
		}
		if err := sc.Err(); err != nil {
			emit(api.Event{Type: api.EventError, Error: &api.Error{
				Category: api.ErrProviderTransient, Message: err.Error(), Retryable: true,
			}})
			return
		}
		if !terminal {
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
