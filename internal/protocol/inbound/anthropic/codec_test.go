package anthropic_test

import (
	"testing"

	"github.com/danakolana/claude-gateway/internal/protocol/inbound/anthropic"
)

func TestDecodeEncode(t *testing.T) {
	raw := []byte(`{"model":"claude-sonnet","max_tokens":32,"system":"sys","messages":[{"role":"user","content":"hi"}],"tools":[{"name":"t","input_schema":{"type":"object"}}],"tool_choice":{"type":"auto"},"temperature":0.2}`)
	req, err := anthropic.DecodeRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if req.SourceModel != "claude-sonnet" || !req.Requirements.Tools {
		t.Fatalf("%+v", req)
	}
	if req.Temperature == nil || *req.Temperature != 0.2 {
		t.Fatalf("temperature %+v", req.Temperature)
	}
	// thinking blocks are preserved (forwarded upstream as reasoning)
	req2, err := anthropic.DecodeRequest([]byte(`{"model":"x","max_tokens":1,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"plan","signature":"sig"},{"type":"text","text":"hi"}]},{"role":"user","content":"go"}],"cache_control":{"type":"ephemeral"},"thinking":{"type":"enabled","budget_tokens":2048},"system":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req2.Messages) != 2 || len(req2.Messages[0].Content) != 2 {
		t.Fatalf("thinking should be kept: %+v", req2.Messages)
	}
	if req2.Messages[0].Content[0].Type != "thinking" || req2.Messages[0].Content[0].Text != "plan" {
		t.Fatalf("%+v", req2.Messages[0].Content[0])
	}
	if req2.CacheControl["type"] != "ephemeral" {
		t.Fatalf("cache_control %+v", req2.CacheControl)
	}
	if req2.Thinking == nil || req2.Thinking.BudgetTokens != 2048 {
		t.Fatalf("thinking %+v", req2.Thinking)
	}
	if len(req2.SystemBlocks) != 1 || req2.SystemBlocks[0].CacheControl["type"] != "ephemeral" {
		t.Fatalf("system blocks %+v", req2.SystemBlocks)
	}
}
