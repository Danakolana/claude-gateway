package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danakolana/claude-gateway/pkg/api"
)

// ChatRequest is the OpenAI / OpenRouter chat completions body.
type ChatRequest struct {
	Model         string         `json:"model"`
	Messages      []ChatMessage  `json:"messages"`
	Stream        bool           `json:"stream,omitempty"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
	// Usage asks OpenRouter to include token/cost accounting on stream chunks.
	Usage *struct {
		Include bool `json:"include"`
	} `json:"usage,omitempty"`
	Tools        []ChatTool      `json:"tools,omitempty"`
	ToolChoice   any             `json:"tool_choice,omitempty"`
	MaxTokens    int             `json:"max_tokens,omitempty"`
	Temperature  *float64        `json:"temperature,omitempty"`
	TopP         *float64        `json:"top_p,omitempty"`
	Stop         []string        `json:"stop,omitempty"`
	CacheControl map[string]any  `json:"cache_control,omitempty"`
	Reasoning    map[string]any  `json:"reasoning,omitempty"`
	// Provider is OpenRouter provider routing (optional).
	Provider map[string]any `json:"provider,omitempty"`
}

type ChatMessage struct {
	Role             string         `json:"role"`
	Content          any            `json:"content,omitempty"`
	ToolCalls        []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	Reasoning        string         `json:"reasoning,omitempty"`
	ReasoningDetails []any          `json:"reasoning_details,omitempty"`
	CacheControl     map[string]any `json:"cache_control,omitempty"`
}

type ChatTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description,omitempty"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
	CacheControl map[string]any `json:"cache_control,omitempty"`
}

type ToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type ChatResponse struct {
	ID       string `json:"id"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
	Choices  []struct {
		Message      ChatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage chatUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

// chatUsage is OpenAI/OpenRouter usage accounting (optional detail fields).
type chatUsage struct {
	PromptTokens     int      `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
	TotalTokens      int      `json:"total_tokens"`
	Cost             *float64 `json:"cost"`
	PromptTokensDetails *struct {
		CachedTokens     int `json:"cached_tokens"`
		CacheWriteTokens int `json:"cache_write_tokens"`
		AudioTokens      int `json:"audio_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
		AudioTokens     int `json:"audio_tokens"`
	} `json:"completion_tokens_details"`
}

func (u chatUsage) toAPI() api.Usage {
	out := api.Usage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
	}
	if u.PromptTokensDetails != nil {
		out.CachedTokens = u.PromptTokensDetails.CachedTokens
		out.CacheWriteTokens = u.PromptTokensDetails.CacheWriteTokens
	}
	if u.CompletionTokensDetails != nil {
		out.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	if u.Cost != nil {
		out.ProviderCostUSD = *u.Cost
		out.HasProviderCost = true
	}
	return out
}

// EncodeRequest maps canonical → OpenAI-compatible (OpenRouter).
// Forwards cache_control and thinking/reasoning; rejects structured output.
func EncodeRequest(req api.Request) ([]byte, error) {
	if req.Requirements.Structured {
		return nil, &api.Error{Category: api.ErrUnsupportedCapability, Message: "structured output not supported"}
	}
	out := ChatRequest{
		Model: req.TargetModel, Stream: req.Stream, MaxTokens: req.MaxTokens,
		Stop: req.StopSequences, Temperature: req.Temperature, TopP: req.TopP,
		ToolChoice: mapToolChoice(req.ToolChoice), CacheControl: req.CacheControl,
		Provider: req.UpstreamProvider,
	}
	if req.Stream {
		out.StreamOptions = &struct {
			IncludeUsage bool `json:"include_usage"`
		}{IncludeUsage: true}
		out.Usage = &struct {
			Include bool `json:"include"`
		}{Include: true}
	}
	if r := mapReasoning(req); len(r) > 0 {
		out.Reasoning = r
	}
	if len(req.SystemBlocks) > 0 {
		parts := make([]map[string]any, 0, len(req.SystemBlocks))
		for _, b := range req.SystemBlocks {
			if b.Type != api.BlockText {
				continue
			}
			p := map[string]any{"type": "text", "text": b.Text}
			if len(b.CacheControl) > 0 {
				p["cache_control"] = b.CacheControl
			}
			parts = append(parts, p)
		}
		if len(parts) > 0 {
			out.Messages = append(out.Messages, ChatMessage{Role: "system", Content: parts})
		}
	} else if req.System != "" {
		out.Messages = append(out.Messages, ChatMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		cm, err := encodeMessage(m)
		if err != nil {
			return nil, err
		}
		out.Messages = append(out.Messages, cm...)
	}
	for _, t := range req.Tools {
		ct := ChatTool{Type: "function", CacheControl: t.CacheControl}
		ct.Function.Name = t.Name
		ct.Function.Description = t.Description
		ct.Function.Parameters = t.InputSchema
		out.Tools = append(out.Tools, ct)
	}
	return json.Marshal(out)
}

func mapReasoning(req api.Request) map[string]any {
	if req.Thinking == nil {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(req.Thinking.Type)) {
	case "", "disabled", "disabled_thinking", "none":
		// Omit the field. Sending reasoning.enabled=false breaks providers
		// where reasoning is mandatory (e.g. DeepSeek V4 Flash on OpenRouter).
		return nil
	case "enabled", "enabled_thinking", "true":
		r := map[string]any{"enabled": true}
		if req.Thinking.BudgetTokens > 0 {
			r["max_tokens"] = req.Thinking.BudgetTokens
		}
		return r
	default:
		r := map[string]any{"enabled": true}
		if req.Thinking.BudgetTokens > 0 {
			r["max_tokens"] = req.Thinking.BudgetTokens
		}
		return r
	}
}

func mapToolChoice(v any) any {
	if v == nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	typ, _ := m["type"].(string)
	switch typ {
	case "auto", "none":
		return typ
	case "any":
		return "required"
	case "tool":
		name, _ := m["name"].(string)
		if name == "" {
			return "required"
		}
		return map[string]any{
			"type":     "function",
			"function": map[string]any{"name": name},
		}
	default:
		return v
	}
}

func encodeMessage(m api.Message) ([]ChatMessage, error) {
	switch m.Role {
	case api.RoleTool:
		var out []ChatMessage
		for _, b := range m.Content {
			if b.Type != api.BlockToolResult {
				continue
			}
			out = append(out, ChatMessage{Role: "tool", ToolCallID: b.ToolUseID, Content: b.ToolContent, CacheControl: b.CacheControl})
		}
		return out, nil
	case api.RoleAssistant:
		cm := ChatMessage{Role: "assistant"}
		var texts []map[string]any
		var reasoningText []string
		var details []any
		for _, b := range m.Content {
			switch b.Type {
			case api.BlockThinking:
				if b.Redacted {
					details = append(details, map[string]any{
						"type": "reasoning.encrypted", "data": b.RedactedData,
					})
					continue
				}
				reasoningText = append(reasoningText, b.Text)
				d := map[string]any{"type": "reasoning.text", "text": b.Text}
				if b.Signature != "" {
					d["signature"] = b.Signature
				}
				details = append(details, d)
			case api.BlockText:
				p := map[string]any{"type": "text", "text": b.Text}
				if len(b.CacheControl) > 0 {
					p["cache_control"] = b.CacheControl
				}
				texts = append(texts, p)
			case api.BlockToolUse:
				input := b.ToolInput
				if input == nil {
					input = map[string]any{}
				}
				args, _ := json.Marshal(input)
				cm.ToolCalls = append(cm.ToolCalls, ToolCall{ID: b.ToolUseID, Type: "function"})
				cm.ToolCalls[len(cm.ToolCalls)-1].Function.Name = b.ToolName
				cm.ToolCalls[len(cm.ToolCalls)-1].Function.Arguments = string(args)
			}
		}
		if len(reasoningText) > 0 {
			cm.Reasoning = strings.Join(reasoningText, "\n")
		}
		if len(details) > 0 {
			cm.ReasoningDetails = details
		}
		if len(texts) == 1 {
			if _, hasCC := texts[0]["cache_control"]; !hasCC {
				cm.Content = texts[0]["text"]
			} else {
				cm.Content = texts
			}
		} else if len(texts) > 1 {
			cm.Content = texts
		}
		return []ChatMessage{cm}, nil
	default: // user (Anthropic puts tool_result blocks on user turns)
		var toolMsgs []ChatMessage
		var parts []map[string]any
		var plain string
		hasCC := false
		for _, b := range m.Content {
			switch b.Type {
			case api.BlockToolResult:
				toolMsgs = append(toolMsgs, ChatMessage{Role: "tool", ToolCallID: b.ToolUseID, Content: b.ToolContent, CacheControl: b.CacheControl})
			case api.BlockText:
				plain += b.Text
				p := map[string]any{"type": "text", "text": b.Text}
				if len(b.CacheControl) > 0 {
					p["cache_control"] = b.CacheControl
					hasCC = true
				}
				parts = append(parts, p)
			case api.BlockImage:
				url := b.URL
				if url == "" && b.DataBase64 != "" {
					mime := b.MIMEType
					if mime == "" {
						mime = "image/png"
					}
					url = "data:" + mime + ";base64," + b.DataBase64
				}
				p := map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": url},
				}
				if len(b.CacheControl) > 0 {
					p["cache_control"] = b.CacheControl
					hasCC = true
				}
				parts = append(parts, p)
			}
		}
		if len(parts) == 0 {
			return toolMsgs, nil
		}
		cm := ChatMessage{Role: "user"}
		if len(parts) == 1 && parts[0]["type"] == "text" && !hasCC {
			cm.Content = plain
		} else {
			cm.Content = parts
		}
		return append(toolMsgs, cm), nil
	}
}

// DecodeResponse maps OpenAI JSON → canonical.
func DecodeResponse(body []byte) (api.Response, error) {
	var cr ChatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return api.Response{}, &api.Error{Category: api.ErrProviderProtocol, Message: "malformed JSON: " + err.Error()}
	}
	if cr.Error != nil {
		return api.Response{Error: mapHTTPError(401, cr.Error.Message)}, nil
	}
	if len(cr.Choices) == 0 {
		return api.Response{}, &api.Error{Category: api.ErrProviderProtocol, Message: "no choices"}
	}
	ch := cr.Choices[0]
	var blocks []api.ContentBlock
	blocks = append(blocks, thinkingBlocksFromMessage(ch.Message)...)
	switch c := ch.Message.Content.(type) {
	case string:
		if c != "" {
			blocks = append(blocks, api.ContentBlock{Type: api.BlockText, Text: c})
		}
	case []any:
		for _, item := range c {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if m["type"] == "text" {
				if tx, ok := m["text"].(string); ok && tx != "" {
					blocks = append(blocks, api.ContentBlock{Type: api.BlockText, Text: tx})
				}
			}
		}
	}
	for _, tc := range ch.Message.ToolCalls {
		var input map[string]any
		args := strings.TrimSpace(tc.Function.Arguments)
		if args == "" {
			input = map[string]any{}
		} else if err := json.Unmarshal([]byte(args), &input); err != nil || input == nil {
			// Keep a parseable object so Anthropic clients don't reject the block.
			input = map[string]any{}
		}
		blocks = append(blocks, api.ContentBlock{
			Type: api.BlockToolUse, ToolUseID: tc.ID, ToolName: tc.Function.Name, ToolInput: input,
		})
	}
	usage := cr.Usage.toAPI()
	usage.UpstreamBackend = cr.Provider
	return api.Response{
		ID: cr.ID, Model: cr.Model, Content: blocks,
		FinishReason: mapFinish(ch.FinishReason),
		Usage:        usage,
	}, nil
}

