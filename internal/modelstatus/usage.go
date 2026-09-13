package modelstatus

import (
	"fmt"
	"strings"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/pkg/api"
)

// Default Anthropic-like cache multipliers vs normal input $/MTok.
// OpenRouter may differ; prefer Usage.HasProviderCost when present.
const (
	DefaultCacheReadMult  = 0.10
	DefaultCacheWriteMult = 1.25
	// Warn when prompt is large but no cache read was reported.
	CacheMissWarnInputTokens = 4000
)

// PricesForModelID returns config.toml $/MTok for an upstream model_id.
func PricesForModelID(models map[string]config.Model, modelID string) (in, out float64, ok bool) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return 0, 0, false
	}
	for _, m := range models {
		if m.ModelID == modelID {
			if m.InputPrice > 0 || m.OutputPrice > 0 {
				return m.InputPrice, m.OutputPrice, true
			}
			return 0, 0, false
		}
	}
	return 0, 0, false
}

// EstimateCostUSD estimates USD from token counts and $/MTok rates.
// Reasoning tokens are already included in OutputTokens by OpenRouter.
// When cache_read / cache_write are present, applies DefaultCache* multipliers
// (OpenRouter-style: cached_tokens are a subset of prompt_tokens).
func EstimateCostUSD(u api.Usage, inPerMTok, outPerMTok float64) float64 {
	return EstimateCostUSDWithCache(u, inPerMTok, outPerMTok, DefaultCacheReadMult, DefaultCacheWriteMult)
}

// EstimateCostUSDWithCache applies explicit cache read/write multipliers vs input price.
func EstimateCostUSDWithCache(u api.Usage, inPerMTok, outPerMTok, cacheReadMult, cacheWriteMult float64) float64 {
	if cacheReadMult <= 0 {
		cacheReadMult = DefaultCacheReadMult
	}
	if cacheWriteMult <= 0 {
		cacheWriteMult = DefaultCacheWriteMult
	}
	outCost := (float64(u.OutputTokens) / 1e6) * outPerMTok
	if u.CachedTokens <= 0 && u.CacheWriteTokens <= 0 {
		return (float64(u.InputTokens)/1e6)*inPerMTok + outCost
	}
	cached := u.CachedTokens
	write := u.CacheWriteTokens
	rest := u.InputTokens - cached
	if rest < 0 {
		rest = 0
	}
	// Treat write tokens as part of rest when reported separately; add write premium only.
	inCost := (float64(rest)/1e6)*inPerMTok +
		(float64(cached)/1e6)*inPerMTok*cacheReadMult
	if write > 0 {
		inCost += (float64(write) / 1e6) * inPerMTok * (cacheWriteMult - 1)
	}
	return inCost + outCost
}

// FormatUsageLine renders a compact post-request usage/cost summary for the terminal.
// The primary model line is the OpenRouter (upstream) id; the Desktop/Anthropic
// alias is shown only as a secondary hint when it differs.
func FormatUsageLine(sourceModel, targetModel string, u api.Usage, inPerMTok, outPerMTok float64, hasPrice bool) string {
	var b strings.Builder
	b.WriteString("── usage ──────────────────────────────────────────────────────\n")
	model := strings.TrimSpace(targetModel)
	if model == "" {
		model = strings.TrimSpace(sourceModel)
	}
	if model == "" {
		model = "(unknown)"
	}
	b.WriteString(fmt.Sprintf("  model    %s\n", model))
	src := strings.TrimSpace(sourceModel)
	if src != "" && model != src {
		b.WriteString(fmt.Sprintf("  desktop  %s\n", src))
	}

	promptExtra := ""
	parts := []string{}
	if u.CachedTokens > 0 {
		parts = append(parts, fmt.Sprintf("cache_read %d", u.CachedTokens))
	}
	if u.CacheWriteTokens > 0 {
		parts = append(parts, fmt.Sprintf("cache_write %d", u.CacheWriteTokens))
	}
	if len(parts) > 0 {
		promptExtra = " (" + strings.Join(parts, ", ") + ")"
	}
	b.WriteString(fmt.Sprintf("  prompt   %d tok%s\n", u.InputTokens, promptExtra))

	outExtra := ""
	if u.ReasoningTokens > 0 {
		completion := u.OutputTokens - u.ReasoningTokens
		if completion < 0 {
			completion = 0
		}
		outExtra = fmt.Sprintf(" (reasoning %d + completion ~%d)", u.ReasoningTokens, completion)
	}
	b.WriteString(fmt.Sprintf("  output   %d tok%s\n", u.OutputTokens, outExtra))

	switch {
	case u.HasProviderCost:
		b.WriteString(fmt.Sprintf("  ~cost    $%.6f  (OpenRouter reported; approximate)\n", u.ProviderCostUSD))
	case hasPrice && (u.InputTokens > 0 || u.OutputTokens > 0):
		est := EstimateCostUSD(u, inPerMTok, outPerMTok)
		note := "est."
		if u.CachedTokens > 0 || u.CacheWriteTokens > 0 {
			note = "est. cache-aware"
		}
		b.WriteString(fmt.Sprintf("  ~cost    $%.6f  (%s from $%.2f/$%.2f per 1MTok; approximate)\n",
			est, note, inPerMTok, outPerMTok))
	default:
		b.WriteString("  ~cost    n/a  (no price for this model yet)\n")
	}
	if u.InputTokens >= CacheMissWarnInputTokens && u.CachedTokens == 0 && u.CacheWriteTokens == 0 {
		b.WriteString("  note     large prompt with no cache_read — upstream may not support prompt cache\n")
	}
	if u.ReasoningTokens > 0 && u.OutputTokens > 0 && u.ReasoningTokens*2 >= u.OutputTokens {
		b.WriteString("  note     reasoning ≥50% of output — consider thinking_policy=force_off or cap\n")
	}
	b.WriteString("────────────────────────────────────────────────────────────────")
	return b.String()
}

