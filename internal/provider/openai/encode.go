package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danakolana/claude-gateway/pkg/api"
)

// ChatRequest is the OpenAI chat completions body.
type ChatRequest struct {
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	Stream    bool          `json:"stream,omitempty"`
	Tools     []ChatTool    `json:"tools,omitempty"`
	MaxTokens int           `json:"max_tokens,omitempty"`
	Stop      []string      `json:"stop,omitempty"`
}

type ChatMessage struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ChatTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description,omitempty"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
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
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message      ChatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

// EncodeRequest maps canonical → OpenAI. Rejects thinking/structured for MVP.
func EncodeRequest(req api.Request) ([]byte, error) {
	for _, m := range req.Messages {
		for _, b := range m.Content {
			if b.Type == api.BlockThinking {
				return nil, &api.Error{Category: api.ErrUnsupportedCapability, Message: "thinking blocks not supported in MVP outbound"}
			}
		}
	}
	if req.Requirements.Reasoning || req.Requirements.Structured {
		return nil, &api.Error{Category: api.ErrUnsupportedCapability, Message: "reasoning/structured output not supported in MVP"}
	}
	out := ChatRequest{
		Model:     req.TargetModel,
		Stream:    req.Stream,
		MaxTokens: req.MaxTokens,
		Stop:      req.StopSequences,
	}
	if req.System != "" {
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
		ct := ChatTool{Type: "function"}
		ct.Function.Name = t.Name
		ct.Function.Description = t.Description
		ct.Function.Parameters = t.InputSchema
		out.Tools = append(out.Tools, ct)
	}
	return json.Marshal(out)
}

func encodeMessage(m api.Message) ([]ChatMessage, error) {
	switch m.Role {
	case api.RoleTool:
		var out []ChatMessage
		for _, b := range m.Content {
			if b.Type != api.BlockToolResult {
				continue
			}
			out = append(out, ChatMessage{Role: "tool", ToolCallID: b.ToolUseID, Content: b.ToolContent})
		}
		return out, nil
	case api.RoleAssistant:
		cm := ChatMessage{Role: "assistant"}
		var texts []map[string]any
		for _, b := range m.Content {
			switch b.Type {
			case api.BlockText:
				texts = append(texts, map[string]any{"type": "text", "text": b.Text})
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
		if len(texts) == 1 {
			cm.Content = texts[0]["text"]
		} else if len(texts) > 1 {
			cm.Content = texts
		}
		return []ChatMessage{cm}, nil
	default: // user (Anthropic puts tool_result blocks on user turns)
		var toolMsgs []ChatMessage
		var parts []map[string]any
		var plain string
		for _, b := range m.Content {
			switch b.Type {
			case api.BlockToolResult:
				toolMsgs = append(toolMsgs, ChatMessage{Role: "tool", ToolCallID: b.ToolUseID, Content: b.ToolContent})
			case api.BlockText:
				plain += b.Text
				parts = append(parts, map[string]any{"type": "text", "text": b.Text})
			case api.BlockImage:
				url := b.URL
				if url == "" && b.DataBase64 != "" {
					mime := b.MIMEType
					if mime == "" {
						mime = "image/png"
					}
					url = "data:" + mime + ";base64," + b.DataBase64
				}
				parts = append(parts, map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": url},
				})
			}
		}
		if len(parts) == 0 {
			return toolMsgs, nil
		}
		cm := ChatMessage{Role: "user"}
		if len(parts) == 1 && parts[0]["type"] == "text" {
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
	if s, ok := ch.Message.Content.(string); ok && s != "" {
		blocks = append(blocks, api.ContentBlock{Type: api.BlockText, Text: s})
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
	return api.Response{
		ID: cr.ID, Model: cr.Model, Content: blocks,
		FinishReason: mapFinish(ch.FinishReason),
		Usage:        api.Usage{InputTokens: cr.Usage.PromptTokens, OutputTokens: cr.Usage.CompletionTokens},
	}, nil
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
