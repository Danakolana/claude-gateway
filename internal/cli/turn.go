package cli

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/danakolana/claude-gateway/internal/modelstatus"
	"github.com/danakolana/claude-gateway/pkg/api"
)

// turnIdleFlush is a backup when a tool_use turn never gets an end_turn
// (user cancel, Desktop crash). Long enough that local tool runs do not
// split one agent turn into multiple totals.
const turnIdleFlush = 12 * time.Second

// turnTotals rolls up /v1/messages usage across one Desktop/agent turn.
// Per-request lines stay as-is; a turn total is printed when the model
// finishes (not tool_use) with 2+ requests, or after idle as a backup.
type turnTotals struct {
	mu       sync.Mutex
	w        io.Writer
	requests int
	inTok    int
	outTok   int
	usd      float64
	hasCost  bool
	timer    *time.Timer
	idle     time.Duration
}

func newTurnTotals(w io.Writer) *turnTotals {
	return &turnTotals{w: w, idle: turnIdleFlush}
}

func (t *turnTotals) add(u api.Usage, estUSD float64, hasCost bool, finish api.FinishReason) {
	if t == nil || t.w == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requests++
	t.inTok += u.InputTokens
	t.outTok += u.OutputTokens
	if hasCost {
		t.usd += estUSD
		t.hasCost = true
	}
	t.resetIdleLocked()

	// tool_use → Desktop will call again; keep accumulating.
	if finish == api.FinishToolUse {
		return
	}
	t.flushLocked()
}

func (t *turnTotals) resetIdleLocked() {
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	idle := t.idle
	if idle <= 0 {
		idle = turnIdleFlush
	}
	t.timer = time.AfterFunc(idle, func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.flushLocked()
	})
}

func (t *turnTotals) flushLocked() {
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	if t.requests < 2 {
		t.requests, t.inTok, t.outTok, t.usd, t.hasCost = 0, 0, 0, 0, false
		return
	}
	line := modelstatus.FormatTurnTotal(t.requests, t.inTok, t.outTok, t.usd, t.hasCost)
	fmt.Fprintln(t.w, line)
	t.requests, t.inTok, t.outTok, t.usd, t.hasCost = 0, 0, 0, 0, false
}
