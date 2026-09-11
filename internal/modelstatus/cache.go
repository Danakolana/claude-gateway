package modelstatus

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// CatalogCache holds last-known live prices. Failed refreshes keep the previous
// snapshot (ADR-014).
type CatalogCache struct {
	mu        sync.RWMutex
	live      map[string]LiveModel
	fetchedAt time.Time
	lastErr   string
}

func NewCatalogCache(initial map[string]LiveModel) *CatalogCache {
	c := &CatalogCache{}
	c.ReplaceOK(initial)
	return c
}

// ReplaceOK stores a non-empty catalog. Empty/nil is ignored so stale data stays.
func (c *CatalogCache) ReplaceOK(live map[string]LiveModel) {
	if c == nil || len(live) == 0 {
		return
	}
	cp := make(map[string]LiveModel, len(live))
	for k, v := range live {
		cp[k] = v
	}
	c.mu.Lock()
	c.live = cp
	c.fetchedAt = time.Now().UTC()
	c.lastErr = ""
	c.mu.Unlock()
}

// Snapshot copies last-known prices. Never fails.
func (c *CatalogCache) Snapshot() (live map[string]LiveModel, fetchedAt time.Time, lastErr string) {
	if c == nil {
		return nil, time.Time{}, ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.live == nil {
		return nil, c.fetchedAt, c.lastErr
	}
	cp := make(map[string]LiveModel, len(c.live))
	for k, v := range c.live {
		cp[k] = v
	}
	return cp, c.fetchedAt, c.lastErr
}

func (c *CatalogCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.live)
}

func (c *CatalogCache) LastError() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastErr
}

// Refresh fetches the catalog. On error it keeps the previous snapshot and
// records lastErr.
func (c *CatalogCache) Refresh(ctx context.Context, baseURL, apiKey string, client *http.Client) {
	if c == nil {
		return
	}
	live, err := FetchCatalog(ctx, baseURL, apiKey, client)
	if err != nil {
		c.mu.Lock()
		c.lastErr = err.Error()
		c.mu.Unlock()
		return
	}
	c.ReplaceOK(live)
}

// Probe is an advisory provider/model health snapshot. It never gates routing.
type Probe struct {
	mu         sync.Mutex
	ok         bool
	msg        string
	checkedAt  time.Time
	modelHints map[string]string
}

func NewProbe() *Probe {
	return &Probe{ok: true, msg: "not checked yet", modelHints: map[string]string{}}
}

func (p *Probe) RecordProvider(ok bool, msg string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.ok, p.msg, p.checkedAt = ok, msg, time.Now().UTC()
	p.mu.Unlock()
}

func (p *Probe) RecordModel(id, hint string) {
	if p == nil || id == "" {
		return
	}
	p.mu.Lock()
	if p.modelHints == nil {
		p.modelHints = map[string]string{}
	}
	if hint == "" {
		delete(p.modelHints, id)
	} else {
		p.modelHints[id] = hint
	}
	p.mu.Unlock()
}

// View is a copy for JSON. Never fails.
func (p *Probe) View() (ok bool, msg string, checkedAt time.Time, hints map[string]string) {
	if p == nil {
		return true, "", time.Time{}, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	ok, msg, checkedAt = p.ok, p.msg, p.checkedAt
	if len(p.modelHints) > 0 {
		hints = make(map[string]string, len(p.modelHints))
		for k, v := range p.modelHints {
			hints[k] = v
		}
	}
	return ok, msg, checkedAt, hints
}
