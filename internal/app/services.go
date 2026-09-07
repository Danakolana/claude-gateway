package app

import (
	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/secrets"
)

// Services is the stable application boundary for CLI and a future GUI.
type Services struct {
	Resolver secrets.Resolver
}

// ValidateConfig loads and validates configuration.
func (s Services) ValidateConfig(path string) (*config.File, string, []config.ValidationError, error) {
	cfg, src, err := config.Load(path, s.Resolver)
	if err != nil {
		return nil, "", nil, err
	}
	return cfg, src, config.Validate(cfg), nil
}
