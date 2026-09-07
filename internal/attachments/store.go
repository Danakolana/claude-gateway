package attachments

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Store is a content-addressed blob store.
type Store struct {
	Dir string
}

// ValidateHash rejects path-traversal and non-hex values.
func ValidateHash(h string) error {
	if !hex64.MatchString(h) {
		return fmt.Errorf("invalid content hash")
	}
	return nil
}

// Put writes data and returns sha256 hex.
func (s *Store) Put(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	h := hex.EncodeToString(sum[:])
	if err := ValidateHash(h); err != nil {
		return "", err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", err
	}
	final := filepath.Join(s.Dir, h)
	if _, err := os.Stat(final); err == nil {
		return h, nil
	}
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return h, nil
}

// Path returns the blob path after validating the hash.
func (s *Store) Path(h string) (string, error) {
	if err := ValidateHash(h); err != nil {
		return "", err
	}
	return filepath.Join(s.Dir, h), nil
}
