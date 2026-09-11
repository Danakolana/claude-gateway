package history_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	for _, name := range []string{"manifest.json", "conversations.jsonl", "messages.jsonl", "checksums.txt"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatal(name, err)
		}
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	src, err := history.Open(filepath.Join(t.TempDir(), "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if err := src.AppendConversation(context.Background(), "c1", "completed", []map[string]any{
		{"role": "user", "content": "hello", "correlation_id": "corr-1"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "exp")
	if err := src.ExportJSONL(dir); err != nil {
		t.Fatal(err)
	}

	dst, err := history.Open(filepath.Join(t.TempDir(), "dst.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	res, err := dst.ImportJSONL(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.ConversationsImported != 1 || res.MessagesImported != 1 {
		t.Fatalf("imported %+v", res)
	}
	ids, err := dst.ListConversations(10)
	if err != nil || len(ids) != 1 || ids[0] != "c1" {
		t.Fatalf("ids %v %v", ids, err)
	}

	skip, err := dst.ImportJSONL(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if skip.ConversationsSkipped != 1 || skip.ConversationsImported != 0 || skip.MessagesSkipped != 1 {
		t.Fatalf("skip %+v", skip)
	}

	repl, err := dst.ImportJSONL(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if repl.ConversationsImported != 1 || repl.MessagesImported != 1 || repl.ConversationsSkipped != 0 {
		t.Fatalf("replace %+v", repl)
	}
}

func TestImportChecksumMismatch(t *testing.T) {
	src, err := history.Open(filepath.Join(t.TempDir(), "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if err := src.AppendConversation(context.Background(), "c1", "completed", []map[string]any{
		{"role": "user", "content": "hi"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "exp")
	if err := src.ExportJSONL(dir); err != nil {
		t.Fatal(err)
	}
	msgPath := filepath.Join(dir, "messages.jsonl")
	if err := os.WriteFile(msgPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dst, err := history.Open(filepath.Join(t.TempDir(), "dst.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	_, err = dst.ImportJSONL(dir, false)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("want checksum mismatch, got %v", err)
	}
}
