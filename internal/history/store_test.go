package history_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/danakolana/claude-gateway/internal/history"
)

func TestAppendExport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "h.db")
	s, err := history.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.AppendConversation(context.Background(), "c1", "completed", []map[string]any{
		{"role": "user", "content": "hi", "correlation_id": "x"},
	}, map[string]any{"provider": "fake", "outcome": "completed"}); err != nil {
		t.Fatal(err)
	}
	ids, err := s.ListConversations(10)
	if err != nil || len(ids) != 1 {
		t.Fatalf("%v %v", ids, err)
	}
	out := filepath.Join(t.TempDir(), "exp")
	if err := s.ExportJSONL(out); err != nil {
		t.Fatal(err)
	}
}
