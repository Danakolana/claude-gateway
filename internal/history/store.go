package history

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store is a local SQLite history database.
type Store struct {
	db *sql.DB
}

// Open opens or creates a history database in WAL mode.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA foreign_keys=ON;`,
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS workspace (
			id TEXT PRIMARY KEY, name TEXT, created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS conversation (
			id TEXT PRIMARY KEY, workspace_id TEXT, title TEXT, status TEXT NOT NULL,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
			sync_version INTEGER, sync_tombstone INTEGER DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS message (
			id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL, role TEXT NOT NULL,
			content_json TEXT NOT NULL, status TEXT NOT NULL, correlation_id TEXT,
			created_at TEXT NOT NULL,
			FOREIGN KEY(conversation_id) REFERENCES conversation(id)
		);`,
		`CREATE TABLE IF NOT EXISTS model_invocation (
			id TEXT PRIMARY KEY, conversation_id TEXT, provider TEXT, source_model TEXT,
			target_model TEXT, usage_json TEXT, outcome TEXT, created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS attachment (
			hash TEXT PRIMARY KEY,
			size INTEGER NOT NULL, mime TEXT, path TEXT NOT NULL, created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS audit_event (
			id TEXT PRIMARY KEY, action TEXT NOT NULL, profile TEXT, result TEXT,
			correlation_id TEXT, created_at TEXT NOT NULL
		);`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate: %w (%s)", err, q)
		}
	}
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM schema_version`).Scan(&n)
	if n == 0 {
		_, _ = s.db.Exec(`INSERT INTO schema_version(version) VALUES (1)`)
	}
	return nil
}

// Close closes the DB.
func (s *Store) Close() error { return s.db.Close() }

// AppendConversation creates a conversation and messages in one transaction.
func (s *Store) AppendConversation(ctx context.Context, convID, status string, messages []map[string]any, inv map[string]any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO conversation(id, status, created_at, updated_at) VALUES (?,?,?,?)`,
		convID, status, now, now)
	if err != nil {
		return err
	}
	_, _ = tx.ExecContext(ctx, `UPDATE conversation SET status=?, updated_at=? WHERE id=?`, status, now, convID)
	for i, m := range messages {
		b, _ := json.Marshal(m)
		id := fmt.Sprintf("%s-%d", convID, i)
		corr, _ := m["correlation_id"].(string)
		role, _ := m["role"].(string)
		_, err = tx.ExecContext(ctx, `INSERT OR REPLACE INTO message(id, conversation_id, role, content_json, status, correlation_id, created_at) VALUES (?,?,?,?,?,?,?)`,
			id, convID, role, string(b), status, corr, now)
		if err != nil {
			return err
		}
	}
	if inv != nil {
		b, _ := json.Marshal(inv)
		_, err = tx.ExecContext(ctx, `INSERT INTO model_invocation(id, conversation_id, provider, source_model, target_model, usage_json, outcome, created_at) VALUES (?,?,?,?,?,?,?,?)`,
			convID+"-inv", convID, inv["provider"], inv["source_model"], inv["target_model"], string(b), inv["outcome"], now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListConversations returns recent conversation IDs.
func (s *Store) ListConversations(limit int) ([]string, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id FROM conversation ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ImportResult summarizes a JSONL import.
type ImportResult struct {
	ConversationsImported int
	ConversationsSkipped  int
	MessagesImported      int
	MessagesSkipped       int
}

type exportConversation struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type exportMessage struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	Role           string `json:"role"`
	ContentJSON    string `json:"content_json"`
	Status         string `json:"status"`
	CorrelationID  string `json:"correlation_id"`
	CreatedAt      string `json:"created_at"`
}

const exportFormatVersion = 1

// ExportJSONL writes a portable JSONL archive to dir (manifest, conversations,
// messages, and sha256 checksums).
func (s *Store) ExportJSONL(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	manifest := map[string]any{
		"format_version":   exportFormatVersion,
		"created_at":       time.Now().UTC().Format(time.RFC3339Nano),
		"contains_prompts": true,
	}
	mb, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), append(mb, '\n'), 0o600); err != nil {
		return err
	}
	if err := s.exportConversations(filepath.Join(dir, "conversations.jsonl")); err != nil {
		return err
	}
	if err := s.exportMessages(filepath.Join(dir, "messages.jsonl")); err != nil {
		return err
	}
	return writeChecksums(dir, []string{"manifest.json", "conversations.jsonl", "messages.jsonl"})
}

func (s *Store) exportConversations(path string) error {
	rows, err := s.db.Query(`SELECT id, status, created_at, updated_at FROM conversation`)
	if err != nil {
		return err
	}
	defer rows.Close()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for rows.Next() {
		var row exportConversation
		if err := rows.Scan(&row.ID, &row.Status, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return err
		}
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Store) exportMessages(path string) error {
	rows, err := s.db.Query(`SELECT id, conversation_id, role, content_json, status, correlation_id, created_at FROM message`)
	if err != nil {
		return err
	}
	defer rows.Close()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for rows.Next() {
		var row exportMessage
		var corr sql.NullString
		if err := rows.Scan(&row.ID, &row.ConversationID, &row.Role, &row.ContentJSON, &row.Status, &corr, &row.CreatedAt); err != nil {
			return err
		}
		row.CorrelationID = corr.String
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ImportJSONL loads a directory produced by ExportJSONL.
// Existing conversation IDs are skipped unless replace is true.
func (s *Store) ImportJSONL(dir string, replace bool) (ImportResult, error) {
	var out ImportResult
	st, err := os.Stat(dir)
	if err != nil {
		return out, err
	}
	if !st.IsDir() {
		return out, fmt.Errorf("import: %s is not a directory", dir)
	}
	if err := validateManifest(filepath.Join(dir, "manifest.json")); err != nil {
		return out, err
	}
	if err := verifyChecksums(dir); err != nil {
		return out, err
	}
	convs, err := readJSONL[exportConversation](filepath.Join(dir, "conversations.jsonl"))
	if err != nil {
		return out, fmt.Errorf("conversations.jsonl: %w", err)
	}
	msgs, err := readJSONL[exportMessage](filepath.Join(dir, "messages.jsonl"))
	if err != nil {
		return out, fmt.Errorf("messages.jsonl: %w", err)
	}
	inArchive := make(map[string]struct{}, len(convs))
	for i, c := range convs {
		if strings.TrimSpace(c.ID) == "" {
			return out, fmt.Errorf("conversations.jsonl: missing id on line %d", i+1)
		}
		if _, dup := inArchive[c.ID]; dup {
			return out, fmt.Errorf("conversations.jsonl: duplicate id %q", c.ID)
		}
		inArchive[c.ID] = struct{}{}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback() }()

	skipConv := make(map[string]struct{})
	for _, c := range convs {
		var exists int
		err := tx.QueryRow(`SELECT 1 FROM conversation WHERE id=?`, c.ID).Scan(&exists)
		switch {
		case err == nil && !replace:
			skipConv[c.ID] = struct{}{}
			out.ConversationsSkipped++
			continue
		case err != nil && err != sql.ErrNoRows:
			return out, err
		}
		status := c.Status
		if status == "" {
			status = "imported"
		}
		created := c.CreatedAt
		if created == "" {
			created = time.Now().UTC().Format(time.RFC3339Nano)
		}
		updated := c.UpdatedAt
		if updated == "" {
			updated = created
		}
		if replace && err == nil {
			if _, err := tx.Exec(`DELETE FROM message WHERE conversation_id=?`, c.ID); err != nil {
				return out, err
			}
			if _, err := tx.Exec(`UPDATE conversation SET status=?, created_at=?, updated_at=? WHERE id=?`,
				status, created, updated, c.ID); err != nil {
				return out, err
			}
		} else {
			if _, err := tx.Exec(`INSERT INTO conversation(id, status, created_at, updated_at) VALUES (?,?,?,?)`,
				c.ID, status, created, updated); err != nil {
				return out, err
			}
		}
		out.ConversationsImported++
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i, m := range msgs {
		if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.ConversationID) == "" {
			return out, fmt.Errorf("messages.jsonl: missing id/conversation_id on line %d", i+1)
		}
		if _, ok := inArchive[m.ConversationID]; !ok {
			out.MessagesSkipped++
			continue
		}
		if _, skip := skipConv[m.ConversationID]; skip {
			out.MessagesSkipped++
			continue
		}
		role := m.Role
		if role == "" {
			role = "user"
		}
		status := m.Status
		if status == "" {
			status = "imported"
		}
		created := m.CreatedAt
		if created == "" {
			created = now
		}
		content := m.ContentJSON
		if content == "" {
			content = "{}"
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO message(id, conversation_id, role, content_json, status, correlation_id, created_at) VALUES (?,?,?,?,?,?,?)`,
			m.ID, m.ConversationID, role, content, status, m.CorrelationID, created); err != nil {
			return out, err
		}
		out.MessagesImported++
	}
	if err := tx.Commit(); err != nil {
		return out, err
	}
	return out, nil
}

