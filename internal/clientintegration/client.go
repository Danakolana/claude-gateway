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

// GatewayRender is the 3P enterpriseConfig fragment we apply.
type GatewayRender struct {
	DeploymentMode   string `json:"deploymentMode"`
	EnterpriseConfig struct {
		InferenceProvider          string   `json:"inferenceProvider"`
		InferenceGatewayBaseURL    string   `json:"inferenceGatewayBaseUrl"`
		InferenceGatewayAPIKey     string   `json:"inferenceGatewayApiKey,omitempty"`
		InferenceGatewayAuthScheme string   `json:"inferenceGatewayAuthScheme,omitempty"`
		InferenceModels            []string `json:"inferenceModels,omitempty"`
	} `json:"enterpriseConfig"`
}

// Render3P builds a candidate Claude Desktop on 3P config.
func Render3P(baseURL, apiKey, authScheme string, models []string) GatewayRender {
	var g GatewayRender
	g.DeploymentMode = "3p"
	g.EnterpriseConfig.InferenceProvider = "gateway"
	g.EnterpriseConfig.InferenceGatewayBaseURL = baseURL
	g.EnterpriseConfig.InferenceGatewayAPIKey = apiKey
	if authScheme == "" {
		authScheme = "bearer"
	}
	g.EnterpriseConfig.InferenceGatewayAuthScheme = authScheme
	g.EnterpriseConfig.InferenceModels = models
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

// Apply writes candidate JSON atomically after backup. dryRun skips write.
func Apply(path string, candidate GatewayRender, backupDir, profile, version string, dryRun bool) (Snapshot, error) {
	raw, err := json.MarshalIndent(candidate, "", "  ")
	if err != nil {
		return Snapshot{}, err
	}
	var snap Snapshot
	if _, err := os.Stat(path); err == nil {
		snap, err = Backup(path, backupDir, profile, version)
		if err != nil {
			return snap, err
		}
	} else {
		snap = Snapshot{SourcePath: path, CreatedAt: time.Now().UTC(), Profile: profile, ToolVersion: version}
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
	if _, err := Backup(target, backupDir, profile, version+"-pre-restore"); err != nil && !os.IsNotExist(err) {
		// if target missing, still restore
		if !os.IsNotExist(err) {
			// Backup fails if target missing — ignore
		}
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
