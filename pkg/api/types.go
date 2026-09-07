package api

import "time"

// Capability tri-state.
type Cap int

const (
	CapUnknown Cap = iota
	CapUnsupported
	CapSupported
)

// Capabilities describes what a target model can do.
type Capabilities struct {
	Streaming  Cap
	Tools      Cap
	Vision     Cap
	Reasoning  Cap
	Structured Cap
	Context    int // 0 = unknown
}

// Requirements are hard requirements for a request.
type Requirements struct {
	Streaming  bool
	Tools      bool
	Vision     bool
	Reasoning  bool
	Structured bool
	MinContext int
}

// Compatible reports whether caps satisfy req. Unknown never counts as supported.
func (c Capabilities) Compatible(req Requirements) bool {
	check := func(need bool, have Cap) bool {
		if !need {
			return true
		}
		return have == CapSupported
	}
	if !check(req.Streaming, c.Streaming) || !check(req.Tools, c.Tools) ||
		!check(req.Vision, c.Vision) || !check(req.Reasoning, c.Reasoning) ||
		!check(req.Structured, c.Structured) {
		return false
	}
	if req.MinContext > 0 && c.Context > 0 && c.Context < req.MinContext {
		return false
	}
	return true
}

// Role of a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

// BlockType for content blocks.
type BlockType string

const (
	BlockText       BlockType = "text"
	BlockImage      BlockType = "image"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
	BlockThinking   BlockType = "thinking"
)

// ContentBlock is a canonical content unit.
type ContentBlock struct {
	Type        BlockType      `json:"type"`
	Text        string         `json:"text,omitempty"`
	MIMEType    string         `json:"mime_type,omitempty"`
	DataBase64  string         `json:"data_base64,omitempty"`
	URL         string         `json:"url,omitempty"`
	ToolUseID   string         `json:"tool_use_id,omitempty"`
	ToolName    string         `json:"tool_name,omitempty"`
	ToolInput   map[string]any `json:"tool_input,omitempty"`
	ToolContent string         `json:"tool_content,omitempty"` // untrusted
	IsError     bool           `json:"is_error,omitempty"`
}

// Message is an ordered turn.
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ToolDef is a tool definition.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

// FinishReason canonical stop reasons.
type FinishReason string

const (
	FinishEndTurn      FinishReason = "end_turn"
	FinishMaxTokens    FinishReason = "max_tokens"
	FinishToolUse      FinishReason = "tool_use"
	FinishStopSequence FinishReason = "stop_sequence"
	FinishCancelled    FinishReason = "cancelled"
	FinishError        FinishReason = "error"
)

// Usage token accounting.
type Usage struct {
	InputTokens  int            `json:"input_tokens"`
	OutputTokens int            `json:"output_tokens"`
	Extension    map[string]int `json:"extension,omitempty"`
}

// ErrorCategory stable taxonomy.
type ErrorCategory string

const (
	ErrInvalidConfig                ErrorCategory = "invalid_config"
	ErrUnsupportedClientIntegration ErrorCategory = "unsupported_client_integration"
	ErrUnsupportedCapability        ErrorCategory = "unsupported_capability"
	ErrProviderAuth                 ErrorCategory = "provider_auth"
	ErrProviderRateLimit            ErrorCategory = "provider_rate_limit"
	ErrProviderTransient            ErrorCategory = "provider_transient"
	ErrProviderProtocol             ErrorCategory = "provider_protocol"
	ErrRequestCancelled             ErrorCategory = "request_cancelled"
	ErrHistoryConflict              ErrorCategory = "history_conflict"
	ErrSyncAuth                     ErrorCategory = "sync_auth"
	ErrSyncChecksum                 ErrorCategory = "sync_checksum"
	ErrStorage                      ErrorCategory = "storage"
	ErrInternal                     ErrorCategory = "internal"
)

// Error is a safe machine-readable error.
type Error struct {
	Category    ErrorCategory `json:"category"`
	Message     string        `json:"message"`
	Retryable   bool          `json:"retryable"`
	Correlation string        `json:"correlation_id,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return "nil error"
	}
	return string(e.Category) + ": " + e.Message
}

// Request is the canonical inbound request.
type Request struct {
	ID              string         `json:"id"`
	SourceModel     string         `json:"source_model"`
	TargetModel     string         `json:"target_model,omitempty"`
	System          string         `json:"system,omitempty"`
	Messages        []Message      `json:"messages"`
	Tools           []ToolDef      `json:"tools,omitempty"`
	ToolChoice      any            `json:"tool_choice,omitempty"`
	Temperature     *float64       `json:"temperature,omitempty"`
	TopP            *float64       `json:"top_p,omitempty"`
	Stream          bool           `json:"stream"`
	MaxTokens       int            `json:"max_tokens,omitempty"`
	StopSequences   []string       `json:"stop_sequences,omitempty"`
	Requirements    Requirements   `json:"requirements"`
	Deadline        time.Time      `json:"deadline,omitempty"`
	Extension       map[string]any `json:"extension,omitempty"`
	EstimatedTokens int            `json:"estimated_tokens,omitempty"`
}

// Response is a non-streaming canonical response.
type Response struct {
	ID           string         `json:"id"`
	Model        string         `json:"model"`
	Content      []ContentBlock `json:"content"`
	FinishReason FinishReason   `json:"finish_reason"`
	Usage        Usage          `json:"usage"`
	Error        *Error         `json:"error,omitempty"`
	Extension    map[string]any `json:"extension,omitempty"`
}

// EventType for streaming.
type EventType string

const (
	EventTextDelta     EventType = "text_delta"
	EventThinkingDelta EventType = "thinking_delta"
	EventToolCallDelta EventType = "tool_call_delta"
	EventUsage         EventType = "usage"
	EventFinish        EventType = "finish"
	EventError         EventType = "error"
)

// Event is a streaming canonical event. Exactly one terminal (finish|error) per stream.
type Event struct {
	Type          EventType    `json:"type"`
	Text          string       `json:"text,omitempty"`
	ToolUseID     string       `json:"tool_use_id,omitempty"`
	ToolName      string       `json:"tool_name,omitempty"`
	ToolInputJSON string       `json:"tool_input_json,omitempty"`
	ToolIndex     int          `json:"tool_index,omitempty"` // OpenAI parallel tool_calls[].index
	Usage         *Usage       `json:"usage,omitempty"`
	FinishReason  FinishReason `json:"finish_reason,omitempty"`
	Error         *Error       `json:"error,omitempty"`
}

func (e Event) Terminal() bool {
	return e.Type == EventFinish || e.Type == EventError
}