func validateManifest(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("import: missing manifest.json")
		}
		return err
	}
	var man struct {
		FormatVersion int `json:"format_version"`
	}
	if err := json.Unmarshal(b, &man); err != nil {
		return fmt.Errorf("manifest.json: %w", err)
	}
	if man.FormatVersion != 0 && man.FormatVersion != exportFormatVersion {
		return fmt.Errorf("manifest.json: unsupported format_version %d", man.FormatVersion)
	}
	return nil
}

func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var v T
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		out = append(out, v)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func writeChecksums(dir string, names []string) error {
	var b strings.Builder
	for _, name := range names {
		sum, err := fileSHA256(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s  %s\n", sum, name)
	}
	return os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(b.String()), 0o600)
}

func verifyChecksums(dir string) error {
	path := filepath.Join(dir, "checksums.txt")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // older exports without checksums
		}
		return err
	}
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return fmt.Errorf("checksums.txt: malformed line %q", line)
		}
		want, name := fields[0], fields[len(fields)-1]
		if filepath.Base(name) != name || strings.Contains(name, "..") {
			return fmt.Errorf("checksums.txt: invalid filename %q", name)
		}
		got, err := fileSHA256(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("checksum %s: %w", name, err)
		}
		if got != want {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
	}
	return sc.Err()
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Audit writes a safe audit event.
func (s *Store) Audit(action, profile, result, corr string) error {
	_, err := s.db.Exec(`INSERT INTO audit_event(id, action, profile, result, correlation_id, created_at) VALUES (?,?,?,?,?,?)`,
		fmt.Sprintf("%d", time.Now().UnixNano()), action, profile, result, corr, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
