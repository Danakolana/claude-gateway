package proxy

import (
	"sync"
	"time"

	"github.com/danakolana/claude-gateway/internal/modelstatus"
	"github.com/danakolana/claude-gateway/pkg/api"
)

const usageRingSize = 24
const cacheMissNagStreak = 2

// UsageSnapshot is request metadata for the local guide. No prompts or tools.
type UsageSnapshot struct {
	Time        time.Time `json:"time"`
	ID          string    `json:"id"`
	SourceModel string    `json:"source_model"`
	TargetModel string    `json:"target_model,omitempty"`
	Stream      bool      `json:"stream"`
	Outcome     string    `json:"outcome"`
	Usage       api.Usage `json:"usage"`
	EstUSD      *float64  `json:"est_usd,omitempty"`
	CostSource  string    `json:"cost_source,omitempty"`
	Notes       []string  `json:"notes,omitempty"`
}

// SessionSpend is this-process totals (not billed, not durable).
type SessionSpend struct {
	Started          time.Time `json:"started"`
	Requests         int       `json:"requests"`
	InputTokens      int       `json:"input_tokens"`
	OutputTokens     int       `json:"output_tokens"`
	EstUSD           float64   `json:"est_usd"`
	CacheMissStreak  int       `json:"cache_miss_streak"`
	CacheMissWarning bool      `json:"cache_miss_warning"`
}

// UsageView is the JSON body for GET /debug/usage.
type UsageView struct {
	OK      bool            `json:"ok"`
	Session SessionSpend    `json:"session"`
	Last    *UsageSnapshot  `json:"last"`
	Recent  []UsageSnapshot `json:"recent"`
	Notes   []string        `json:"notes,omitempty"`
}

// UsageStore is an in-memory sidecar. It must not fail the proxy request path.
type UsageStore struct {
	mu      sync.Mutex
	items   []UsageSnapshot
	session SessionSpend
}

func NewUsageStore() *UsageStore {
	now := time.Now().UTC()
	return &UsageStore{
		items:   make([]UsageSnapshot, 0, usageRingSize),
		session: SessionSpend{Started: now},
	}
}

// Record appends a snapshot and updates session totals. Never panics callers:
// the proxy recovers around OnRequest as well.
func (u *UsageStore) Record(s UsageSnapshot) {
	if u == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if s.Time.IsZero() {
		s.Time = time.Now().UTC()
	}
	if s.Outcome == "" {
		s.Outcome = "completed"
	}
	u.items = append(u.items, s)
	if len(u.items) > usageRingSize {
		u.items = u.items[len(u.items)-usageRingSize:]
	}
	u.session.Requests++
	u.session.InputTokens += s.Usage.InputTokens
	u.session.OutputTokens += s.Usage.OutputTokens
	if s.EstUSD != nil {
		u.session.EstUSD += *s.EstUSD
	}
	if isCacheMiss(s.Usage) {
		u.session.CacheMissStreak++
	} else if s.Usage.InputTokens > 0 {
		u.session.CacheMissStreak = 0
	}
	u.session.CacheMissWarning = u.session.CacheMissStreak >= cacheMissNagStreak
}

func (u *UsageStore) View() UsageView {
	if u == nil {
		return UsageView{OK: true, Recent: []UsageSnapshot{}}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	recent := make([]UsageSnapshot, len(u.items))
	copy(recent, u.items)
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	view := UsageView{OK: true, Session: u.session, Recent: recent}
	if len(u.items) > 0 {
		last := u.items[len(u.items)-1]
		view.Last = &last
	}
	var notes []string
	if u.session.CacheMissWarning {
		notes = append(notes, "repeated large prompts with no cache_read — upstream may not support prompt cache")
	}
	if view.Last != nil {
		notes = append(notes, view.Last.Notes...)
	}
	view.Notes = notes
	return view
}

func isCacheMiss(u api.Usage) bool {
	return u.InputTokens >= modelstatus.CacheMissWarnInputTokens && u.CachedTokens == 0 && u.CacheWriteTokens == 0
}

// SnapshotFrom builds a prompt-free snapshot from a completed request.
func SnapshotFrom(req api.Request, resp api.Response, inPerMTok, outPerMTok float64, hasPrice bool) UsageSnapshot {
	outcome := "completed"
	if resp.Error != nil || resp.FinishReason == api.FinishCancelled || resp.FinishReason == api.FinishError {
		outcome = "incomplete"
	}
	target := req.TargetModel
	if target == "" {
		target = resp.Model
	}
	usd, src := modelstatus.CostUSD(resp.Usage, inPerMTok, outPerMTok, hasPrice)
	s := UsageSnapshot{
		Time:        time.Now().UTC(),
		ID:          req.ID,
		SourceModel: req.SourceModel,
		TargetModel: target,
		Stream:      req.Stream,
		Outcome:     outcome,
		Usage:       resp.Usage,
		CostSource:  src,
		Notes:       modelstatus.AdvisoryNotes(resp.Usage),
	}
	if src != modelstatus.CostNone {
		v := usd
		s.EstUSD = &v
	}
	return s
}
