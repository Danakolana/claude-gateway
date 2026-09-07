package openrouter

import (
	"time"

	"github.com/danakolana/claude-gateway/internal/provider/openai"
)

// New builds an OpenRouter OpenAI-compatible client. Host is never hard-coded
// beyond the caller-supplied baseURL.
func New(baseURL, apiKey string, headers map[string]string, tlsVerify bool, timeout time.Duration) *openai.Client {
	h := map[string]string{}
	for k, v := range headers {
		h[k] = v
	}
	return openai.NewClient("openrouter", baseURL, apiKey, "bearer", tlsVerify, h, timeout)
}
