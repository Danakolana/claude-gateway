package modelstatus

import (
	"fmt"
	"strings"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/pkg/api"
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
func EstimateCostUSD(u api.Usage, inPerMTok, outPerMTok float64) float64 {
	return (float64(u.InputTokens)/1e6)*inPerMTok + (float64(u.OutputTokens)/1e6)*outPerMTok
}

// FormatUsageLine renders a compact post-request usage/cost summary for the terminal.
func FormatUsageLine(sourceModel, targetModel string, u api.Usage, inPerMTok, outPerMTok float64, hasPrice bool) string {
	var b strings.Builder
	b.WriteString("── usage ──────────────────────────────────────────────────────\n")
	route := targetModel
	if sourceModel != "" && sourceModel != targetModel {
		route = fmt.Sprintf("%s → %s", sourceModel, targetModel)
	}
	b.WriteString(fmt.Sprintf("  model    %s\n", route))

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
		b.WriteString(fmt.Sprintf("  ~cost    $%.6f  (est. from $%.2f/$%.2f per 1MTok; approximate)\n",
			est, inPerMTok, outPerMTok))
	default:
		b.WriteString("  ~cost    n/a  (no price for this model yet)\n")
	}
	b.WriteString("────────────────────────────────────────────────────────────────")
	return b.String()
}
