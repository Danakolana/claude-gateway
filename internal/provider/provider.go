package provider

import (
	"context"

	"github.com/danakolana/claude-gateway/pkg/api"
)

// Target identifies a model on a provider.
type Target struct {
	Provider string
	ModelID  string
}

// HealthResult is a safe health snapshot.
type HealthResult struct {
	OK      bool
	Message string
}

// Adapter is the upstream provider boundary.
type Adapter interface {
	Name() string
	Capabilities(ctx context.Context, target Target) (api.Capabilities, error)
	Send(ctx context.Context, req api.Request) (api.Response, error)
	Stream(ctx context.Context, req api.Request) (<-chan api.Event, error)
	Health(ctx context.Context) HealthResult
}
