package styles

import "encoding/json"

// ================================================================================
// OpenAI Responses API Request Types
// ================================================================================

// ResponsesInputItem represents an input item in the Responses API
type ResponsesInputItem struct {
	Type    string `json:"type,omitempty"` // message, item_reference, etc.
	ID      string `json:"id,omitempty"`
	Role    string `json:"role,omitempty"`
	Content any    `json:"content,omitempty"` // string or []ContentPart
	Status  string `json:"status,omitempty"`
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
	Format *ChatCompletionsResponseFormat `json:"format,omitempty"`
}

// ResponsesRequest represents a full Responses API request
type ResponsesRequest struct {
	Model        string `json:"model"`
	Input        any    `json:"input"` // string, []ChatCompletionsMessage, or []ResponsesInputItem
	Instructions string `json:"instructions,omitempty"`

	// Streaming
	Stream bool `json:"stream,omitempty"`

	// Generation controls
	MaxOutputTokens int      `json:"max_output_tokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"top_p,omitempty"`

	// Tools
	Tools      []ResponsesTool `json:"tools,omitempty"`
	ToolChoice any             `json:"tool_choice,omitempty"`

	// Output
	Text           *ResponsesTextConfig           `json:"text,omitempty"`
	ResponseFormat *ChatCompletionsResponseFormat `json:"response_format,omitempty"`

	// Context
	PreviousResponseID string `json:"previous_response_id,omitempty"`

	// Store
	Store    *bool             `json:"store,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ================================================================================
// OpenAI Responses API Response Types
// ================================================================================

// ResponsesOutputItem represents an output item in the response
type ResponsesOutputItem struct {
	Type    string `json:"type,omitempty"` // message, function_call, function_call_output, etc.
	ID      string `json:"id,omitempty"`
	Status  string `json:"status,omitempty"`
	Role    string `json:"role,omitempty"`
	Content any    `json:"content,omitempty"`

	// For function calls
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

// ResponsesUsage represents token usage in Responses API
type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ResponsesResponse represents a full Responses API response
type ResponsesResponse struct {
	ID           string                `json:"id"`
	Object       string                `json:"object"`
	CreatedAt    int64                 `json:"created_at"`
	Model        string                `json:"model"`
	Output       []ResponsesOutputItem `json:"output"`
	Usage        *ResponsesUsage       `json:"usage,omitempty"`
	Status       string                `json:"status,omitempty"`
	StatusReason string                `json:"status_reason,omitempty"`
	Metadata     map[string]string     `json:"metadata,omitempty"`
}

// ================================================================================
// Parsing Helpers
// ================================================================================

// ParseResponsesRequest parses a request body into ResponsesRequest
func ParseResponsesRequest(reqBody []byte) (*ResponsesRequest, error) {
	var req ResponsesRequest
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// ParseResponsesResponse parses a response body into ResponsesResponse
func ParseResponsesResponse(resBody []byte) (*ResponsesResponse, error) {
	var res ResponsesResponse
	if err := json.Unmarshal(resBody, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