func thinkingBlocksFromMessage(m ChatMessage) []api.ContentBlock {
	if len(m.ReasoningDetails) > 0 {
		var out []api.ContentBlock
		for _, raw := range m.ReasoningDetails {
			d, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := d["type"].(string)
			switch typ {
			case "reasoning.encrypted", "reasoning.redacted":
				data, _ := d["data"].(string)
				out = append(out, api.ContentBlock{Type: api.BlockThinking, Redacted: true, RedactedData: data})
			default: // reasoning.text and unknowns with text
				tx, _ := d["text"].(string)
				sig, _ := d["signature"].(string)
				if tx == "" && sig == "" {
					continue
				}
				out = append(out, api.ContentBlock{Type: api.BlockThinking, Text: tx, Signature: sig})
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if m.Reasoning != "" {
		return []api.ContentBlock{{Type: api.BlockThinking, Text: m.Reasoning}}
	}
	return nil
}

func mapFinish(s string) api.FinishReason {
	switch s {
	case "stop":
		return api.FinishEndTurn
	case "length":
		return api.FinishMaxTokens
	case "tool_calls", "function_call":
		return api.FinishToolUse
	default:
		return api.FinishEndTurn
	}
}

func mapHTTPError(status int, msg string) *api.Error {
	switch {
	case status == 401 || status == 403:
		return &api.Error{Category: api.ErrProviderAuth, Message: msg}
	case status == 429:
		return &api.Error{Category: api.ErrProviderRateLimit, Message: msg, Retryable: true}
	case status >= 500:
		return &api.Error{Category: api.ErrProviderTransient, Message: msg, Retryable: true}
	default:
		return &api.Error{Category: api.ErrProviderProtocol, Message: fmt.Sprintf("%d: %s", status, msg)}
	}
}
