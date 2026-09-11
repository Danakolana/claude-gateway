package routing

import (
	"strings"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/pkg/api"
)

var ephemeralCache = map[string]any{"type": "ephemeral"}

// ApplyCostPolicies mutates req for thinking, max_tokens, prompt-cache
// injection, and OpenRouter provider preferences after a routing Decision.
func ApplyCostPolicies(req *api.Request, m config.Model, routing config.Routing, prov config.Provider) {
	if req == nil {
		return
	}
	applyThinkingPolicy(req, m)
	applyMaxTokensCap(req, m, routing)
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

func applyMaxTokensCap(req *api.Request, m config.Model, routing config.Routing) {
	cap := m.MaxTokensCap
	if cap <= 0 {
		cap = routing.MaxTokensCap
	}
	if cap > 0 && req.MaxTokens > cap {
		req.MaxTokens = cap
	}
	if req.Thinking != nil && req.Thinking.BudgetTokens > 0 && req.MaxTokens > 0 && req.Thinking.BudgetTokens > req.MaxTokens {
		req.Thinking.BudgetTokens = req.MaxTokens
	}
}

// EnsurePromptCache adds ephemeral cache_control on system blocks and tools
// when the client did not set any there, then marks the last stable history
// block so multi-turn prefixes can be cached. Does not drop or rewrite text.
func EnsurePromptCache(req *api.Request) {
	if req == nil {
		return
	}
	if !hasSystemOrToolCache(*req) {
		injectSystemToolCache(req)
	}
	markConversationPrefix(req)
}

func injectSystemToolCache(req *api.Request) {
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

// markConversationPrefix puts cache_control on the last cacheable block of
// the penultimate message (Anthropic multi-turn prefix). The newest turn
// stays uncached. No-op when there is no prior turn or the client already
// marked a message block.
func markConversationPrefix(req *api.Request) {
	if messagesHaveCache(*req) || len(req.Messages) < 2 {
		return
	}
	for i := len(req.Messages) - 2; i >= 0; i-- {
		content := req.Messages[i].Content
		for j := len(content) - 1; j >= 0; j-- {
			switch content[j].Type {
			case api.BlockText, api.BlockImage, api.BlockToolResult:
				if len(content[j].CacheControl) == 0 {
					req.Messages[i].Content[j].CacheControl = cloneCache(ephemeralCache)
				}
				return
			}
		}
	}
}

func hasSystemOrToolCache(req api.Request) bool {
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
	return false
}

func messagesHaveCache(req api.Request) bool {
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

func stickyEnabled(p config.Provider) bool {
	return p.Sticky != nil && *p.Sticky
}

// ApplyStickyPin prepends pin to OpenRouter provider.order so the next
// request prefers the last successful backend.
func ApplyStickyPin(req *api.Request, pin string) {
	if req == nil || pin == "" {
		return
	}
	req.UpstreamProvider = mergeProviderOrder(req.UpstreamProvider, pin)
}

func mergeProviderOrder(prefs map[string]any, pin string) map[string]any {
	if pin == "" {
		return prefs
	}
	if prefs == nil {
		prefs = map[string]any{}
	}
	var rest []string
	switch existing := prefs["order"].(type) {
	case []string:
		rest = existing
	case []any:
		for _, v := range existing {
			if s, ok := v.(string); ok && s != "" {
				rest = append(rest, s)
			}
		}
	}
	order := []string{pin}
	for _, s := range rest {
		if s != pin {
			order = append(order, s)
		}
	}
	prefs["order"] = order
	return prefs
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
