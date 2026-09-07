package clientintegration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Snapshot is a checksummed backup of a client config file.
type Snapshot struct {
	SourcePath  string    `json:"source_path"`
	BackupPath  string    `json:"backup_path"`
	Checksum    string    `json:"checksum"`
	CreatedAt   time.Time `json:"created_at"`
	ToolVersion string    `json:"tool_version"`
	Profile     string    `json:"profile"`
}

// InferenceModelEntry is an explicit Desktop model list entry.
// Field names match Claude Desktop on 3P configuration reference (camelCase).
type InferenceModelEntry struct {
	Name                 string `json:"name"`
	LabelOverride        string `json:"labelOverride,omitempty"`
	AnthropicFamilyTier  string `json:"anthropicFamilyTier,omitempty"`
	IsFamilyDefault      bool   `json:"isFamilyDefault,omitempty"`
}

// EnterpriseConfig is the Claude Desktop on 3P gateway block.
type EnterpriseConfig struct {
	InferenceProvider          string                `json:"inferenceProvider"`
	InferenceGatewayBaseURL    string                `json:"inferenceGatewayBaseUrl"`
	InferenceGatewayAPIKey     string                `json:"inferenceGatewayApiKey,omitempty"`
	InferenceGatewayAuthScheme string                `json:"inferenceGatewayAuthScheme,omitempty"`
	InferenceModels            []InferenceModelEntry `json:"inferenceModels,omitempty"`
	ModelDiscoveryEnabled      *bool                 `json:"modelDiscoveryEnabled,omitempty"`
}

// GatewayRender is the 3P fragment we merge into an existing Desktop config.
type GatewayRender struct {
	DeploymentMode   string          `json:"deploymentMode"`
	EnterpriseConfig EnterpriseConfig `json:"enterpriseConfig"`
}

// Render3P builds a candidate Claude Desktop on 3P config fragment.
// Desktop validates Model IDs as Anthropic-looking names (claude-* /
// anthropic/claude-*). Real OpenRouter IDs belong only in gateway routing.
func Render3P(baseURL, apiKey, authScheme string, models []string) GatewayRender {
	entries := make([]InferenceModelEntry, 0, len(models))
	for i, m := range models {
		tier := "sonnet"
		if i == 0 {
			tier = "haiku"
		}
		entries = append(entries, InferenceModelEntry{
			Name: m, LabelOverride: m, AnthropicFamilyTier: tier, IsFamilyDefault: i < 2,
		})
	}
	return Render3PEntries(baseURL, apiKey, authScheme, entries, false)
}

// Render3PEntries builds Desktop config from explicit picker entries.
// discoveryEnabled=true is typical for direct OpenRouter (their /v1/models catalog).
func Render3PEntries(baseURL, apiKey, authScheme string, entries []InferenceModelEntry, discoveryEnabled bool) GatewayRender {
	var g GatewayRender
	g.DeploymentMode = "3p"
	g.EnterpriseConfig.InferenceProvider = "gateway"
	g.EnterpriseConfig.InferenceGatewayBaseURL = baseURL
	g.EnterpriseConfig.InferenceGatewayAPIKey = apiKey
	if authScheme == "" {
		authScheme = "bearer"
	}
	g.EnterpriseConfig.InferenceGatewayAuthScheme = authScheme
	if len(entries) == 0 {
		entries = []InferenceModelEntry{
			{Name: "claude-sonnet-4-5", LabelOverride: "Sonnet (gateway)", AnthropicFamilyTier: "sonnet", IsFamilyDefault: true},
		}
	}
	g.EnterpriseConfig.InferenceModels = entries
	disc := discoveryEnabled
	g.EnterpriseConfig.ModelDiscoveryEnabled = &disc
	return g
}

// RedactedDiff returns a redacted JSON view of the candidate.
func RedactedDiff(g GatewayRender) string {
	cp := g
	if cp.EnterpriseConfig.InferenceGatewayAPIKey != "" {
		cp.EnterpriseConfig.InferenceGatewayAPIKey = "<redacted>"
	}
	b, _ := json.MarshalIndent(cp, "", "  ")
	return string(b)
}

func checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Backup copies path into backupDir and returns a snapshot record.
func Backup(path, backupDir, profile, version string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, err
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return Snapshot{}, err
	}
	name := fmt.Sprintf("backup-%s.json", time.Now().UTC().Format("20060102T150405"))
	dst := filepath.Join(backupDir, name)
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return Snapshot{}, err
	}
	meta := Snapshot{
		SourcePath: path, BackupPath: dst, Checksum: checksum(data),
		CreatedAt: time.Now().UTC(), ToolVersion: version, Profile: profile,
	}
	mb, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(dst+".meta.json", mb, 0o600)
	return meta, nil
}

// MergeIntoExisting merges gateway settings into an existing Desktop JSON
// document without wiping preferences or other keys.
func MergeIntoExisting(existing []byte, candidate GatewayRender) ([]byte, error) {
	root := map[string]any{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, fmt.Errorf("parse existing config: %w", err)
		}
	}
	root["deploymentMode"] = candidate.DeploymentMode
	entBytes, err := json.Marshal(candidate.EnterpriseConfig)
	if err != nil {
		return nil, err
	}
	var entMap map[string]any
	if err := json.Unmarshal(entBytes, &entMap); err != nil {
		return nil, err
	}
	root["enterpriseConfig"] = entMap
	return json.MarshalIndent(root, "", "  ")
}

// Apply merges candidate into the existing file (preserving preferences),
// after backup. dryRun skips write.
func Apply(path string, candidate GatewayRender, backupDir, profile, version string, dryRun bool) (Snapshot, error) {
	var existing []byte
	var snap Snapshot
	if data, err := os.ReadFile(path); err == nil {
		existing = data
		snap, err = Backup(path, backupDir, profile, version)
		if err != nil {
			return snap, err
		}
	} else {
		snap = Snapshot{SourcePath: path, CreatedAt: time.Now().UTC(), Profile: profile, ToolVersion: version}
	}
	raw, err := MergeIntoExisting(existing, candidate)
	if err != nil {
		return snap, err
	}
	if dryRun {
		return snap, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return snap, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return snap, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return snap, err
	}
	snap.Checksum = checksum(raw)
	return snap, nil
}

// Restore copies a backup file back to the target, creating a new rollback backup first.
func Restore(target, backupFile, backupDir, profile, version string) error {
	if _, err := os.Stat(target); err == nil {
		_, _ = Backup(target, backupDir, profile, version+"-pre-restore")
	}
	data, err := os.ReadFile(backupFile)
	if err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}
