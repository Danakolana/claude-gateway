package routing

import (
	"sync"
	"time"

	"github.com/danakolana/claude-gateway/pkg/api"
)

// Breaker skips a model key after N transient failures, only when Resolve
// still has another eligible candidate. Disabled when Failures == 0.
// Skip/Fail recover from panic and fail open (send the original target).
type Breaker struct {
	Failures int
	Cooldown time.Duration

	mu    sync.Mutex
	count map[string]int
	until map[string]time.Time
}

func NewBreaker(failures int, cooldown time.Duration) *Breaker {
	if failures <= 0 {
		return nil
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &Breaker{
		Failures: failures,
		Cooldown: cooldown,
		count:    map[string]int{},
		until:    map[string]time.Time{},
	}
}

func (b *Breaker) enabled() bool {
	return b != nil && b.Failures > 0
}

// Skip reports whether key is currently open. Panic or unset breaker → false.
func (b *Breaker) Skip(key string) (open bool) {
	defer func() {
		if recover() != nil {
			open = false
		}
	}()
	if !b.enabled() || key == "" {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	until, ok := b.until[key]
	if !ok {
		return false
	}
	if time.Now().Before(until) {
		return true
	}
	delete(b.until, key)
	b.count[key] = 0
	return false
}

// Fail records a transient failure. Panic is ignored.
func (b *Breaker) Fail(key string) {
	defer func() { _ = recover() }()
	if !b.enabled() || key == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.count == nil {
		b.count = map[string]int{}
	}
	if b.until == nil {
		b.until = map[string]time.Time{}
	}
	b.count[key]++
	if b.count[key] >= b.Failures {
		b.until[key] = time.Now().Add(b.Cooldown)
	}
}

// OK clears failure state after a successful call. Panic is ignored.
func (b *Breaker) OK(key string) {
	defer func() { _ = recover() }()
	if !b.enabled() || key == "" {
		return
	}
	b.mu.Lock()
	delete(b.count, key)
	delete(b.until, key)
	b.mu.Unlock()
}

func breakerFailure(err error, resp api.Response) bool {
	if err != nil {
		if ae, ok := err.(*api.Error); ok {
			switch ae.Category {
			case api.ErrProviderAuth, api.ErrInvalidConfig, api.ErrUnsupportedCapability:
				return false
			default:
				return true
			}
		}
		return true
	}
	if resp.Error == nil {
		return false
	}
	if resp.Error.Retryable {
		return true
	}
	switch resp.Error.Category {
	case api.ErrProviderTransient, api.ErrProviderRateLimit, api.ErrProviderProtocol:
		return true
	default:
		return false
	}
}
