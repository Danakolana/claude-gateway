package clientintegration

import (
	"os"
)

// FileChecksum returns the SHA-256 hex digest of path. Missing files error.
func FileChecksum(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return checksum(data), nil
}

// Drifted reports whether path's contents no longer match expectedChecksum.
// Read errors are returned (caller should WARN, not rewrite).
func Drifted(path, expectedChecksum string) (bool, error) {
	if expectedChecksum == "" {
		return false, nil
	}
	got, err := FileChecksum(path)
	if err != nil {
		return false, err
	}
	return got != expectedChecksum, nil
}
