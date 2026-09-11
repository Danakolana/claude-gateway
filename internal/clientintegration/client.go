package clientintegration

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
// Prefer discoveryEnabled=false when entries are set: Desktop skips /v1/models
// discovery when inferenceModels is present, and OpenRouter discovery is
// Anthropic-filtered (would hide DeepSeek/GLM/Kimi from the TOML list).
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
// after backup. dryRun skips write. Also syncs Claude Desktop 3P
// configLibrary (the Connection UI source of truth) next to the config file.
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
	if _, err := SyncConfigLibrary(path, candidate, backupDir, profile, version, false); err != nil {
		return snap, fmt.Errorf("configLibrary: %w", err)
	}
	return snap, nil
}

type configLibraryMeta struct {
	AppliedID string `json:"appliedId"`
	Entries   []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"entries"`
}

func newLibraryID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// libraryPayload is the Connection profile JSON Desktop reads from configLibrary.
func libraryPayload(c EnterpriseConfig) map[string]any {
	disc := false
	if c.ModelDiscoveryEnabled != nil {
		disc = *c.ModelDiscoveryEnabled
	}
	return map[string]any{
		"inferenceGatewayBaseUrl":    c.InferenceGatewayBaseURL,
		"inferenceGatewayApiKey":     c.InferenceGatewayAPIKey,
		"inferenceGatewayAuthScheme": c.InferenceGatewayAuthScheme,
		"inferenceProvider":          c.InferenceProvider,
		"inferenceCredentialKind":    "static",
		"modelDiscoveryEnabled":      disc,
		"modelPrefer1mContext":       false,
		"inferenceModels":            c.InferenceModels,
	}
}

// SyncConfigLibrary updates the applied Claude Desktop 3P Connection profile
// under <configDir>/configLibrary so restart does not revert to an old gateway URL.
// If no library exists, it creates a "claude-gateway" profile and marks it applied.
func SyncConfigLibrary(desktopConfigPath string, candidate GatewayRender, backupDir, profile, version string, dryRun bool) (string, error) {
	libDir := filepath.Join(filepath.Dir(desktopConfigPath), "configLibrary")
	metaPath := filepath.Join(libDir, "_meta.json")
	var meta configLibraryMeta
	if data, err := os.ReadFile(metaPath); err == nil {
		_ = json.Unmarshal(data, &meta)
	}
	id := meta.AppliedID
	if id == "" && len(meta.Entries) > 0 {
		id = meta.Entries[0].ID
	}
	if id == "" {
		id = newLibraryID()
		meta = configLibraryMeta{
			AppliedID: id,
			Entries: []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}{{ID: id, Name: "claude-gateway"}},
		}
	} else {
		meta.AppliedID = id
		found := false
		for _, e := range meta.Entries {
			if e.ID == id {
				found = true
				break
			}
		}
		if !found {
			meta.Entries = append(meta.Entries, struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}{ID: id, Name: "claude-gateway"})
		}
	}
	entryPath := filepath.Join(libDir, id+".json")
	payload := libraryPayload(candidate.EnterpriseConfig)
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	metaRaw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", err
	}
	if dryRun {
		return entryPath, nil
	}
	if err := os.MkdirAll(libDir, 0o700); err != nil {
		return "", err
	}
	if st, err := os.Stat(entryPath); err == nil && !st.IsDir() {
		_, _ = Backup(entryPath, backupDir, profile, version+"-configLibrary")
	}
	tmp := entryPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, entryPath); err != nil {
		return "", err
	}
	mtmp := metaPath + ".tmp"
	if err := os.WriteFile(mtmp, metaRaw, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(mtmp, metaPath); err != nil {
		return "", err
	}
	return entryPath, nil
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

// ConsumerEnvRender is the experimental consumer Desktop env fragment.
// Regular Claude Desktop (non-3P) may honor ANTHROPIC_BASE_URL in config env.
type ConsumerEnvRender struct {
	Env map[string]string `json:"env"`
}

// RenderConsumerEnv builds the experimental consumer env block.
// apiKey may be empty (omitted from env).
func RenderConsumerEnv(baseURL, apiKey string) ConsumerEnvRender {
	env := map[string]string{
		"ANTHROPIC_BASE_URL": strings.TrimRight(strings.TrimSpace(baseURL), "/"),
	}
	if k := strings.TrimSpace(apiKey); k != "" {
		env["ANTHROPIC_API_KEY"] = k
	}
	return ConsumerEnvRender{Env: env}
}

// RedactedConsumerDiff returns a redacted JSON view of the consumer fragment.
func RedactedConsumerDiff(c ConsumerEnvRender) string {
	cp := ConsumerEnvRender{Env: map[string]string{}}
	for k, v := range c.Env {
		if k == "ANTHROPIC_API_KEY" && v != "" {
			cp.Env[k] = "<redacted>"
		} else {
			cp.Env[k] = v
		}
	}
	b, _ := json.MarshalIndent(cp, "", "  ")
	return string(b)
}

// MergeConsumerEnv merges ANTHROPIC_* env keys into an existing Desktop JSON
// document without wiping preferences, MCP, or other keys.
func MergeConsumerEnv(existing []byte, candidate ConsumerEnvRender) ([]byte, error) {
	root := map[string]any{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, fmt.Errorf("parse existing config: %w", err)
		}
	}
	envMap := map[string]any{}
	if prev, ok := root["env"].(map[string]any); ok {
		for k, v := range prev {
			envMap[k] = v
		}
	}
	for k, v := range candidate.Env {
		envMap[k] = v
	}
	root["env"] = envMap
	return json.MarshalIndent(root, "", "  ")
}

// ApplyConsumer merges the experimental consumer env fragment after backup.
// Does not touch 3P configLibrary.
func ApplyConsumer(path string, candidate ConsumerEnvRender, backupDir, profile, version string, dryRun bool) (Snapshot, error) {
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
	raw, err := MergeConsumerEnv(existing, candidate)
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

