package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
