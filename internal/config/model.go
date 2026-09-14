package config

import (
	"sort"
	"strings"

	"github.com/danakolana/claude-gateway/internal/secrets"
)

// File is the top-level TOML configuration document.
type File struct {
	Version       int                 `toml:"version"`
	ActiveProfile string              `toml:"active_profile"`
	Proxy         Proxy               `toml:"proxy"`
	Client        Client              `toml:"client"`
	Profiles      map[string]Profile  `toml:"profiles"`
	Providers     map[string]Provider `toml:"providers"`
	Models        map[string]Model    `toml:"models"`
	History       History             `toml:"history"`
	Sync          Sync                `toml:"sync"`
}

// Client selects which Claude Desktop product to configure (usually set by
// interactive prompt at start / client apply — not via CLI flags).
type Client struct {
	// Desktop is "3p" (default, recommended) or "consumer" (experimental).
	Desktop string `toml:"desktop"`
	// AllowExperimental is set true when the user confirms consumer apply
	// in the interactive prompt.
	AllowExperimental bool `toml:"allow_experimental"`
}

// IsConsumer reports whether regular (non-3P) Claude Desktop is the apply target.
func (c Client) IsConsumer() bool {
	switch strings.ToLower(strings.TrimSpace(c.Desktop)) {
	case "consumer", "stable", "regular", "main":
		return true
	default:
		return false
	}
}

// AllowsExperimental reports whether experimental consumer apply is permitted by TOML.
func (c Client) AllowsExperimental() bool {
	return c.AllowExperimental
}

// DesktopTarget returns "3p" or "consumer".
func (c Client) DesktopTarget() string {
	if c.IsConsumer() {
		return "consumer"
	}
	return "3p"
}

// Proxy is the local Anthropic-compatible listen settings.
type Proxy struct {
	// Mode selects how Desktop reaches models:
	//   "local"  (default) — Desktop → local proxy → OpenRouter/9router
	//   "direct" — Desktop → OpenRouter Anthropic API (no local proxy)
	Mode string `toml:"mode"`
	// Listen is host:port for the local proxy (default 127.0.0.1:8080).
	Listen string `toml:"listen"`
	// DirectBaseURL overrides the upstream Anthropic-compatible gateway URL in
	// direct mode (default: provider base_url with trailing /v1 stripped,
	// e.g. https://openrouter.ai/api).
	DirectBaseURL string `toml:"direct_base_url"`
	// ApplyDesktop, when true/omitted, writes Claude Desktop on 3P config on start.
	ApplyDesktop *bool `toml:"apply_desktop"`
	// OpenGuide, when true/omitted, opens the local bilingual guide in a browser
	// after the proxy starts (local mode).
	OpenGuide *bool `toml:"open_guide"`
	// InspectPrompts, when true, keeps recent system/tool harness snapshots for
	// the local guide at /debug/harness (off by default).
	InspectPrompts bool `toml:"inspect_prompts"`
	// SpendAlertUSD, when > 0, fires a one-shot webhook after this-process
	// estimated spend crosses the threshold. 0 (default) disables alerts.
	SpendAlertUSD float64 `toml:"spend_alert_usd"`
	// SpendAlertURL is an ntfy/webhook URL. Empty (default) disables alerts.
	SpendAlertURL string `toml:"spend_alert_url"`
}

// IsDirect reports whether Desktop should call the upstream Anthropic API
// without the local proxy.
func (p Proxy) IsDirect() bool {
	switch strings.ToLower(strings.TrimSpace(p.Mode)) {
	case "direct", "upstream", "openrouter-direct":
		return true
	default:
		return false
	}
}

// Addr returns the listen address with a safe default.
func (p Proxy) Addr() string {
	if strings.TrimSpace(p.Listen) != "" {
		return strings.TrimSpace(p.Listen)
	}
	return "127.0.0.1:8080"
}

// BaseURL is the local proxy HTTP base URL.
func (p Proxy) BaseURL() string {
	return "http://" + p.Addr()
}

// ShouldApplyDesktop defaults to true when unset.
func (p Proxy) ShouldApplyDesktop() bool {
	if p.ApplyDesktop == nil {
		return true
	}
	return *p.ApplyDesktop
}

// ShouldOpenGuide defaults to true when unset.
func (p Proxy) ShouldOpenGuide() bool {
	if p.OpenGuide == nil {
		return true
	}
	return *p.OpenGuide
}

