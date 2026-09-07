package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Config for the history sync server.
type Config struct {
	Addr    string
	Token   string
	DataDir string
	TLSCert string
	TLSKey  string
}

// Server is a minimal authenticated sync server.
type Server struct {
	cfg     Config
	mu      sync.Mutex
	objects map[string]Object
	revoked map[string]bool
	idem    map[string]time.Time
}

// Object is a sync envelope.
type Object struct {
	ID        string          `json:"id"`
	Version   int             `json:"version"`
	Tenant    string          `json:"tenant"`
	User      string          `json:"user"`
	Checksum  string          `json:"checksum"`
	Tombstone bool            `json:"tombstone"`
	Body      json.RawMessage `json:"body"`
}

func New(cfg Config) *Server {
	return &Server{
		cfg: cfg, objects: map[string]Object{}, revoked: map[string]bool{},
		idem: map[string]time.Time{},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.health)
	mux.HandleFunc("/v1/sync/push", s.auth(s.push))
	mux.HandleFunc("/v1/sync/changes", s.auth(s.changes))
	mux.HandleFunc("/v1/attachments/", s.auth(s.attachments))
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		token := strings.TrimPrefix(h, "Bearer ")
		if token == "" || token != s.cfg.Token || s.revoked[token] {
			http.Error(w, `{"category":"sync_auth"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) push(w http.ResponseWriter, r *http.Request) {
	idem := r.Header.Get("Idempotency-Key")
	s.mu.Lock()
	defer s.mu.Unlock()
	if idem != "" {
		if t, ok := s.idem[idem]; ok && time.Since(t) < 24*time.Hour {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "duplicate"})
			return
		}
		s.idem[idem] = time.Now()
	}
	var batch struct {
		Tenant  string   `json:"tenant"`
		User    string   `json:"user"`
		Objects []Object `json:"objects"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&batch); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var accepted, conflicts []string
	for _, o := range batch.Objects {
		o.Tenant, o.User = batch.Tenant, batch.User
		key := batch.Tenant + "/" + o.ID
		if prev, ok := s.objects[key]; ok && prev.Version >= o.Version && !o.Tombstone {
			conflicts = append(conflicts, o.ID)
			continue
		}
		s.objects[key] = o
		accepted = append(accepted, o.ID)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"accepted": accepted, "conflicts": conflicts})
}

func (s *Server) changes(w http.ResponseWriter, r *http.Request) {
	tenant := r.URL.Query().Get("tenant")
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Object
	for k, o := range s.objects {
		if strings.HasPrefix(k, tenant+"/") {
			out = append(out, o)
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"changes": out, "cursor": "end"})
}

func (s *Server) attachments(w http.ResponseWriter, r *http.Request) {
	hash := strings.TrimPrefix(r.URL.Path, "/v1/attachments/")
	if err := validateHash(hash); err != nil {
		http.Error(w, "invalid hash", 400)
		return
	}
	dir := filepath.Join(s.cfg.DataDir, "attachments")
	_ = os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, hash)
	switch r.Method {
	case http.MethodPut:
		data, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != hash {
			http.Error(w, "checksum mismatch", 400)
			return
		}
		_ = os.WriteFile(path, data, 0o600)
		w.WriteHeader(201)
	case http.MethodGet:
		http.ServeFile(w, r, path)
	default:
		http.Error(w, "method", http.StatusMethodNotAllowed)
	}
}

func validateHash(h string) error {
	if len(h) != 64 {
		return os.ErrInvalid
	}
	for _, c := range h {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return os.ErrInvalid
		}
	}
	if strings.Contains(h, "/") || strings.Contains(h, "..") {
		return os.ErrInvalid
	}
	return nil
}

// Revoke marks a token revoked.
func (s *Server) Revoke(token string) { s.mu.Lock(); s.revoked[token] = true; s.mu.Unlock() }
