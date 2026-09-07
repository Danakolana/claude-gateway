package observability

import (
	"crypto/rand"
	"encoding/hex"
)

// CorrelationID returns an existing ID or a new random one.
func CorrelationID(existing string) string {
	if existing != "" {
		return existing
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
