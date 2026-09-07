package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/danakolana/claude-gateway/pkg/api"
)

// MessagesRequest is the Anthropic Messages API request shape.
type MessagesRequest struct {
	Model         string    `json:"model"`
	MaxTokens     int       `json:"max_tokens"`
	System        any       `json:"system,omitempty"`
	Messages      []Message `json:"messages"`
	Tools         []Tool    `json:"tools,omitempty"`
	Stream        bool      `json:"stream,omitempty"`
	StopSequences []string  `json:"stop_sequences,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

// DecodeRequest converts Anthropic JSON to canonical.
func DecodeRequest(body []byte) (api.Request, error) {
	var mr MessagesRequest
	if err := json.Unmarshal(body, &mr); err != nil {
		return api.Request{}, &api.Error{Category: api.ErrProviderProtocol, Message: "invalid JSON: " + err.Error()}
	}
	if mr.Model == "" {
		return api.Request{}, &api.Error{Category: api.ErrInvalidConfig, Message: "model required"}
	}
	if mr.MaxTokens <= 0 {
		return api.Request{}, &api.Error{Category: api.ErrInvalidConfig, Message: "max_tokens required"}
	}
	req := api.Request{
		SourceModel: mr.Model, MaxTokens: mr.MaxTokens, Stream: mr.Stream, StopSequences: mr.StopSequences,
		Requirements: api.Requirements{Streaming: mr.Stream},
	}
	req.System = systemText(mr.System)
	for _, m := range mr.Messages {
		msg, err := decodeMessage(m)
		if err != nil {
			return api.Request{}, err
		}
		req.Messages = append(req.Messages, msg)
	}
	for _, t := range mr.Tools {
		req.Tools = append(req.Tools, api.ToolDef{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
		req.Requirements.Tools = true
	}
	for _, m := range req.Messages {
		for _, b := range m.Content {
			if b.Type == api.BlockThinking {
				return api.Request{}, &api.Error{Category: api.ErrUnsupportedCapability, Message: "thinking blocks not supported in MVP"}
			}
			if b.Type == api.BlockImage {
				req.Requirements.Vision = true
			}
		}
	}
	return req, nil
}

func systemText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var s string
		for _, item := range t {
			if m, ok := item.(map[string]any); ok {
				if m["type"] == "text" {
					if tx, ok := m["text"].(string); ok {
						s += tx
					}
				}
			}
		}
		return s
	default:
		return ""
	}
}

func decodeMessage(m Message) (api.Message, error) {
	role := api.Role(m.Role)
	msg := api.Message{Role: role}
	switch c := m.Content.(type) {
	case string:
		msg.Content = []api.ContentBlock{{Type: api.BlockText, Text: c}}
	case []any:
		for _, item := range c {
			b, err := decodeBlock(item)
			if err != nil {
				return msg, err
			}
			msg.Content = append(msg.Content, b)
		}
	default:
		return msg, &api.Error{Category: api.ErrProviderProtocol, Message: "unsupported content shape"}
	}
	return msg, nil
}

func decodeBlock(item any) (api.ContentBlock, error) {
	m, ok := item.(map[string]any)
	if !ok {
		return api.ContentBlock{}, &api.Error{Category: api.ErrProviderProtocol, Message: "block must be object"}
	}
	typ, _ := m["type"].(string)
	switch typ {
	case "text":
		tx, _ := m["text"].(string)
		return api.ContentBlock{Type: api.BlockText, Text: tx}, nil
	case "thinking":
		tx, _ := m["thinking"].(string)
		return api.ContentBlock{Type: api.BlockThinking, Text: tx}, nil
	case "tool_use":
		id, _ := m["id"].(string)
		name, _ := m["name"].(string)
		input, _ := m["input"].(map[string]any)
		return api.ContentBlock{Type: api.BlockToolUse, ToolUseID: id, ToolName: name, ToolInput: input}, nil
	case "tool_result":
		id, _ := m["tool_use_id"].(string)
		content := fmt.Sprint(m["content"])
		errFlag, _ := m["is_error"].(bool)
		return api.ContentBlock{Type: api.BlockToolResult, ToolUseID: id, ToolContent: content, IsError: errFlag}, nil
	case "image":
		src, _ := m["source"].(map[string]any)
		b := api.ContentBlock{Type: api.BlockImage}
		if src != nil {
			b.MIMEType, _ = src["media_type"].(string)
			b.DataBase64, _ = src["data"].(string)
			if src["type"] == "url" {
				b.URL, _ = src["url"].(string)
			}
		}
		return b, nil
	default:
		return api.ContentBlock{}, &api.Error{Category: api.ErrUnsupportedCapability, Message: "unsupported content type: " + typ}
	}
}

// EncodeResponse maps canonical → Anthropic messages response.
func EncodeResponse(resp api.Response) ([]byte, error) {
	if resp.Error != nil {
		return json.Marshal(map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    string(resp.Error.Category),
				"message": resp.Error.Message,
			},
		})
	}
	var content []map[string]any
	for _, b := range resp.Content {
		switch b.Type {
		case api.BlockText:
			content = append(content, map[string]any{"type": "text", "text": b.Text})
		case api.BlockToolUse:
			content = append(content, map[string]any{
				"type": "tool_use", "id": b.ToolUseID, "name": b.ToolName, "input": b.ToolInput,
			})
		}
	}
	stop := mapFinish(resp.FinishReason)
	return json.Marshal(map[string]any{
		"id": resp.ID, "type": "message", "role": "assistant", "model": resp.Model,
		"content": content, "stop_reason": stop, "stop_sequence": nil,
		"usage": map[string]any{"input_tokens": resp.Usage.InputTokens, "output_tokens": resp.Usage.OutputTokens},
	})
}

func mapFinish(f api.FinishReason) string {
	switch f {
	case api.FinishMaxTokens:
		return "max_tokens"
	case api.FinishToolUse:
		return "tool_use"
	case api.FinishStopSequence:
		return "stop_sequence"
	default:
		return "end_turn"
	}
}

// EncodeEvent writes one Anthropic SSE event payload (without data: prefix).
func EncodeEvent(e api.Event) (eventName string, payload []byte, terminal bool) {
	switch e.Type {
	case api.EventTextDelta:
		payload, _ = json.Marshal(map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{"type": "text_delta", "text": e.Text},
		})
		return "content_block_delta", payload, false
	case api.EventFinish:
		payload, _ = json.Marshal(map[string]any{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": mapFinish(e.FinishReason)},
			"usage": map[string]any{"output_tokens": 0},
		})
		return "message_delta", payload, false
	case api.EventError:
		msg := "error"
		if e.Error != nil {
			msg = e.Error.Message
		}
		payload, _ = json.Marshal(map[string]any{"type": "error", "error": map[string]any{"type": "api_error", "message": msg}})
		return "error", payload, true
	default:
		payload, _ = json.Marshal(map[string]any{"type": "ping"})
		return "ping", payload, false
	}
}
