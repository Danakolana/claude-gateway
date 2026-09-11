package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danakolana/claude-gateway/internal/history"
)

func TestContinueAfterDesktopApply(t *testing.T) {
	var buf bytes.Buffer
	if !continueAfterDesktopApply(ExitOK, &buf) {
		t.Fatal("ok should continue")
	}
	if buf.Len() != 0 {
		t.Fatalf("ok should be silent: %s", buf.String())
	}
	if continueAfterDesktopApply(ExitUsage, &buf) {
		t.Fatal("user cancel must stop start")
	}
	if !continueAfterDesktopApply(ExitInternal, &buf) {
		t.Fatal("apply IO error should fail-open")
	}
	if !strings.Contains(buf.String(), "WARN") {
		t.Fatalf("expected WARN, got %s", buf.String())
	}
	if !continueAfterDesktopApply(ExitInvalidConfig, &buf) {
		t.Fatal("invalid apply should fail-open on local start")
	}
}

func TestOpenHistoryBestEffort(t *testing.T) {
	var buf bytes.Buffer
	okPath := filepath.Join(t.TempDir(), "h.db")
	store := openHistoryBestEffort(okPath, &buf)
	if store == nil {
		t.Fatalf("expected store: %s", buf.String())
	}
	_ = store.Close()

	block := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(block, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	bad := openHistoryBestEffort(filepath.Join(block, "h.db"), &buf)
	if bad != nil {
		t.Fatal("expected nil store")
	}
	if !strings.Contains(buf.String(), "WARN") || !strings.Contains(buf.String(), "history unavailable") {
		t.Fatalf("expected history WARN, got %s", buf.String())
	}
}

func TestPersistProxyHistorySkipsOnRedactError(t *testing.T) {
	prev := history.RedactFunc
	t.Cleanup(func() { history.RedactFunc = prev })
	history.RedactFunc = func(any) (any, error) {
		return nil, fmt.Errorf("boom")
	}

	store, err := history.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var stderr bytes.Buffer
	persistProxyHistory(&stderr, store, true, "c1", "completed", "cheap",
		[]map[string]any{{"role": "user", "content": "secret"}}, nil)
	ids, err := store.ListConversations(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("redact failure must skip write, got %v", ids)
	}
	if !strings.Contains(stderr.String(), "WARN") {
		t.Fatalf("expected WARN, got %s", stderr.String())
	}
}

func TestPersistProxyHistoryRedactsAPIKey(t *testing.T) {
	store, err := history.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	persistProxyHistory(bytes.NewBuffer(nil), store, true, "c1", "completed", "cheap",
		[]map[string]any{{"role": "user", "content": "my key is sk-or-abcdefghijklmnopqrstuvwxyz"}}, nil)
	ids, err := store.ListConversations(10)
	if err != nil || len(ids) != 1 {
		t.Fatalf("%v %v", ids, err)
	}
}

func TestPersistProxyHistoryNilStore(t *testing.T) {
	persistProxyHistory(bytes.NewBuffer(nil), nil, true, "c1", "completed", "cheap", nil, nil)
}
