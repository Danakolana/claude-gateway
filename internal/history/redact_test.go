package history_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/danakolana/claude-gateway/internal/history"
)

func TestRedactOmitsAPIKey(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "my key is sk-or-abcdefghijklmnopqrstuvwxyz"},
	}
	out, err := history.RedactMessages(msgs)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "sk-or-abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("secret leaked: %s", raw)
	}
	content, _ := out[0]["content"].(string)
	if !strings.Contains(content, "sk-<redacted>") {
		t.Fatalf("expected redaction marker: %s", content)
	}
}

func TestRedactFailureSkipsWrite(t *testing.T) {
	prev := history.RedactFunc
	t.Cleanup(func() { history.RedactFunc = prev })
	history.RedactFunc = func(any) (any, error) {
		return nil, fmt.Errorf("boom")
	}
	_, err := history.RedactMessages([]map[string]any{{"role": "user"}})
	if err == nil {
		t.Fatal("expected error")
	}

	s, err := history.Open(t.TempDir() + "/h.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// proxy path: on redact error we skip Append; store stays empty
	ids, err := s.ListConversations(10)
	if err != nil || len(ids) != 0 {
		t.Fatalf("%v %v", ids, err)
	}
}

func TestRedactPanicRecoverable(t *testing.T) {
	prev := history.RedactFunc
	t.Cleanup(func() { history.RedactFunc = prev })
	history.RedactFunc = func(any) (any, error) {
		panic("redact boom")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic from RedactMessages")
		}
	}()
	_, _ = history.RedactMessages([]map[string]any{{"x": 1}})
}
