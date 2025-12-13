// Package formats provides managed data structures for AI API requests/responses.
// V3 upgrade: Structures are deserialized once at request start and serialized once at end,
// minimizing intermediate JSON marshaling. Supports merging for passthrough scenarios.
package formats

import (
	"encoding/json"
)

// Format is the base interface for all format types
type Format interface {
	FromJSON(data []byte) error
	ToJSON() ([]byte, error)
}

// ManagedRequest represents a managed request that can be modified and merged
type ManagedRequest interface {
	Format
	// GetModel returns the model name from the request
	GetModel() string
	// SetModel updates the model name
	SetModel(model string)
	// GetMessages returns the messages in the request
	GetMessages() []Message
	// SetMessages updates the messages
	SetMessages(messages []Message)
	// IsStreaming returns true if this is a streaming request
	IsStreaming() bool
	// GetRawExtras returns extra fields not explicitly handled by the struct
	GetRawExtras() map[string]json.RawMessage
	// SetRawExtras sets extra fields
	SetRawExtras(extras map[string]json.RawMessage)
	// MergeFrom merges provider-specific extras from another raw JSON
	MergeFrom(raw []byte) error
	// Clone creates a deep copy of the request for isolated plugin processing
	Clone() ManagedRequest
}

// ManagedResponse represents a managed response
type ManagedResponse interface {
	Format
	// GetModel returns the model name from the response
	GetModel() string
	// GetUsage returns usage information if available
	GetUsage() *Usage
	// SetUsage sets usage information
	SetUsage(usage *Usage)
	// GetChoices returns the response choices
	GetChoices() []Choice
	// IsChunk returns true if this is a streaming chunk
	IsChunk() bool
	// GetRawExtras returns extra fields
	GetRawExtras() map[string]json.RawMessage
}

// Usage represents token usage information
type Usage struct {
	PromptTokens             int `json:"prompt_tokens,omitempty"`
	CompletionTokens         int `json:"completion_tokens,omitempty"`
	TotalTokens              int `json:"total_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

// Message represents a chat message in a unified format
type Message struct {
	Role       string     `json:"role,omitempty"`
	Name       string     `json:"name,omitempty"`
	Content    any        `json:"content,omitempty"` // string or []ContentPart
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Refusal    string     `json:"refusal,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ContentPart represents multimodal content
type ContentPart struct {
	Type     string `json:"type,omitempty"`
	Text     string `json:"text,omitempty"`
	ImageURL *struct {
		URL    string `json:"url,omitempty"`
		Detail string `json:"detail,omitempty"`
	} `json:"image_url,omitempty"`
	InputAudio *struct {
		Data   string `json:"data,omitempty"`
		Format string `json:"format,omitempty"`
	} `json:"input_audio,omitempty"`
}

// Tool represents a tool definition
type Tool struct {
	Type     string        `json:"type"`
	Function *ToolFunction `json:"function,omitempty"`
}

// ToolFunction defines a function for tool calling
type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
	Strict      *bool  `json:"strict,omitempty"`
}

// ToolCall represents a tool call in a response
type ToolCall struct {
	Index    int    `json:"index,omitempty"` // Index for streaming tool calls
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function *struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

// ResponseFormat specifies output format constraints
type ResponseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

// JSONSchema for structured outputs
type JSONSchema struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Schema      any    `json:"schema"`
	Strict      *bool  `json:"strict,omitempty"`
}

// StreamOptions for streaming configuration
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// Choice represents a response choice
type Choice struct {
	Index        int      `json:"index"`
	Message      *Message `json:"message,omitempty"`
	Delta        *Message `json:"delta,omitempty"`
	FinishReason string   `json:"finish_reason,omitempty"`
	Logprobs     any      `json:"logprobs,omitempty"`
}
