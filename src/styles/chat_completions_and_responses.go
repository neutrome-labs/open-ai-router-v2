package styles

import (
	"encoding/json"
	"fmt"
)

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
	Index    int    `json:"index"` // Index for streaming tool calls - always included
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function *struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
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

// ResponseFormat specifies output format constraints
type ResponseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
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

type EnoughChatCompletionsRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`

	// Streaming
	Stream        bool           `json:"stream,omitempty"`
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`

	// Generation controls
	MaxTokens           int      `json:"max_tokens,omitempty"`
	MaxCompletionTokens int      `json:"max_completion_tokens,omitempty"`
	Temperature         *float64 `json:"temperature,omitempty"`

	// Tools and function calling
	Tools      []Tool `json:"tools,omitempty"`
	ToolChoice any    `json:"tool_choice,omitempty"`

	// Output and formatting
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

// ResponsesTool represents a tool in the Responses API
// Note: Responses API has a different format than Chat Completions - function tools have
// name/description/parameters at top level, not nested in a "function" object
type ResponsesTool struct {
	Type string `json:"type"` // function, file_search, code_interpreter, computer_use, etc.

	// For function tools (Responses API format - flat structure)
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
	Strict      *bool  `json:"strict,omitempty"`

	// For other tool types
	FileSearch  any `json:"file_search,omitempty"`
	ComputerUse any `json:"computer_use,omitempty"`
}

// ResponsesTextConfig configures text output
type ResponsesTextConfig struct {
	Format *ResponseFormat `json:"format,omitempty"`
}

type EnoughResponsesRequest struct {
	Model        string `json:"model"`
	Input        any    `json:"input"` // string, []Message, or []InputItem
	Instructions string `json:"instructions,omitempty"`

	// Streaming
	Stream bool `json:"stream,omitempty"`

	// Generation controls
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`

	// Tools
	Tools      []ResponsesTool `json:"tools,omitempty"`
	ToolChoice any             `json:"tool_choice,omitempty"`

	// Output
	Text           *ResponsesTextConfig `json:"text,omitempty"`
	ResponseFormat *ResponseFormat      `json:"response_format,omitempty"`
}

func ConvertChatCompletionsRequestToResponses(reqBody []byte) ([]byte, error) {
	var chatReq EnoughChatCompletionsRequest
	if err := json.Unmarshal(reqBody, &chatReq); err != nil {
		return nil, fmt.Errorf("ConvertChatCompletionsRequestToResponses: failed to unmarshal chat completions request: %w", err)
	}

	responsesReq := &EnoughResponsesRequest{
		Model: chatReq.Model,
		Input: chatReq.Messages,
		// Instructions:    "",
		Stream:          chatReq.Stream,
		MaxOutputTokens: chatReq.MaxTokens,
		Temperature:     chatReq.Temperature,
		ToolChoice:      chatReq.ToolChoice,
		// Text:            nil,
		// ResponseFormat:  nil,
	}

	for _, tool := range chatReq.Tools {
		respTool := ResponsesTool{
			Type: tool.Type,
		}
		if tool.Function != nil {
			respTool.Name = tool.Function.Name
			respTool.Description = tool.Function.Description
			respTool.Parameters = tool.Function.Parameters
			respTool.Strict = tool.Function.Strict
		}
		responsesReq.Tools = append(responsesReq.Tools, respTool)
	}

	responsesReqBody, err := json.Marshal(responsesReq)
	if err != nil {
		return nil, fmt.Errorf("ConvertChatCompletionsRequestToResponses: failed to marshal responses request: %w", err)
	}

	return responsesReqBody, nil
}

func ConvertResponsesResponseToChatCompletions(respBody []byte) ([]byte, error) {
	return respBody, nil // Passthrough for now
}

func ConvertResponsesResponseChunkToChatCompletions(chunkBody []byte) ([]byte, error) {
	return chunkBody, nil // Passthrough for now
}
