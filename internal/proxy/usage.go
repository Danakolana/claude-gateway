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
	Catalog *CatalogInfo    `json:"catalog,omitempty"`
	Health  *HealthInfo     `json:"health,omitempty"`
	Drift   string          `json:"drift,omitempty"`
}

// CatalogInfo is last-known live price catalog metadata (no secrets).
type CatalogInfo struct {
	FetchedAt string `json:"fetched_at,omitempty"`
	Models    int    `json:"models"`
	Error     string `json:"error,omitempty"`
}

// HealthInfo is advisory provider reachability. It never gates routing.
type HealthInfo struct {
	OK        bool              `json:"ok"`
	Message   string            `json:"message,omitempty"`
	CheckedAt string            `json:"checked_at,omitempty"`
	Models    map[string]string `json:"models,omitempty"`
}

// UsageStore is an in-memory sidecar. It must not fail the proxy request path.
type UsageStore struct {
	mu      sync.Mutex
	items   []UsageSnapshot
	session SessionSpend
	catalog *CatalogInfo
	health  *HealthInfo
	drift   string
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

func (u *UsageStore) SetCatalog(info CatalogInfo) {
	if u == nil {
		return
	}
	u.mu.Lock()
	cp := info
	u.catalog = &cp
	u.mu.Unlock()
}

func (u *UsageStore) SetHealth(info HealthInfo) {
	if u == nil {
		return
	}
	u.mu.Lock()
	cp := info
	u.health = &cp
	u.mu.Unlock()
}

func (u *UsageStore) SetDrift(msg string) {
	if u == nil {
		return
	}
	u.mu.Lock()
	u.drift = msg
	u.mu.Unlock()
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
	view := UsageView{OK: true, Session: u.session, Recent: recent, Drift: u.drift}
	if u.catalog != nil {
		cp := *u.catalog
		view.Catalog = &cp
	}
	if u.health != nil {
		hp := *u.health
		if len(hp.Models) > 0 {
			m := make(map[string]string, len(hp.Models))
			for k, v := range hp.Models {
				m[k] = v
			}
			hp.Models = m
		}
		view.Health = &hp
	}
	if len(u.items) > 0 {
		last := u.items[len(u.items)-1]
		view.Last = &last
	}
	var notes []string
	if u.session.CacheMissWarning {
		notes = append(notes, "repeated large prompts with no cache_read — upstream may not support prompt cache")
	}
	if u.health != nil && !u.health.OK && u.health.Message != "" {
		notes = append(notes, "provider health: "+u.health.Message)
	}
	if u.catalog != nil && u.catalog.Error != "" {
		notes = append(notes, "price catalog stale: "+u.catalog.Error)
	}
	if u.drift != "" {
		notes = append(notes, u.drift)
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
func SnapshotFrom(req api.Request, resp api.Response, inPerMTok, outPerMTok float64, hasPrice bool, contextLimit int) UsageSnapshot {
	outcome := "completed"
	if resp.Error != nil || resp.FinishReason == api.FinishCancelled || resp.FinishReason == api.FinishError {
		outcome = "incomplete"
	}
	target := req.TargetModel
	if target == "" {
		target = resp.Model
	}
	usd, src := modelstatus.CostUSD(resp.Usage, inPerMTok, outPerMTok, hasPrice)
	notes := modelstatus.AdvisoryNotes(resp.Usage)
	notes = append(notes, modelstatus.ContextNotes(req, resp.Usage, contextLimit)...)
	s := UsageSnapshot{
		Time:        time.Now().UTC(),
		ID:          req.ID,
		SourceModel: req.SourceModel,
		TargetModel: target,
		Stream:      req.Stream,
		Outcome:     outcome,
		Usage:       resp.Usage,
		CostSource:  src,
		Notes:       notes,
	}
	if src != modelstatus.CostNone {
		v := usd
		s.EstUSD = &v
	}
	return s
}
