package routing

import (
	"strings"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/pkg/api"
)

var ephemeralCache = map[string]any{"type": "ephemeral"}

// ApplyCostPolicies mutates req for thinking, prompt-cache injection, and
// OpenRouter provider preferences after a routing Decision is known.
func ApplyCostPolicies(req *api.Request, m config.Model, routing config.Routing, prov config.Provider) {
	if req == nil {
		return
	}
	applyThinkingPolicy(req, m)
	if routing.EnsurePromptCache {
		EnsurePromptCache(req)
	}
	if prefs := BuildUpstreamProvider(prov); len(prefs) > 0 {
		req.UpstreamProvider = prefs
	}
}

func applyThinkingPolicy(req *api.Request, m config.Model) {
	policy := strings.ToLower(strings.TrimSpace(m.ThinkingPolicy))
	if policy == "" {
		if !m.Reasoning {
			policy = "force_off"
		} else {
			policy = "passthrough"
		}
	}
	switch policy {
	case "force_off", "off", "disabled":
		req.Thinking = &api.ThinkingConfig{Type: "disabled"}
	case "cap":
		max := m.ThinkingBudgetMax
		if max <= 0 {
			max = 1024
		}
		if req.Thinking == nil {
			req.Thinking = &api.ThinkingConfig{Type: "enabled", BudgetTokens: max}
			return
		}
		t := strings.ToLower(strings.TrimSpace(req.Thinking.Type))
		if t == "" || t == "disabled" || t == "disabled_thinking" || t == "none" {
			req.Thinking = &api.ThinkingConfig{Type: "disabled"}
			return
		}
		if req.Thinking.BudgetTokens <= 0 || req.Thinking.BudgetTokens > max {
			req.Thinking.BudgetTokens = max
		}
	case "passthrough":
		// leave client thinking as-is
	default:
		// unknown → treat like passthrough unless reasoning disabled
		if !m.Reasoning {
			req.Thinking = &api.ThinkingConfig{Type: "disabled"}
		}
	}
}

// EnsurePromptCache adds ephemeral cache_control on system blocks and tools
// when the client did not set any. Does not rewrite message history prefixes.
func EnsurePromptCache(req *api.Request) {
	if req == nil {
		return
	}
	if hasAnyCacheControl(*req) {
		return
	}
	if len(req.SystemBlocks) > 0 {
		for i := range req.SystemBlocks {
			if req.SystemBlocks[i].Type == api.BlockText && len(req.SystemBlocks[i].CacheControl) == 0 {
				req.SystemBlocks[i].CacheControl = cloneCache(ephemeralCache)
			}
		}
	} else if req.System != "" {
		req.SystemBlocks = []api.ContentBlock{{
			Type: api.BlockText, Text: req.System, CacheControl: cloneCache(ephemeralCache),
		}}
		req.System = ""
	}
	for i := range req.Tools {
		if len(req.Tools[i].CacheControl) == 0 {
			req.Tools[i].CacheControl = cloneCache(ephemeralCache)
		}
	}
	if len(req.CacheControl) == 0 && (len(req.SystemBlocks) > 0 || len(req.Tools) > 0) {
		req.CacheControl = cloneCache(ephemeralCache)
	}
}

func hasAnyCacheControl(req api.Request) bool {
	if len(req.CacheControl) > 0 {
		return true
	}
	for _, b := range req.SystemBlocks {
		if len(b.CacheControl) > 0 {
			return true
		}
	}
	for _, t := range req.Tools {
		if len(t.CacheControl) > 0 {
			return true
		}
	}
	for _, m := range req.Messages {
		for _, b := range m.Content {
			if len(b.CacheControl) > 0 {
				return true
			}
		}
	}
	return false
}

func cloneCache(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// BuildUpstreamProvider maps provider TOML into an OpenRouter provider object.
func BuildUpstreamProvider(p config.Provider) map[string]any {
	out := map[string]any{}
	if s := strings.ToLower(strings.TrimSpace(p.Sort)); s != "" {
		out["sort"] = s
	}
	if len(p.ProviderOrder) > 0 {
		out["order"] = append([]string(nil), p.ProviderOrder...)
	}
	if len(p.IgnoreProviders) > 0 {
		out["ignore"] = append([]string(nil), p.IgnoreProviders...)
	}
	if p.RequireParameters != nil {
		out["require_parameters"] = *p.RequireParameters
	}
	if p.AllowFallbacks != nil {
		out["allow_fallbacks"] = *p.AllowFallbacks
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// priceScore ranks models for prefer_cheapest (lower is better).
func priceScore(m config.Model) float64 {
	in, out := m.InputPrice, m.OutputPrice
	if in <= 0 && out <= 0 {
		return 1e12 // unknown → last
	}
	score := in
	if out/5 > score {
		score = out / 5
	}
	return score
}
