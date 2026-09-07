package custom

import (
	"time"

	"github.com/danakolana/claude-gateway/internal/provider/openai"
)

// New builds a custom OpenAI-compatible gateway client.
func New(baseURL, apiKey, authScheme string, headers map[string]string, tlsVerify bool, timeout time.Duration) *openai.Client {
	if authScheme == "" {
		authScheme = "bearer"
	}
	return openai.NewClient("custom", baseURL, apiKey, authScheme, tlsVerify, headers, timeout)
}