// DirectGatewayBaseURL is the Anthropic-compatible base URL for Desktop in
// direct mode (OpenRouter expects https://openrouter.ai/api, not .../api/v1).
func DirectGatewayBaseURL(p Proxy, prov Provider) string {
	if u := strings.TrimSpace(p.DirectBaseURL); u != "" {
		return strings.TrimRight(u, "/")
	}
	u := strings.TrimRight(strings.TrimSpace(prov.BaseURL), "/")
	u = strings.TrimSuffix(u, "/v1")
	if u == "" {
		return "https://openrouter.ai/api"
	}
	return u
}

// Profile selects provider, routing, and modes.
type Profile struct {
	Provider    string  `toml:"provider"`
	ProxyMode   string  `toml:"proxy_mode"`
	HistoryMode string  `toml:"history_mode"`
	Routing     Routing `toml:"routing"`
}

// Provider is an upstream gateway profile.
type Provider struct {
	BaseURL             string            `toml:"base_url"`
	Protocol            string            `toml:"protocol"`
	APIKeyEnv           string            `toml:"api_key_env"`
	APIKeyInline        string            `toml:"api_key"` // discouraged
	AuthScheme          string            `toml:"auth_scheme"`
	TimeoutSeconds      int               `toml:"timeout_seconds"`
	TLSVerify           *bool             `toml:"tls_verify"`
	Headers             map[string]string `toml:"headers"`
	HealthPath          string            `toml:"health_path"`
	AllowPrivateNetwork bool              `toml:"allow_private_network"`
	// OpenRouter-style provider routing (optional; ignored by plain OpenAI gateways).
	Sort              string   `toml:"sort"`               // price | throughput | latency
	ProviderOrder     []string `toml:"provider_order"`     // preferred backend names
	IgnoreProviders   []string `toml:"ignore_providers"`   // backends to skip
	RequireParameters *bool    `toml:"require_parameters"` // require endpoints that honor request params
	AllowFallbacks    *bool    `toml:"allow_fallbacks"`
	// Sticky pins the last successful OpenRouter backend per model so
	// prompt-cache reads are not lost when sort=price hops providers.
	Sticky *bool `toml:"sticky"`
}

// APIKeyHandle returns a secret handle for the provider key.
func (p Provider) APIKeyHandle() secrets.Handle {
	if p.APIKeyEnv != "" {
		return secrets.Handle{Kind: "env", Ref: p.APIKeyEnv}
	}
	return secrets.Handle{}
}

// Model is a provider-facing model record.
type Model struct {
	ModelID      string `toml:"model_id"`
	DisplayName  string `toml:"display_name"`
	TierAlias    string `toml:"tier_alias"`
	Streaming    bool   `toml:"streaming"`
	ToolCalls    bool   `toml:"tool_calls"`
	Vision       bool   `toml:"vision"`
	Reasoning    bool   `toml:"reasoning"`
	Enabled      bool   `toml:"enabled"`
	ContextLimit int    `toml:"context_limit"`
	// Desktop picker: Anthropic-looking ID required by Claude Desktop on 3P.
	DesktopID      string  `toml:"desktop_id"`
	DesktopLabel   string  `toml:"desktop_label"`
	DesktopTier    string  `toml:"desktop_tier"` // haiku | sonnet | opus
	DesktopDefault bool    `toml:"desktop_default"`
	InputPrice     float64 `toml:"input_price_per_mtok"`
	OutputPrice    float64 `toml:"output_price_per_mtok"`
	Notes          string  `toml:"notes"`
	// ThinkingPolicy: "" (default), "passthrough", "force_off", "cap".
	// Empty + Reasoning=false → force_off. Cap uses ThinkingBudgetMax.
	ThinkingPolicy    string `toml:"thinking_policy"`
	ThinkingBudgetMax int    `toml:"thinking_budget_max"`
	// MaxTokensCap clamps request max_tokens when > 0. Overrides routing cap.
	MaxTokensCap int `toml:"max_tokens_cap"`
}

// DesktopPickerEntry is derived for Claude Desktop inferenceModels.
type DesktopPickerEntry struct {
	Key          string
	DesktopID    string
	DesktopLabel string
	DesktopTier  string
	ModelID      string
	IsDefault    bool
	ToolCalls    bool
	Vision       bool
	Reasoning    bool
	ContextLimit int
	InputPrice   float64 // config.toml input $/MTok (0 = unknown)
	OutputPrice  float64
}

func tierRank(tier string) int {
	switch tier {
	case "haiku":
		return 0
	case "sonnet":
		return 1
	case "opus":
		return 2
	default:
		return 3
	}
}

