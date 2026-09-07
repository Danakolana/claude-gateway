package config

import (
	"github.com/danakolana/claude-gateway/internal/secrets"
)

// File is the top-level TOML configuration document.
type File struct {
	Version       int                 `toml:"version"`
	ActiveProfile string              `toml:"active_profile"`
	Profiles      map[string]Profile  `toml:"profiles"`
	Providers     map[string]Provider `toml:"providers"`
	Models        map[string]Model    `toml:"models"`
	History       History             `toml:"history"`
	Sync          Sync                `toml:"sync"`
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