// FormatTurnTotal renders a rolled-up usage/cost summary for one Desktop/agent
// turn (many /v1/messages calls that end when the model stops with end_turn).
func FormatTurnTotal(requests, inputTokens, outputTokens int, estUSD float64, hasCost bool) string {
	var b strings.Builder
	b.WriteString("── turn total ─────────────────────────────────────────────────\n")
	b.WriteString(fmt.Sprintf("  requests %d\n", requests))
	b.WriteString(fmt.Sprintf("  prompt   %d tok\n", inputTokens))
	b.WriteString(fmt.Sprintf("  output   %d tok\n", outputTokens))
	if hasCost {
		b.WriteString(fmt.Sprintf("  ~cost    $%.6f  (sum of this turn; approximate)\n", estUSD))
	} else {
		b.WriteString("  ~cost    n/a\n")
	}
	b.WriteString("────────────────────────────────────────────────────────────────")
	return b.String()
}

const (
	CostNone     = "none"
	CostProvider = "provider"
	CostEstimate = "estimate"
)

// CostUSD returns a USD figure and how it was produced. Never errors.
func CostUSD(u api.Usage, inPerMTok, outPerMTok float64, hasPrice bool) (usd float64, source string) {
	switch {
	case u.HasProviderCost:
		return u.ProviderCostUSD, CostProvider
	case hasPrice && (u.InputTokens > 0 || u.OutputTokens > 0):
		return EstimateCostUSD(u, inPerMTok, outPerMTok), CostEstimate
	default:
		return 0, CostNone
	}
}

// AdvisoryNotes are fail-open spend nags. They must never reject a request.
func AdvisoryNotes(u api.Usage) []string {
	var notes []string
	if u.InputTokens >= CacheMissWarnInputTokens && u.CachedTokens == 0 && u.CacheWriteTokens == 0 {
		notes = append(notes, "large prompt with no cache_read — upstream may not support prompt cache")
	}
	if u.ReasoningTokens > 0 && u.OutputTokens > 0 && u.ReasoningTokens*2 >= u.OutputTokens {
		notes = append(notes, "reasoning ≥50% of output — consider thinking_policy=force_off or cap")
	}
	return notes
}

// ContextWarnRatio is the advisory threshold vs declared context_limit.
const ContextWarnRatio = 0.80

// ContextNotes warns when the prompt is near the selected model's context
// window. It never truncates or rejects (FR-SIDECAR-005 / FR-PROXY-011).
func ContextNotes(req api.Request, u api.Usage, contextLimit int) []string {
	if contextLimit <= 0 {
		return nil
	}
	n := u.InputTokens
	if n <= 0 {
		n = CoarseTokenEstimate(req)
	}
	if n <= 0 {
		return nil
	}
	if float64(n) < float64(contextLimit)*ContextWarnRatio {
		return nil
	}
	return []string{"prompt is near the model's context limit; start a new chat — gateway will not truncate"}
}

// CoarseTokenEstimate is chars/4 when the client did not send a token count.
func CoarseTokenEstimate(req api.Request) int {
	if req.EstimatedTokens > 0 {
		return req.EstimatedTokens
	}
	n := len(req.System)
	for _, b := range req.SystemBlocks {
		n += len(b.Text)
	}
	for _, m := range req.Messages {
		for _, b := range m.Content {
			n += len(b.Text)
		}
	}
	if n <= 0 {
		return 0
	}
	return n / 4
}
