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
}

// DesktopPickerEntry is derived for Claude Desktop inferenceModels.
type DesktopPickerEntry struct {
	Key          string
	DesktopID    string
	DesktopLabel string
	DesktopTier string
	ModelID      string
	IsDefault    bool
	ToolCalls    bool
	Vision       bool
	Reasoning    bool
	ContextLimit int
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
// First model per family tier is marked IsDefault.
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
		})
	}
	sort.Slice(all, func(i, j int) bool {
		ri, rj := tierRank(all[i].DesktopTier), tierRank(all[j].DesktopTier)
		if ri != rj {
			return ri < rj
		}
		// Prefer explicit desktop_default within a tier.
		if all[i].IsDefault != all[j].IsDefault {
			return all[i].IsDefault
		}
		// Prefer shorter / base Claude IDs as family defaults (claude-sonnet-4 before -4-5).
		if all[i].DesktopID != all[j].DesktopID {
			return all[i].DesktopID < all[j].DesktopID
		}
		return all[i].DesktopLabel < all[j].DesktopLabel
	})
	seenTier := map[string]bool{}
	for i := range all {
		if seenTier[all[i].DesktopTier] {
			all[i].IsDefault = false
			continue
		}
		// Sorted with desktop_default first within each tier.
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
