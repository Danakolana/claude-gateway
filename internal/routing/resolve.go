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
	candidates := r.candidates(source, routing)
	if len(candidates) == 0 {
		return Decision{}, &api.Error{Category: api.ErrUnsupportedCapability, Message: "no routing candidate for " + source}
	}
	for i, key := range candidates {
		m, ok := r.Models[key]
		if !ok {
			continue
		}
		if !m.Enabled {
			continue
		}
		caps := modelCaps(m)
		req2 := req
		if estimatedTokens > 0 {
			req2.MinContext = estimatedTokens
		}
		if !caps.Compatible(req2) {
			if i == len(candidates)-1 {
				return Decision{}, &api.Error{
					Category: api.ErrUnsupportedCapability,
					Message:  fmt.Sprintf("model %q cannot satisfy required capabilities/context", key),
				}
			}
			continue
		}
		return Decision{
			Source: source, TargetKey: key, TargetModel: m.ModelID, Provider: provider,
			Reason: "matched", FallbackUsed: i > 0, Attempt: i + 1,
		}, nil
	}
	return Decision{}, &api.Error{Category: api.ErrUnsupportedCapability, Message: "no eligible model"}
}

func (r *Registry) candidates(source string, routing config.Routing) []string {
	var out []string
	for _, rule := range routing.Rules {
		if rule.Source == source {
			out = append(out, rule.TargetModel)
		}
	}
	// Exact provider model ID passthrough (Desktop often sends inferenceModels IDs).
	for key, m := range r.Models {
		if m.Enabled && (m.ModelID == source || key == source) {
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
