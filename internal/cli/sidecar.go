package cli

import (
	"fmt"
	"io"

	"github.com/danakolana/claude-gateway/internal/history"
)

// continueAfterDesktopApply reports whether local proxy start should proceed
// after a failed Desktop apply. User cancel (ExitUsage) is not fail-open.
func continueAfterDesktopApply(code int, stderr io.Writer) bool {
	if code == ExitOK {
		return true
	}
	if code == ExitUsage {
		return false
	}
	fmt.Fprintln(stderr, "WARN: desktop apply failed; local proxy will still start. Re-run: claude-gateway client apply")
	return true
}

// openHistoryBestEffort opens SQLite history. Failures are WARN-only so the
// local proxy can still serve /v1/messages (ADR-014).
func openHistoryBestEffort(path string, stderr io.Writer) *history.Store {
	store, err := history.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "WARN: history unavailable (%v); proxy continues without local history\n", err)
		return nil
	}
	return store
}
