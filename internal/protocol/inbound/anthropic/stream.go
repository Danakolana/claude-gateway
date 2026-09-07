package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/danakolana/claude-gateway/pkg/api"
)

// Frame is one Anthropic SSE event (event name + JSON data payload).
type Frame struct {
	Event string
	Data  []byte
}

// StreamEncoder converts canonical stream events into Anthropic Messages SSE.
// It opens text/tool content blocks lazily and always closes them before stop.
type StreamEncoder struct {
	MsgID string
	Model string

	nextIndex int
	textOpen  bool
	textIndex int

	toolOpen  map[int]bool
	toolBlock map[int]int
	toolOrder []int
}

// NewStreamEncoder builds an encoder for one assistant message stream.
func NewStreamEncoder(msgID, model string) *StreamEncoder {
	return &StreamEncoder{
		MsgID: msgID, Model: model,
		toolOpen: map[int]bool{}, toolBlock: map[int]int{},
	}
}

// Begin emits message_start only (no empty text block).
func (s *StreamEncoder) Begin() []Frame {
	payload, _ := json.Marshal(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": s.MsgID, "type": "message", "role": "assistant", "model": s.Model,
			"content": []any{}, "usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	})
	return []Frame{{Event: "message_start", Data: payload}}
}

// Push maps one canonical event to zero or more Anthropic SSE frames.
func (s *StreamEncoder) Push(e api.Event) []Frame {
	switch e.Type {
	case api.EventTextDelta:
		var out []Frame
		out = append(out, s.ensureText()...)
		payload, _ := json.Marshal(map[string]any{
			"type": "content_block_delta", "index": s.textIndex,
			"delta": map[string]any{"type": "text_delta", "text": e.Text},
		})
		return append(out, Frame{Event: "content_block_delta", Data: payload})
	case api.EventToolCallDelta:
		return s.pushTool(e)
	case api.EventFinish:
		return s.finish(e.FinishReason)
	case api.EventError:
		msg := "error"
		if e.Error != nil {
			msg = e.Error.Message
		}
		payload, _ := json.Marshal(map[string]any{
			"type": "error", "error": map[string]any{"type": "api_error", "message": msg},
		})
		return []Frame{{Event: "error", Data: payload}}
	default:
		return nil
	}
}

func (s *StreamEncoder) ensureText() []Frame {
	if s.textOpen {
		return nil
	}
	s.textIndex = s.nextIndex
	s.nextIndex++
	s.textOpen = true
	payload, _ := json.Marshal(map[string]any{
		"type": "content_block_start", "index": s.textIndex,
		"content_block": map[string]any{"type": "text", "text": ""},
	})
	return []Frame{{Event: "content_block_start", Data: payload}}
}

func (s *StreamEncoder) closeText() []Frame {
	if !s.textOpen {
		return nil
	}
	s.textOpen = false
	payload, _ := json.Marshal(map[string]any{"type": "content_block_stop", "index": s.textIndex})
	return []Frame{{Event: "content_block_stop", Data: payload}}
}

func (s *StreamEncoder) pushTool(e api.Event) []Frame {
	idx := e.ToolIndex
	var out []Frame
	out = append(out, s.closeText()...)
	if !s.toolOpen[idx] {
		id := e.ToolUseID
		if id == "" {
			id = fmt.Sprintf("toolu_%s_%d", s.MsgID, idx)
		}
		name := e.ToolName
		if name == "" {
			name = "tool"
		}
		blockIdx := s.nextIndex
		s.nextIndex++
		s.toolOpen[idx] = true
		s.toolBlock[idx] = blockIdx
		s.toolOrder = append(s.toolOrder, idx)
		start, _ := json.Marshal(map[string]any{
			"type": "content_block_start", "index": blockIdx,
			"content_block": map[string]any{
				"type": "tool_use", "id": id, "name": name, "input": map[string]any{},
			},
		})
		out = append(out, Frame{Event: "content_block_start", Data: start})
	}
	if e.ToolInputJSON != "" {
		payload, _ := json.Marshal(map[string]any{
			"type": "content_block_delta", "index": s.toolBlock[idx],
			"delta": map[string]any{"type": "input_json_delta", "partial_json": e.ToolInputJSON},
		})
		out = append(out, Frame{Event: "content_block_delta", Data: payload})
	}
	return out
}

func (s *StreamEncoder) closeTools() []Frame {
	var out []Frame
	for _, idx := range s.toolOrder {
		if !s.toolOpen[idx] {
			continue
		}
		s.toolOpen[idx] = false
		payload, _ := json.Marshal(map[string]any{"type": "content_block_stop", "index": s.toolBlock[idx]})
		out = append(out, Frame{Event: "content_block_stop", Data: payload})
	}
	s.toolOrder = nil
	return out
}

func (s *StreamEncoder) finish(reason api.FinishReason) []Frame {
	var out []Frame
	out = append(out, s.closeText()...)
	out = append(out, s.closeTools()...)
	delta, _ := json.Marshal(map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": mapFinish(reason)},
		"usage": map[string]any{"output_tokens": 0},
	})
	out = append(out, Frame{Event: "message_delta", Data: delta})
	stop, _ := json.Marshal(map[string]any{"type": "message_stop"})
	out = append(out, Frame{Event: "message_stop", Data: stop})
	return out
}