// DesktopPickerEntries returns enabled models that have a desktop_id.
// Sorted cheapest input price first (unknown prices last). First model per
// family tier is marked IsDefault (Claude Desktop keyboard shortcuts 1–9
// follow this list order).
func DesktopPickerEntries(models map[string]Model) []DesktopPickerEntry {
	var all []DesktopPickerEntry
	for key, m := range models {
		if !m.Enabled || m.DesktopID == "" {
			continue
		}
		tier := m.DesktopTier
		if tier == "" {
			tier = "sonnet"
		}
		label := m.DesktopLabel
		if label == "" {
			label = m.DisplayName
		}
		if label == "" {
			label = m.DesktopID
		}
		all = append(all, DesktopPickerEntry{
			Key: key, DesktopID: m.DesktopID, DesktopLabel: label,
			DesktopTier: tier, ModelID: m.ModelID,
			ToolCalls: m.ToolCalls, Vision: m.Vision, Reasoning: m.Reasoning,
			ContextLimit: m.ContextLimit, IsDefault: m.DesktopDefault,
			InputPrice: m.InputPrice, OutputPrice: m.OutputPrice,
		})
	}
	sort.SliceStable(all, func(i, j int) bool {
		pi, pj := all[i].InputPrice, all[j].InputPrice
		if pi <= 0 && pj > 0 {
			return false
		}
		if pj <= 0 && pi > 0 {
			return true
		}
		if pi != pj {
			return pi < pj
		}
		// Prefer explicit desktop_default when prices tie.
		if all[i].IsDefault != all[j].IsDefault {
			return all[i].IsDefault
		}
		ri, rj := tierRank(all[i].DesktopTier), tierRank(all[j].DesktopTier)
		if ri != rj {
			return ri < rj
		}
		if all[i].DesktopLabel != all[j].DesktopLabel {
			return all[i].DesktopLabel < all[j].DesktopLabel
		}
		return all[i].DesktopID < all[j].DesktopID
	})
	seenTier := map[string]bool{}
	for i := range all {
		if seenTier[all[i].DesktopTier] {
			all[i].IsDefault = false
			continue
		}
		all[i].IsDefault = true
		seenTier[all[i].DesktopTier] = true
	}
	return all
}

// LooksLikeAnthropicModelRoute reports whether Claude Desktop accepts id as an
// inferenceModels route name (claude-* or anthropic/claude-*). Non-matching
// IDs are stripped from the picker (see Desktop logs).
func LooksLikeAnthropicModelRoute(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return false
	}
	return strings.HasPrefix(id, "anthropic/claude-") || strings.HasPrefix(id, "claude-")
}

// Routing holds replacement rules.
type Routing struct {
	DefaultTier   string   `toml:"default_tier"`
	FallbackTiers []string `toml:"fallback_tiers"`
	Rules         []Rule   `toml:"rules"`
	// PreferCheapest picks the lowest list-price eligible candidate.
	PreferCheapest bool `toml:"prefer_cheapest"`
	// EnsurePromptCache injects cache_control on system + tools when missing,
	// and on the last stable history block (conversation prefix).
	EnsurePromptCache bool `toml:"ensure_prompt_cache"`
	// MaxTokensCap clamps request max_tokens when > 0. 0 = unlimited.
	// Per-model max_tokens_cap wins when set.
	MaxTokensCap int `toml:"max_tokens_cap"`
	// CircuitFailures is how many transient upstream failures open a skip
	// for that model key. 0 (default) disables the breaker (ADR-014).
	CircuitFailures int `toml:"circuit_failures"`
	// CircuitCooldownSeconds is how long a tripped key is skipped when a
	// fallback exists. 0 uses 30s.
	CircuitCooldownSeconds int `toml:"circuit_cooldown_seconds"`
}

// Rule maps a source model/tier to a target model key.
type Rule struct {
	Source      string `toml:"source"`
	TargetModel string `toml:"target_model"`
}

// History local store settings.
type History struct {
	LocalDatabase       string `toml:"local_database"`
	AttachmentDirectory string `toml:"attachment_directory"`
	RedactSecrets       bool   `toml:"redact_secrets"`
	RetentionDays       int    `toml:"retention_days"`
}

// Sync remote settings.
type Sync struct {
	Enabled   bool   `toml:"enabled"`
	ServerURL string `toml:"server_url"`
	TokenEnv  string `toml:"token_env"`
	Token     string `toml:"token"` // discouraged inline
}

// Effective is a resolved, non-secret view of the active profile.
type Effective struct {
	SourcePath    string
	ActiveProfile string
	ProviderName  string
	Provider      Provider
	Routing       Routing
	Models        map[string]Model
	History       History
	Sync          Sync
	Origins       map[string]string
}
