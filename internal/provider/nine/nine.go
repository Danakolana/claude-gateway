package nine

import (
	"time"

	"github.com/danakolana/claude-gateway/internal/provider/openai"
)

// New builds a 9router OpenAI-compatible client.
// Default documented base URL is http://localhost:20128/v1 but callers supply it.
func New(baseURL, apiKey string, tlsVerify bool, timeout time.Duration) *openai.Client {
	return openai.NewClient("nine", baseURL, apiKey, "bearer", tlsVerify, nil, timeout)
}
