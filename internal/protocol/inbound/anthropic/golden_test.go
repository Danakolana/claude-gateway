package anthropic_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/danakolana/claude-gateway/internal/protocol/inbound/anthropic"
	"github.com/danakolana/claude-gateway/internal/provider/openai"
	"github.com/danakolana/claude-gateway/pkg/api"
)

func testdata(t *testing.T, rel string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	return filepath.Join(root, "testdata", rel)
}

func TestGoldenInboundToolsRoundTrip(t *testing.T) {
	raw, err := os.ReadFile(testdata(t, "protocol/anthropic/inbound_tools_roundtrip.json"))
	if err != nil {
		t.Fatal(err)
	}
	req, err := anthropic.DecodeRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !req.Stream || !req.Requirements.Tools {
		t.Fatalf("%+v", req)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("messages=%d", len(req.Messages))
	}
	asst := req.Messages[1]
	if len(asst.Content) != 1 || asst.Content[0].Type != api.BlockToolUse {
		t.Fatalf("%+v", asst.Content)
	}
	if req.Messages[2].Content[0].ToolContent != "22C sunny" {
		t.Fatalf("%q", req.Messages[2].Content[0].ToolContent)
	}

	req.TargetModel = "deepseek/deepseek-v4-flash-0731"
	req.System = "be brief"
	out, err := openai.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "deepseek/deepseek-v4-flash-0731" {
		t.Fatalf("%v", got["model"])
	}
	if got["tool_choice"] != "auto" {
		t.Fatalf("tool_choice=%v", got["tool_choice"])
	}
	msgs := got["messages"].([]any)
	roles := make([]string, 0, len(msgs))
	for _, m := range msgs {
		roles = append(roles, m.(map[string]any)["role"].(string))
	}
	// system, user, assistant(tool_calls), tool
	if len(roles) < 4 || roles[0] != "system" || roles[len(roles)-1] != "tool" {
		t.Fatalf("roles=%v body=%s", roles, out)
	}
	wantRaw, err := os.ReadFile(testdata(t, "protocol/openai/outbound_tools_roundtrip.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		t.Fatal(err)
	}
	if len(want["messages"].([]any)) != len(msgs) {
		t.Fatalf("message count got=%d want=%d\ngot=%s", len(msgs), len(want["messages"].([]any)), out)
	}
}

func TestGoldenInboundText(t *testing.T) {
	raw, err := os.ReadFile(testdata(t, "protocol/anthropic/inbound_text.json"))
	if err != nil {
		t.Fatal(err)
	}
	req, err := anthropic.DecodeRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if req.SourceModel != "claude-haiku-4" || req.System != "be brief" {
		t.Fatalf("%+v", req)
	}
	if req.Temperature == nil || *req.Temperature != 0.2 {
		t.Fatalf("%v", req.Temperature)
	}
}
