package routing

import (
	"context"
	"time"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/provider"
	"github.com/danakolana/claude-gateway/pkg/api"
)

// RetryPolicy controls retries on retryable provider errors.
type RetryPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
}

func (p RetryPolicy) withDefaults() RetryPolicy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 2
	}
	if p.Backoff <= 0 {
		p.Backoff = 50 * time.Millisecond
	}
	return p
}

// Engine routes requests through a registry and adapter.
type Engine struct {
	Registry    *Registry
	Adapter     provider.Adapter
	Provider    string
	ProviderCfg config.Provider
	Routing     config.Routing
	Retry       RetryPolicy
	Breaker     *Breaker
}

// Route resolves the target model.
func (e *Engine) Route(req api.Request) (Decision, error) {
	source := req.SourceModel
	d, err := e.Registry.Resolve(source, e.Provider, e.Routing, req.Requirements, req.EstimatedTokens)
	if err != nil {
		return d, err
	}
	if e.Breaker != nil && e.Breaker.Skip(d.TargetKey) {
		d2, err2 := e.Registry.resolve(source, e.Provider, e.Routing, req.Requirements, req.EstimatedTokens, map[string]bool{d.TargetKey: true})
		if err2 == nil && d2.TargetKey != d.TargetKey {
			d2.Reason = "circuit_breaker"
			return d2, nil
		}
	}
	return d, nil
}

func (e *Engine) prepare(req *api.Request) (Decision, error) {
	d, err := e.Route(*req)
	if err != nil {
		return d, err
	}
	req.TargetModel = d.TargetModel
	m := config.Model{}
	if e.Registry != nil {
		if mm, ok := e.Registry.Models[d.TargetKey]; ok {
			m = mm
		}
	}
	ApplyCostPolicies(req, m, e.Routing, e.ProviderCfg)
	return d, nil
}

// Send routes and sends a non-streaming request.
func (e *Engine) Send(ctx context.Context, req api.Request) (api.Response, Decision, error) {
	d, err := e.prepare(&req)
	if err != nil {
		return api.Response{}, d, err
	}
	retry := e.Retry.withDefaults()
	var last api.Response
	for attempt := 1; attempt <= retry.MaxAttempts; attempt++ {
		resp, err := e.Adapter.Send(ctx, req)
		if breakerFailure(err, resp) {
			if e.Breaker != nil {
				e.Breaker.Fail(d.TargetKey)
			}
		} else if err == nil {
			if e.Breaker != nil {
				e.Breaker.OK(d.TargetKey)
			}
		}
		if err != nil {
			return api.Response{}, d, err
		}
		last = resp
		if resp.Error == nil || !resp.Error.Retryable {
			return resp, d, nil
		}
		// Do not retry if upstream already billed output tokens.
		if resp.Usage.OutputTokens > 0 {
			return resp, d, nil
		}
		select {
		case <-ctx.Done():
			return api.Response{Error: &api.Error{Category: api.ErrRequestCancelled, Message: "cancelled"}}, d, nil
		case <-time.After(retry.Backoff * time.Duration(attempt)):
		}
	}
	return last, d, nil
}

// Stream routes and streams.
func (e *Engine) Stream(ctx context.Context, req api.Request) (<-chan api.Event, Decision, error) {
	d, err := e.prepare(&req)
	if err != nil {
		return nil, d, err
	}
	req.Stream = true
	ch, err := e.Adapter.Stream(ctx, req)
	if err != nil && e.Breaker != nil {
		e.Breaker.Fail(d.TargetKey)
	}
	return ch, d, err
}
