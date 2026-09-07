package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// ExportJSONL writes a simple portable export to dir.
func (s *Store) ExportJSONL(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	manifest := map[string]any{"format_version": 1, "created_at": time.Now().UTC(), "contains_prompts": true}
	mb, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), mb, 0o600); err != nil {
		return err
	}
	rows, err := s.db.Query(`SELECT id, status, created_at, updated_at FROM conversation`)
	if err != nil {
		return err
	}
	defer rows.Close()
	cf, err := os.Create(filepath.Join(dir, "conversations.jsonl"))
	if err != nil {
		return err
	}
	defer cf.Close()
	for rows.Next() {
		var id, status, c, u string
		if err := rows.Scan(&id, &status, &c, &u); err != nil {
			return err
		}
		b, _ := json.Marshal(map[string]any{"id": id, "status": status, "created_at": c, "updated_at": u})
		_, _ = cf.Write(append(b, '\n'))
	}
	mrows, err := s.db.Query(`SELECT id, conversation_id, role, content_json, status, correlation_id, created_at FROM message`)
	if err != nil {
		return err
	}
	defer mrows.Close()
	mf, err := os.Create(filepath.Join(dir, "messages.jsonl"))
	if err != nil {
		return err
	}
	defer mf.Close()
	for mrows.Next() {
		var id, cid, role, content, status, corr, created string
		if err := mrows.Scan(&id, &cid, &role, &content, &status, &corr, &created); err != nil {
			return err
		}
		b, _ := json.Marshal(map[string]any{
			"id": id, "conversation_id": cid, "role": role, "content_json": content,
			"status": status, "correlation_id": corr, "created_at": created,
		})
		_, _ = mf.Write(append(b, '\n'))
	}
	return nil
}

// Audit writes a safe audit event.
func (s *Store) Audit(action, profile, result, corr string) error {
	_, err := s.db.Exec(`INSERT INTO audit_event(id, action, profile, result, correlation_id, created_at) VALUES (?,?,?,?,?,?)`,
		fmt.Sprintf("%d", time.Now().UnixNano()), action, profile, result, corr, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
