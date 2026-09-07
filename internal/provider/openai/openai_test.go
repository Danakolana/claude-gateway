package openai_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danakolana/claude-gateway/internal/provider/openai"
	"github.com/danakolana/claude-gateway/pkg/api"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	req := api.Request{
		ID: "1", TargetModel: "m", System: "sys",
		Messages: []api.Message{{Role: api.RoleUser, Content: []api.ContentBlock{{Type: api.BlockText, Text: "hi"}}}},
		Tools:    []api.ToolDef{{Name: "search", InputSchema: map[string]any{"type": "object"}}},
	}
	b, err := openai.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["model"] != "m" {
		t.Fatalf("%v", raw)
	}

	respJSON := `{"id":"x","model":"m","choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`
	resp, err := openai.DecodeResponse([]byte(respJSON))
	if err != nil || resp.Content[0].Text != "hello" || resp.FinishReason != api.FinishEndTurn {
		t.Fatalf("%+v %v", resp, err)
	}
}

func TestRejectThinking(t *testing.T) {
	_, err := openai.EncodeRequest(api.Request{
		Messages: []api.Message{{Role: api.RoleAssistant, Content: []api.ContentBlock{{Type: api.BlockThinking, Text: "..."}}}},
	})
	if err == nil {
		t.Fatal("expected unsupported")
	}
}

func TestStreamTerminal(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"
	ch := openai.StreamFromString(sse)
	var events []api.Event
	for e := range ch {
		events = append(events, e)
	}
	if len(events) < 2 || !events[len(events)-1].Terminal() {
		t.Fatalf("%+v", events)
	}
	nTerm := 0
	for _, e := range events {
		if e.Terminal() {
			nTerm++
		}
	}
	if nTerm != 1 {
		t.Fatalf("want 1 terminal, got %d: %+v", nTerm, events)
	}
	_ = strings.Builder{}
}

func TestGoldenOpenAIToolStream(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "protocol", "stream", "openai_tool_calls.sse"))
	if err != nil {
		// from module root via go test ./...
		raw, err = os.ReadFile(filepath.Join("testdata", "protocol", "stream", "openai_tool_calls.sse"))
	}
	if err != nil {
		t.Fatal(err)
	}
	ch := openai.StreamFromString(string(raw))
	var toolDeltas int
	var finish api.FinishReason
	for e := range ch {
		if e.Type == api.EventToolCallDelta {
			toolDeltas++
			if e.ToolName == "get_weather" && e.ToolUseID == "call_abc" {
				// first chunk carries identity
			}
		}
		if e.Type == api.EventFinish {
			finish = e.FinishReason
		}
	}
	if toolDeltas < 2 {
		t.Fatalf("tool deltas=%d", toolDeltas)
	}
	if finish != api.FinishToolUse {
		t.Fatalf("finish=%q", finish)
	}
}
