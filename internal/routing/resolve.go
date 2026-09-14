package routing

import (
	"fmt"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/pkg/api"
)

// Decision explains a routing outcome.
type Decision struct {
	Source       string
	TargetKey    string
	TargetModel  string
	Provider     string
	Reason       string
	FallbackUsed bool
	Attempt      int
}

// Registry holds models from config.
type Registry struct {
	Models map[string]config.Model
}

func NewRegistry(models map[string]config.Model) *Registry {
	if models == nil {
		models = map[string]config.Model{}
	}
	return &Registry{Models: models}
}

// Resolve picks a target for source model/tier with capability checks.
func (r *Registry) Resolve(source, provider string, routing config.Routing, req api.Requirements, estimatedTokens int) (Decision, error) {
	return r.resolve(source, provider, routing, req, estimatedTokens, nil)
}

func (r *Registry) resolve(source, provider string, routing config.Routing, req api.Requirements, estimatedTokens int, skip map[string]bool) (Decision, error) {
	candidates := r.candidates(source, routing)
	if len(candidates) == 0 {
		return Decision{}, &api.Error{Category: api.ErrUnsupportedCapability, Message: "no routing candidate for " + source}
	}
	type eligible struct {
		i        int
		key      string
		m        config.Model
		fromRule bool
	}
	var okList []eligible
	for i, key := range candidates {
		m, ok := r.Models[key]
		if !ok || !m.Enabled {
			continue
		}
		if skip[key] {
			continue
		}
		caps := modelCaps(m)
		req2 := req
		if estimatedTokens > 0 {
			req2.MinContext = estimatedTokens
		}
		if !caps.Compatible(req2) {
			continue
		}
		okList = append(okList, eligible{i: i, key: key, m: m, fromRule: i < len(routing.Rules)})
	}
	if len(okList) == 0 {
		if len(skip) > 0 {
			// Fail open: ignore skip and resolve normally.
			return r.resolve(source, provider, routing, req, estimatedTokens, nil)
		}
		return Decision{}, &api.Error{
			Category: api.ErrUnsupportedCapability,
			Message:  fmt.Sprintf("no eligible model for %q (capabilities/context)", source),
		}
	}

	// Explicit routing rules always win. They express user intent and must not
	// be overruled by price sorting.
	for _, e := range okList {
		if e.fromRule {
			return Decision{
				Source: source, TargetKey: e.key, TargetModel: e.m.ModelID, Provider: provider,
				Reason: "matched", FallbackUsed: e.i > 0, Attempt: e.i + 1,
			}, nil
		}
	}

	pick := okList[0]
	if routing.PreferCheapest && len(okList) > 1 {
		best := pick
		bestScore := priceScore(best.m)
		for _, e := range okList[1:] {
			s := priceScore(e.m)
			if s < bestScore {
				best, bestScore = e, s
			}
		}
		pick = best
	}
	return Decision{
		Source: source, TargetKey: pick.key, TargetModel: pick.m.ModelID, Provider: provider,
		Reason: "matched", FallbackUsed: pick.i > 0, Attempt: pick.i + 1,
	}, nil
}

func (r *Registry) candidates(source string, routing config.Routing) []string {
	var out []string
	for _, rule := range routing.Rules {
		if rule.Source == source {
			out = append(out, rule.TargetModel)
		}
	}
	// Exact provider model ID or desktop_id passthrough.
	for key, m := range r.Models {
		if m.Enabled && (m.ModelID == source || key == source || m.DesktopID == source) {
			out = append(out, key)
		}
	}
	// tier match: source equals a tier alias
	for key, m := range r.Models {
		if m.TierAlias == source && m.Enabled {
			out = append(out, key)
		}
	}
	if routing.DefaultTier != "" {
		for key, m := range r.Models {
			if m.TierAlias == routing.DefaultTier {
				out = append(out, key)
			}
		}
	}
	for _, ft := range routing.FallbackTiers {
		for key, m := range r.Models {
			if m.TierAlias == ft {
				out = append(out, key)
			}
		}
	}
	// de-dupe preserve order
	seen := map[string]bool{}
	var uniq []string
	for _, k := range out {
		if seen[k] {
			continue
		}
		seen[k] = true
		uniq = append(uniq, k)
	}
	return uniq
}

func modelCaps(m config.Model) api.Capabilities {
	to := func(b bool) api.Cap {
		if b {
			return api.CapSupported
		}
		return api.CapUnsupported
	}
	return api.Capabilities{
		Streaming: to(m.Streaming), Tools: to(m.ToolCalls), Vision: to(m.Vision),
		Reasoning: to(m.Reasoning), Context: m.ContextLimit,
	}
}
