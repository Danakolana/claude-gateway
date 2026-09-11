package routing

import "sync"

// PinStore remembers the last successful OpenRouter backend per target model
// so sticky routing can reuse the same provider (prompt-cache hits).
type PinStore struct {
	mu   sync.Mutex
	last map[string]string
}

// NewPinStore returns an empty pin map.
func NewPinStore() *PinStore {
	return &PinStore{last: make(map[string]string)}
}

// Get returns the pinned backend for model, or "".
func (s *PinStore) Get(model string) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last[model]
}

// Remember records backend for model. Empty values are ignored.
func (s *PinStore) Remember(model, backend string) {
	if s == nil || model == "" || backend == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		s.last = make(map[string]string)
	}
	s.last[model] = backend
}
