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
		ID: "1", TargetModel: "m", System: "sys", Stream: true,
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
	if so, ok := raw["stream_options"].(map[string]any); !ok || so["include_usage"] != true {
		t.Fatalf("expected stream_options.include_usage: %v", raw["stream_options"])
	}
	if u, ok := raw["usage"].(map[string]any); !ok || u["include"] != true {
		t.Fatalf("expected usage.include: %v", raw["usage"])
	}

	cost := 0.0014
	respJSON := `{"id":"x","model":"m","choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":4,"cost":0.0014,"prompt_tokens_details":{"cached_tokens":2},"completion_tokens_details":{"reasoning_tokens":1}}}`
	_ = cost
	resp, err := openai.DecodeResponse([]byte(respJSON))
	if err != nil || resp.Content[0].Text != "hello" || resp.FinishReason != api.FinishEndTurn {
		t.Fatalf("%+v %v", resp, err)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 4 {
		t.Fatalf("usage tokens: %+v", resp.Usage)
	}
	if resp.Usage.CachedTokens != 2 || resp.Usage.ReasoningTokens != 1 {
		t.Fatalf("usage details: %+v", resp.Usage)
	}
	if !resp.Usage.HasProviderCost || resp.Usage.ProviderCostUSD != 0.0014 {
		t.Fatalf("provider cost: %+v", resp.Usage)
	}
}

func TestEncodeThinkingAndCacheControl(t *testing.T) {
	req := api.Request{
		TargetModel:  "m",
		CacheControl: map[string]any{"type": "ephemeral"},
		Thinking:     &api.ThinkingConfig{Type: "enabled", BudgetTokens: 1024},
		SystemBlocks: []api.ContentBlock{{
			Type: api.BlockText, Text: "sys",
			CacheControl: map[string]any{"type": "ephemeral"},
		}},
		Messages: []api.Message{{
			Role: api.RoleAssistant,
			Content: []api.ContentBlock{
				{Type: api.BlockThinking, Text: "plan", Signature: "sig"},
				{Type: api.BlockText, Text: "hi"},
			},
		}, {
			Role: api.RoleUser,
			Content: []api.ContentBlock{{
				Type: api.BlockText, Text: "go",
				CacheControl: map[string]any{"type": "ephemeral"},
			}},
		}},
		Tools: []api.ToolDef{{
			Name: "t", InputSchema: map[string]any{"type": "object"},
			CacheControl: map[string]any{"type": "ephemeral"},
		}},
	}
	b, err := openai.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["cache_control"].(map[string]any)["type"] != "ephemeral" {
		t.Fatalf("%v", raw["cache_control"])
	}
	reasoning := raw["reasoning"].(map[string]any)
	if reasoning["enabled"] != true || int(reasoning["max_tokens"].(float64)) != 1024 {
		t.Fatalf("%v", reasoning)
	}
	msgs := raw["messages"].([]any)
	asst := msgs[1].(map[string]any)
	if asst["reasoning"] != "plan" {
		t.Fatalf("%v", asst)
	}
	tools := raw["tools"].([]any)
	if tools[0].(map[string]any)["cache_control"].(map[string]any)["type"] != "ephemeral" {
		t.Fatalf("%v", tools[0])
	}

	respJSON := `{"id":"x","model":"m","choices":[{"message":{"role":"assistant","content":"hello","reasoning":"think","reasoning_details":[{"type":"reasoning.text","text":"think","signature":"s"}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`
	resp, err := openai.DecodeResponse([]byte(respJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Content) < 2 || resp.Content[0].Type != api.BlockThinking || resp.Content[0].Text != "think" {
		t.Fatalf("%+v", resp.Content)
	}
}

func TestRejectStructured(t *testing.T) {
	_, err := openai.EncodeRequest(api.Request{
		Requirements: api.Requirements{Structured: true},
		Messages:     []api.Message{{Role: api.RoleUser, Content: []api.ContentBlock{{Type: api.BlockText, Text: "hi"}}}},
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

func TestStreamUsageAfterFinish(t *testing.T) {
	sse := "" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3,\"completion_tokens_details\":{\"reasoning_tokens\":1},\"cost\":0.0002}}\n\n" +
		"data: [DONE]\n\n"
	ch := openai.StreamFromString(sse)
	var usage *api.Usage
	var finish api.FinishReason
	for e := range ch {
		if e.Type == api.EventUsage {
			usage = e.Usage
		}
		if e.Type == api.EventFinish {
			finish = e.FinishReason
		}
	}
	if finish != api.FinishEndTurn {
		t.Fatalf("finish=%q", finish)
	}
	if usage == nil || usage.InputTokens != 12 || usage.OutputTokens != 3 || usage.ReasoningTokens != 1 {
		t.Fatalf("usage=%+v", usage)
	}
	if !usage.HasProviderCost || usage.ProviderCostUSD != 0.0002 {
		t.Fatalf("cost=%+v", usage)
	}
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
