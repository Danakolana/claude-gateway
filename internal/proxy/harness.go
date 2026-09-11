package proxy

import (
	"strings"
	"sync"
	"time"

	"github.com/danakolana/claude-gateway/pkg/api"
)

const harnessRingSize = 24
const harnessSystemMax = 16000

// HarnessItem is a snapshot of what Claude Desktop sent (opt-in inspect).
type HarnessItem struct {
	Time          time.Time `json:"time"`
	ID            string    `json:"id"`
	SourceModel   string    `json:"source_model"`
	TargetModel   string    `json:"target_model,omitempty"`
	Stream        bool      `json:"stream"`
	SystemChars   int       `json:"system_chars"`
	SystemPreview string    `json:"system_preview"`
	ToolNames     []string  `json:"tool_names"`
	ToolCount     int       `json:"tool_count"`
	MessageCount  int       `json:"message_count"`
}

// HarnessStore keeps recent request harness snapshots when inspect is enabled.
type HarnessStore struct {
	mu      sync.RWMutex
	enabled bool
	items   []HarnessItem
}

func NewHarnessStore(enabled bool) *HarnessStore {
	return &HarnessStore{enabled: enabled, items: make([]HarnessItem, 0, harnessRingSize)}
}

func (h *HarnessStore) Enabled() bool {
	if h == nil {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.enabled
}

func (h *HarnessStore) Record(req api.Request, targetModel string) {
	if h == nil || !h.Enabled() {
		return
	}
	sys := req.System
	if sys == "" && len(req.SystemBlocks) > 0 {
		var b strings.Builder
		for _, blk := range req.SystemBlocks {
			if blk.Type == api.BlockText {
				b.WriteString(blk.Text)
			}
		}
		sys = b.String()
	}
	names := make([]string, 0, len(req.Tools))
	for _, t := range req.Tools {
		names = append(names, t.Name)
	}
	item := HarnessItem{
		Time:          time.Now().UTC(),
		ID:            req.ID,
		SourceModel:   req.SourceModel,
		TargetModel:   targetModel,
		Stream:        req.Stream,
		SystemChars:   len(sys),
		SystemPreview: truncateBytes(sys, harnessSystemMax),
		ToolNames:     names,
		ToolCount:     len(names),
		MessageCount:  len(req.Messages),
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.items = append(h.items, item)
	if len(h.items) > harnessRingSize {
		h.items = h.items[len(h.items)-harnessRingSize:]
	}
}

func (h *HarnessStore) List() []HarnessItem {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]HarnessItem, len(h.items))
	copy(out, h.items)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func truncateBytes(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max < 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
