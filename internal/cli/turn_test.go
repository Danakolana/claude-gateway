package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/danakolana/claude-gateway/pkg/api"
)

func TestTurnTotalsFlushOnEndTurn(t *testing.T) {
	var buf bytes.Buffer
	tt := newTurnTotals(&buf)
	tt.idle = time.Hour // disable idle path for this test

	u := api.Usage{InputTokens: 10, OutputTokens: 5}
	tt.add(u, 0.01, true, api.FinishToolUse)
	tt.add(u, 0.02, true, api.FinishToolUse)
	if buf.Len() != 0 {
		t.Fatalf("should not flush during tool_use: %s", buf.String())
	}
	tt.add(u, 0.03, true, api.FinishEndTurn)
	out := buf.String()
	if !strings.Contains(out, "Turn total") || !strings.Contains(out, "requests") || !strings.Contains(out, "3") {
		t.Fatalf("expected turn total after end_turn: %s", out)
	}
	if !strings.Contains(out, "0.060000") {
		t.Fatalf("expected summed cost: %s", out)
	}
}

func TestTurnTotalsSkipSingleRequest(t *testing.T) {
	var buf bytes.Buffer
	tt := newTurnTotals(&buf)
	tt.idle = time.Hour
	tt.add(api.Usage{InputTokens: 1, OutputTokens: 1}, 0.01, true, api.FinishEndTurn)
	if buf.Len() != 0 {
		t.Fatalf("single request should not print turn total: %s", buf.String())
	}
}

func TestTurnTotalsIdleFlush(t *testing.T) {
	var buf bytes.Buffer
	tt := newTurnTotals(&buf)
	tt.idle = 30 * time.Millisecond
	tt.add(api.Usage{InputTokens: 2, OutputTokens: 2}, 0.01, true, api.FinishToolUse)
	tt.add(api.Usage{InputTokens: 3, OutputTokens: 3}, 0.02, true, api.FinishToolUse)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "Turn total") && strings.Contains(buf.String(), "requests") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected idle flush: %q", buf.String())
}
