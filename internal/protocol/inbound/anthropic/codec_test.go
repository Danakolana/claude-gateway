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
	// thinking / unknown blocks are stripped, not fatal
	req2, err := anthropic.DecodeRequest([]byte(`{"model":"x","max_tokens":1,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"x"},{"type":"text","text":"hi"}]},{"role":"user","content":"go"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req2.Messages) != 2 || len(req2.Messages[0].Content) != 1 || req2.Messages[0].Content[0].Text != "hi" {
		t.Fatalf("thinking should be stripped: %+v", req2.Messages)
	}
}
