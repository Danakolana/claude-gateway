package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/danakolana/claude-gateway/internal/secrets"
)

// Load reads configuration using discovery order when path is empty.
// If nothing is found (and no explicit path / CLAUDE_GATEWAY_CONFIG was set),
// the embedded default is written to ~/.config/claude-gateway/config.toml.
func Load(explicitPath string, _ secrets.Resolver) (*File, string, error) {
	path, created, err := DiscoverOrCreate(explicitPath)
	if err != nil {
		return nil, "", err
	}
	f, err := ParseFile(path)
	if err != nil {
		return nil, path, err
	}
	ApplyEnvOverrides(f)
	if created {
		// Stash for CLI messaging without changing the Load signature.
		lastCreatedConfigPath = path
	} else {
		lastCreatedConfigPath = ""
	}
	return f, path, nil
}

// lastCreatedConfigPath is set when Load just wrote the embedded default.
var lastCreatedConfigPath string

// CreatedDefaultConfigPath returns the path of a config file written by the
// most recent Load call, or "" if Load reused an existing file.
func CreatedDefaultConfigPath() string { return lastCreatedConfigPath }

// ParseFile parses a TOML configuration file.
func ParseFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var f File
	meta, err := toml.Decode(string(data), &f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	_ = meta
	if f.Profiles == nil {
		f.Profiles = map[string]Profile{}
	}
	if f.Providers == nil {
		f.Providers = map[string]Provider{}
	}
	if f.Models == nil {
		f.Models = map[string]Model{}
	}
	return &f, nil
}

// Discover implements FR-CONFIG-002. First win; later sources ignored.
// It does not create files — see DiscoverOrCreate for first-run bootstrap.
func Discover(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if v := os.Getenv("CLAUDE_GATEWAY_CONFIG"); v != "" {
		return v, nil
	}
	var candidates []string
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".config", "claude-gateway", "config.toml"),
			filepath.Join(home, ".claude-gateway", "config.toml"),
		)
	}
	// Convenient when running from a checkout / quickstart.
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "config.toml"),
			filepath.Join(cwd, "examples", "config.toml"), // legacy
		)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("no configuration found: set --config PATH, export CLAUDE_GATEWAY_CONFIG, or place config at ~/.config/claude-gateway/config.toml (or ./config.toml)")
}

// DiscoverOrCreate is Discover, then on miss writes the embedded default to
// the user config path (unless an explicit path or CLAUDE_GATEWAY_CONFIG was set).
func DiscoverOrCreate(explicit string) (path string, created bool, err error) {
	path, err = Discover(explicit)
	if err == nil {
		return path, false, nil
	}
	if explicit != "" || strings.TrimSpace(os.Getenv("CLAUDE_GATEWAY_CONFIG")) != "" {
		return "", false, err
	}
	return EnsureUserConfig()
}

// ApplyEnvOverrides applies declared overrides only.
// Supported: CLAUDE_GATEWAY_ACTIVE_PROFILE, CLAUDE_GATEWAY_LISTEN,
// OPENROUTER_BASE_URL (overrides providers.openrouter.base_url).
func ApplyEnvOverrides(f *File) {
	if v := os.Getenv("CLAUDE_GATEWAY_ACTIVE_PROFILE"); v != "" {
		f.ActiveProfile = v
	}
	if v := os.Getenv("CLAUDE_GATEWAY_LISTEN"); v != "" {
		f.Proxy.Listen = v
	}
	if v := strings.TrimSpace(os.Getenv("OPENROUTER_BASE_URL")); v != "" {
		if f.Providers == nil {
			f.Providers = map[string]Provider{}
		}
		p := f.Providers["openrouter"]
		p.BaseURL = v
		f.Providers["openrouter"] = p
	}
}

// ProfileNames returns sorted profile names.
func ProfileNames(f *File) []string {
	names := make([]string, 0, len(f.Profiles))
	for n := range f.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Save writes the configuration file atomically.
func Save(path string, f *File) error {
	if path == "" {
		var err error
		path, err = Discover("")
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	enc := toml.NewEncoder(out)
	if err := enc.Encode(f); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// CreateProfile adds an empty profile.
func CreateProfile(path, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("profile name required")
	}
	f, resolved, err := Load(path, nil)
	if err != nil {
		// Allow creating a brand-new file when explicit path is given.
		if path == "" {
			return err
		}
		f = &File{Version: 1, Profiles: map[string]Profile{}, Providers: map[string]Provider{}, Models: map[string]Model{}}
		resolved = path
	}
	if _, exists := f.Profiles[name]; exists {
		return fmt.Errorf("profile %q already exists", name)
	}
	f.Profiles[name] = Profile{ProxyMode: "local", HistoryMode: "local"}
	if f.ActiveProfile == "" {
		f.ActiveProfile = name
	}
	return Save(resolved, f)
}

// SelectProfile sets the active profile.
func SelectProfile(path, name string) error {
	f, resolved, err := Load(path, nil)
	if err != nil {
		return err
	}
	if _, ok := f.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	f.ActiveProfile = name
	return Save(resolved, f)
}

// DeleteProfile removes a profile without deleting unrelated ones.
func DeleteProfile(path, name string) error {
	f, resolved, err := Load(path, nil)
	if err != nil {
		return err
	}
	if _, ok := f.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	delete(f.Profiles, name)
	if f.ActiveProfile == name {
		f.ActiveProfile = ""
		for n := range f.Profiles {
			f.ActiveProfile = n
			break
		}
	}
	return Save(resolved, f)
}
