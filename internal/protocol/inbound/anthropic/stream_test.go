package anthropic_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/danakolana/claude-gateway/internal/protocol/inbound/anthropic"
	"github.com/danakolana/claude-gateway/pkg/api"
)

func TestStreamEncoderThinking(t *testing.T) {
	enc := anthropic.NewStreamEncoder("msg_1", "m")
	var buf bytes.Buffer
	write := func(frames []anthropic.Frame) {
		for _, fr := range frames {
			buf.WriteString("event: " + fr.Event + "\ndata: " + string(fr.Data) + "\n\n")
		}
	}
	write(enc.Begin())
	write(enc.Push(api.Event{Type: api.EventThinkingDelta, Text: "plan"}))
	write(enc.Push(api.Event{Type: api.EventTextDelta, Text: "hi"}))
	write(enc.Push(api.Event{Type: api.EventUsage, Usage: &api.Usage{InputTokens: 10, OutputTokens: 4, CachedTokens: 2}}))
	write(enc.Push(api.Event{Type: api.EventFinish, FinishReason: api.FinishEndTurn}))
	out := buf.String()
	for _, want := range []string{
		`"type":"thinking"`,
		`"thinking_delta"`,
		`"type":"text"`,
		`"cache_read_input_tokens":2`,
		`"stop_reason":"end_turn"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestStreamEncoderToolUse(t *testing.T) {
	enc := anthropic.NewStreamEncoder("msg_1", "deepseek/x")
	var buf bytes.Buffer
	write := func(frames []anthropic.Frame) {
		for _, fr := range frames {
			buf.WriteString("event: " + fr.Event + "\ndata: " + string(fr.Data) + "\n\n")
		}
	}
	write(enc.Begin())
	write(enc.Push(api.Event{Type: api.EventTextDelta, Text: "hi "}))
	write(enc.Push(api.Event{
		Type: api.EventToolCallDelta, ToolUseID: "call_1", ToolName: "search",
		ToolIndex: 0, ToolInputJSON: "",
	}))
	write(enc.Push(api.Event{
		Type: api.EventToolCallDelta, ToolIndex: 0, ToolInputJSON: `{"q":"x"}`,
	}))
	write(enc.Push(api.Event{Type: api.EventFinish, FinishReason: api.FinishToolUse}))

	out := buf.String()
	for _, want := range []string{
		"event: message_start",
		`"type":"text"`,
		`"type":"tool_use"`,
		`"name":"search"`,
		`"partial_json":"{\"q\":\"x\"}"`,
		`"stop_reason":"tool_use"`,
		"event: content_block_stop",
		"event: message_stop",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	// No bare pings for tool deltas.
	if strings.Contains(out, `"type":"ping"`) {
		t.Fatalf("unexpected ping: %s", out)
	}
}

func TestToolResultContentArray(t *testing.T) {
	raw := []byte(`{
		"model":"claude-sonnet","max_tokens":32,
		"messages":[{"role":"user","content":[
			{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"result-ok"}]}
		]}]
	}`)
	req, err := anthropic.DecodeRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].Content) != 1 {
		t.Fatalf("%+v", req.Messages)
	}
	if req.Messages[0].Content[0].ToolContent != "result-ok" {
		t.Fatalf("%q", req.Messages[0].Content[0].ToolContent)
	}
}

func TestEncodeResponseEmptyToolInput(t *testing.T) {
	raw, err := anthropic.EncodeResponse(api.Response{
		ID: "1", Model: "m", FinishReason: api.FinishToolUse,
		Content: []api.ContentBlock{{
			Type: api.BlockToolUse, ToolUseID: "c1", ToolName: "t", ToolInput: nil,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var msg map[string]any
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}
	content := msg["content"].([]any)
	block := content[0].(map[string]any)
	input := block["input"].(map[string]any)
	if input == nil {
		t.Fatalf("%s", raw)
	}
}
