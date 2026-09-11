package clientintegration

import (
	"os"
	"path/filepath"
	"strings"
)

// AppliedView is a redacted read of Desktop config on disk.
type AppliedView struct {
	Path           string
	Exists         bool
	BaseURL        string
	LibraryPath    string
	LibraryURL     string
	ConsumerURL    string
	DeploymentMode string
	ModelCount     int
}

// EffectiveBaseURL prefers configLibrary (current Desktop), then enterpriseConfig, then env.
func (v AppliedView) EffectiveBaseURL() string {
	for _, u := range []string{v.LibraryURL, v.BaseURL, v.ConsumerURL} {
		if strings.TrimSpace(u) != "" {
			return strings.TrimRight(strings.TrimSpace(u), "/")
		}
	}
	return ""
}

// SameGatewayURL compares base URLs ignoring trailing slashes and case of the host scheme.
func SameGatewayURL(a, b string) bool {
	return CanonicalGatewayURL(a) != "" && CanonicalGatewayURL(a) == CanonicalGatewayURL(b)
}

func CanonicalGatewayURL(u string) string {
	return strings.TrimRight(strings.TrimSpace(u), "/")
}

// Inspect reads Claude Desktop JSON + configLibrary without printing secrets.
func Inspect(path string) AppliedView {
	v := AppliedView{Path: path}
	if path == "" {
		return v
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		v.Exists = true
		root, err := parseJSONObject(raw, "desktop")
		if err == nil {
			v.DeploymentMode = stringField(root, "deploymentMode", "deployment_mode")
			if ent, ok := root["enterpriseConfig"].(map[string]any); ok {
				v.BaseURL = stringField(ent, "inferenceGatewayBaseUrl", "inferenceGatewayBaseURL")
				if models, ok := ent["inferenceModels"].([]any); ok {
					v.ModelCount = len(models)
				}
			}
			if env, ok := root["env"].(map[string]any); ok {
				v.ConsumerURL = stringField(env, "ANTHROPIC_BASE_URL")
			}
		}
	} else if st, statErr := os.Stat(filepath.Dir(path)); statErr == nil && st.IsDir() {
		v.Exists = true
	}

	libDir := filepath.Join(filepath.Dir(path), "configLibrary")
	metaPath := filepath.Join(libDir, "_meta.json")
	metaRaw, err := os.ReadFile(metaPath)
	if err != nil {
		return v
	}
	meta, err := parseJSONObject(metaRaw, "configLibrary/_meta.json")
	if err != nil {
		return v
	}
	id := libraryAppliedID(meta)
	if id == "" {
		return v
	}
	entryPath := filepath.Join(libDir, id+".json")
	v.LibraryPath = entryPath
	entryRaw, err := os.ReadFile(entryPath)
	if err != nil {
		return v
	}
	entry, err := parseJSONObject(entryRaw, "configLibrary entry")
	if err != nil {
		return v
	}
	v.LibraryURL = stringField(entry, "inferenceGatewayBaseUrl", "inferenceGatewayBaseURL")
	if models, ok := entry["inferenceModels"].([]any); ok && v.ModelCount == 0 {
		v.ModelCount = len(models)
	}
	return v
}
