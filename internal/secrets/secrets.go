package secrets

import (
	"fmt"
	"os"
	"strings"
)

// Handle is a reference to a secret, never the printable secret itself.
type Handle struct {
	Kind string // "env"
	Ref  string // environment variable name
}

// String redacts the secret.
func (h Handle) String() string {
	if h.Ref == "" {
		return "<secret:unset>"
	}
	return fmt.Sprintf("<secret:%s:%s>", h.Kind, h.Ref)
}

// GoString ensures %#v also redacts.
func (h Handle) GoString() string { return h.String() }

// Resolver resolves secret handles at operation time.
type Resolver interface {
	Resolve(h Handle) (string, error)
}

// EnvResolver reads secrets from the process environment.
type EnvResolver struct{}

// Resolve implements Resolver.
func (EnvResolver) Resolve(h Handle) (string, error) {
	if h.Kind != "" && h.Kind != "env" {
		return "", fmt.Errorf("unsupported secret kind %q", h.Kind)
	}
	if strings.TrimSpace(h.Ref) == "" {
		return "", fmt.Errorf("empty secret reference")
	}
	v, ok := os.LookupEnv(h.Ref)
	if !ok || v == "" {
		return "", fmt.Errorf("secret environment variable %q is missing or empty", h.Ref)
	}
	return v, nil
}

// ParseEnvRef parses "${ENV:NAME}" or "env:NAME" into a Handle.
func ParseEnvRef(s string) (Handle, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "${ENV:") && strings.HasSuffix(s, "}") {
		return Handle{Kind: "env", Ref: strings.TrimSuffix(strings.TrimPrefix(s, "${ENV:"), "}")}, true
	}
	if strings.HasPrefix(s, "env:") {
		return Handle{Kind: "env", Ref: strings.TrimPrefix(s, "env:")}, true
	}
	return Handle{}, false
}
