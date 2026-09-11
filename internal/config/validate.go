package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/danakolana/claude-gateway/internal/security"
)

// ValidationError is an actionable configuration error.
type ValidationError struct {
	Field       string
	Reason      string
	Remediation string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s (%s)", e.Field, e.Reason, e.Remediation)
}

// Validate reports all actionable errors.
func Validate(f *File) []ValidationError {
	var errs []ValidationError
	if f.Version != 0 && f.Version != 1 {
		errs = append(errs, ValidationError{
			Field: "version", Reason: fmt.Sprintf("unsupported version %d", f.Version),
			Remediation: "set version = 1",
		})
	}
	if f.ActiveProfile == "" {
		errs = append(errs, ValidationError{
			Field: "active_profile", Reason: "missing active profile",
			Remediation: "set active_profile or run profile select",
		})
	} else if _, ok := f.Profiles[f.ActiveProfile]; !ok {
		errs = append(errs, ValidationError{
			Field: "active_profile", Reason: fmt.Sprintf("profile %q not found", f.ActiveProfile),
			Remediation: "create the profile or select an existing one",
		})
	}

	for name, p := range f.Providers {
		prefix := "providers." + name
		if strings.TrimSpace(p.BaseURL) == "" {
			errs = append(errs, ValidationError{
				Field: prefix + ".base_url", Reason: "missing base URL",
				Remediation: "set an https:// URL for the provider",
			})
		} else if verr := validateProviderURL(prefix+".base_url", p.BaseURL, p.AllowPrivateNetwork); verr != nil {
			errs = append(errs, *verr)
		}
		if p.APIKeyEnv == "" && p.APIKeyInline != "" {
			errs = append(errs, ValidationError{
				Field: prefix + ".api_key", Reason: "inline API key is discouraged",
				Remediation: "use api_key_env and store the secret in the environment",
			})
		}
		if p.APIKeyEnv == "" && p.APIKeyInline == "" {
			errs = append(errs, ValidationError{
				Field: prefix + ".api_key_env", Reason: "missing API key reference",
				Remediation: "set api_key_env to an environment variable name",
			})
		}
		if p.TLSVerify != nil && !*p.TLSVerify {
			errs = append(errs, ValidationError{
				Field: prefix + ".tls_verify", Reason: "TLS verification disabled",
				Remediation: "set tls_verify = true unless you explicitly accept the risk",
			})
		}
	}

	if f.Sync.Token != "" && f.Sync.TokenEnv == "" {
		errs = append(errs, ValidationError{
			Field: "sync.token", Reason: "inline sync token is discouraged",
			Remediation: "use token_env and store the secret in the environment",
		})
	}
	if f.Sync.Enabled && f.Sync.ServerURL != "" {
		if verr := validateURL("sync.server_url", f.Sync.ServerURL); verr != nil {
			errs = append(errs, *verr)
		}
	}

	// Route cycle / missing target checks for active profile.
	if prof, ok := f.Profiles[f.ActiveProfile]; ok {
		seen := map[string]bool{}
		for i, r := range prof.Routing.Rules {
			field := fmt.Sprintf("profiles.%s.routing.rules[%d]", f.ActiveProfile, i)
			if r.Source == "" || r.TargetModel == "" {
				errs = append(errs, ValidationError{
					Field: field, Reason: "source and target_model are required",
					Remediation: "fill both fields",
				})
				continue
			}
			if _, ok := f.Models[r.TargetModel]; !ok {
				errs = append(errs, ValidationError{
					Field: field + ".target_model", Reason: fmt.Sprintf("model %q not found", r.TargetModel),
					Remediation: "define the model under [models.*]",
				})
			} else if m := f.Models[r.TargetModel]; !m.Enabled {
				errs = append(errs, ValidationError{
					Field: field + ".target_model", Reason: fmt.Sprintf("model %q is disabled", r.TargetModel),
					Remediation: "enable the model or choose another target",
				})
			}
			key := r.Source + "->" + r.TargetModel
			if seen[key] {
				errs = append(errs, ValidationError{
					Field: field, Reason: "duplicate routing rule",
					Remediation: "remove the duplicate",
				})
			}
			seen[key] = true
		}
		if _, ok := f.Providers[prof.Provider]; prof.Provider != "" && !ok {
			errs = append(errs, ValidationError{
				Field:       "profiles." + f.ActiveProfile + ".provider",
				Reason:      fmt.Sprintf("provider %q not found", prof.Provider),
				Remediation: "define the provider under [providers.*]",
			})
		}
	}

	// Duplicate model tier aliases among enabled models.
	tiers := map[string]string{}
	desktopIDs := map[string]string{}
	for name, m := range f.Models {
		if m.Enabled && m.DesktopID != "" {
			if prev, ok := desktopIDs[m.DesktopID]; ok {
				errs = append(errs, ValidationError{
					Field:       "models." + name + ".desktop_id",
					Reason:      fmt.Sprintf("duplicate desktop_id %q (also on %s)", m.DesktopID, prev),
					Remediation: "use a unique Anthropic-looking desktop_id per enabled model",
				})
			}
			desktopIDs[m.DesktopID] = name
		}
		if pol := strings.ToLower(strings.TrimSpace(m.ThinkingPolicy)); pol != "" {
			switch pol {
			case "passthrough", "force_off", "off", "disabled", "cap":
			default:
				errs = append(errs, ValidationError{
					Field:       "models." + name + ".thinking_policy",
					Reason:      fmt.Sprintf("unknown thinking_policy %q", m.ThinkingPolicy),
					Remediation: "use passthrough, force_off, or cap",
				})
			}
		}
		if m.MaxTokensCap < 0 {
			errs = append(errs, ValidationError{
				Field:       "models." + name + ".max_tokens_cap",
				Reason:      "max_tokens_cap must be >= 0",
				Remediation: "set a positive cap or omit / 0 for unlimited",
			})
		}
		if m.ThinkingPolicy != "" && strings.EqualFold(strings.TrimSpace(m.ThinkingPolicy), "cap") && m.ThinkingBudgetMax < 0 {
			errs = append(errs, ValidationError{
				Field:       "models." + name + ".thinking_budget_max",
				Reason:      "thinking_budget_max must be >= 0",
				Remediation: "set a positive budget or omit for default 1024",
			})
		}
		if m.TierAlias == "" || !m.Enabled {
			continue
		}
		if prev, ok := tiers[m.TierAlias]; ok {
			errs = append(errs, ValidationError{
				Field:       "models." + name + ".tier_alias",
				Reason:      fmt.Sprintf("duplicate tier alias %q (also on %s)", m.TierAlias, prev),
				Remediation: "use unique tier aliases for enabled models",
			})
		}
		tiers[m.TierAlias] = name
	}
	for name, p := range f.Providers {
		if s := strings.ToLower(strings.TrimSpace(p.Sort)); s != "" {
			switch s {
			case "price", "throughput", "latency":
			default:
				errs = append(errs, ValidationError{
					Field:       "providers." + name + ".sort",
					Reason:      fmt.Sprintf("unknown sort %q", p.Sort),
					Remediation: "use price, throughput, or latency",
				})
			}
		}
	}
	for name, prof := range f.Profiles {
		if prof.Routing.MaxTokensCap < 0 {
			errs = append(errs, ValidationError{
				Field:       "profiles." + name + ".routing.max_tokens_cap",
				Reason:      "max_tokens_cap must be >= 0",
				Remediation: "set a positive cap or omit / 0 for unlimited",
			})
		}
	}
	return errs
}

func validateURL(field, raw string) *ValidationError {
	return validateURLPolicy(field, raw, false)
}

func validateProviderURL(field, raw string, allowPrivate bool) *ValidationError {
	return validateURLPolicy(field, raw, allowPrivate)
}

func validateURLPolicy(field, raw string, allowPrivate bool) *ValidationError {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return &ValidationError{
			Field: field, Reason: "invalid URL",
			Remediation: "use an absolute URL with scheme, e.g. https://example.com/v1",
		}
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return &ValidationError{
			Field: field, Reason: fmt.Sprintf("unsupported scheme %q", u.Scheme),
			Remediation: "use http or https",
		}
	}
	host := u.Hostname()
	switch security.ClassifyURLHost(host) {
	case "blocked":
		return &ValidationError{
			Field: field, Reason: "URL targets a blocked address (SSRF policy)",
			Remediation: "do not use link-local or cloud metadata addresses",
		}
	case "private":
		if !allowPrivate {
			return &ValidationError{
				Field: field, Reason: "URL targets a private network address",
				Remediation: "set allow_private_network = true only for trusted local gateways",
			}
		}
	case "loopback":
		// Allowed: local gateways such as 9router on localhost:20128.
	}
	return nil
}
