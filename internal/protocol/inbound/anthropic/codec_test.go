package anthropic_test

import (
	"testing"

	"github.com/danakolana/claude-gateway/internal/protocol/inbound/anthropic"
)

func TestDecodeEncode(t *testing.T) {
	raw := []byte(`{"model":"claude-sonnet","max_tokens":32,"system":"sys","messages":[{"role":"user","content":"hi"}],"tools":[{"name":"t","input_schema":{"type":"object"}}]}`)
	req, err := anthropic.DecodeRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if req.SourceModel != "claude-sonnet" || !req.Requirements.Tools {
		t.Fatalf("%+v", req)
	}
	_, err = anthropic.DecodeRequest([]byte(`{"model":"x","max_tokens":1,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"x"}]}]}`))
	if err == nil {
		t.Fatal("thinking should fail")
	}
}
