package config

import "fmt"

// Resolve builds an effective configuration for the active profile.
func Resolve(f *File) (*Effective, error) {
	if f.ActiveProfile == "" {
		return nil, fmt.Errorf("no active profile")
	}
	prof, ok := f.Profiles[f.ActiveProfile]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", f.ActiveProfile)
	}
	prov, ok := f.Providers[prof.Provider]
	if !ok && prof.Provider != "" {
		return nil, fmt.Errorf("provider %q not found", prof.Provider)
	}
	origins := map[string]string{
		"active_profile": "toml:active_profile",
		"provider":       "toml:profiles." + f.ActiveProfile + ".provider",
	}
	return &Effective{
		ActiveProfile: f.ActiveProfile,
		ProviderName:  prof.Provider,
		Provider:      prov,
		Routing:       prof.Routing,
		Models:        f.Models,
		History:       f.History,
		Sync:          f.Sync,
		Origins:       origins,
	}, nil
}

// Explain returns a redacted human-readable effective configuration.
func Explain(eff *Effective, sourcePath string) string {
	var b stringsBuilder
	b.WriteString("source: " + sourcePath + "\n")
	b.WriteString("active_profile: " + eff.ActiveProfile + " (from " + eff.Origins["active_profile"] + ")\n")
	b.WriteString("provider: " + eff.ProviderName + "\n")
	b.WriteString("  base_url: " + eff.Provider.BaseURL + "\n")
	b.WriteString("  protocol: " + eff.Provider.Protocol + "\n")
	b.WriteString("  auth_scheme: " + eff.Provider.AuthScheme + "\n")
	if eff.Provider.APIKeyEnv != "" {
		b.WriteString("  api_key: <secret:env:" + eff.Provider.APIKeyEnv + ">\n")
	} else if eff.Provider.APIKeyInline != "" {
		b.WriteString("  api_key: <redacted-inline>\n")
	} else {
		b.WriteString("  api_key: <unset>\n")
	}
	b.WriteString("models:\n")
	for name, m := range eff.Models {
		b.WriteString(fmt.Sprintf("  %s: id=%s tier=%s enabled=%v\n", name, m.ModelID, m.TierAlias, m.Enabled))
	}
	b.WriteString("routing.rules:\n")
	for _, r := range eff.Routing.Rules {
		b.WriteString(fmt.Sprintf("  %s -> %s\n", r.Source, r.TargetModel))
	}
	if eff.Sync.TokenEnv != "" {
		b.WriteString("sync.token: <secret:env:" + eff.Sync.TokenEnv + ">\n")
	}
	if eff.Sync.Token != "" {
		b.WriteString("sync.token: <redacted-inline>\n")
	}
	return b.String()
}

// tiny wrapper to avoid importing strings only for Builder in older style
type stringsBuilder struct{ b []byte }

func (s *stringsBuilder) WriteString(v string) { s.b = append(s.b, v...) }
func (s *stringsBuilder) String() string       { return string(s.b) }
