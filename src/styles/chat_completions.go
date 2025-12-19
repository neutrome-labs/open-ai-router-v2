package styles

import "encoding/json"

// ================================================================================
// OpenAI Chat Completions API Request Types
// ================================================================================

// ChatCompletionsContentPart represents multimodal content in a message
type ChatCompletionsContentPart struct {
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

// ChatCompletionsTool represents a tool definition
type ChatCompletionsTool struct {
	Type     string                       `json:"type"`
	Function *ChatCompletionsToolFunction `json:"function,omitempty"`
}

// ChatCompletionsToolFunction defines a function for tool calling
type ChatCompletionsToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
	Strict      *bool  `json:"strict,omitempty"`
}

// ChatCompletionsToolCall represents a tool call in a message
type ChatCompletionsToolCall struct {
	Index    int    `json:"index"` // Index for streaming tool calls
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function *struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

// ChatCompletionsJSONSchema for structured outputs
type ChatCompletionsJSONSchema struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Schema      any    `json:"schema"`
	Strict      *bool  `json:"strict,omitempty"`
}

// ChatCompletionsStreamOptions for streaming configuration
type ChatCompletionsStreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// ChatCompletionsResponseFormat specifies output format constraints
type ChatCompletionsResponseFormat struct {
	Type       string                     `json:"type"`
	JSONSchema *ChatCompletionsJSONSchema `json:"json_schema,omitempty"`
}

// ChatCompletionsMessage represents a chat message
type ChatCompletionsMessage struct {
	Role       string                    `json:"role,omitempty"`
	Name       string                    `json:"name,omitempty"`
	Content    any                       `json:"content,omitempty"` // string or []ChatCompletionsContentPart
	ToolCallID string                    `json:"tool_call_id,omitempty"`
	Refusal    string                    `json:"refusal,omitempty"`
	ToolCalls  []ChatCompletionsToolCall `json:"tool_calls,omitempty"`
}

// ChatCompletionsRequest represents a full Chat Completions API request
type ChatCompletionsRequest struct {
	Model    string                   `json:"model"`
	Messages []ChatCompletionsMessage `json:"messages"`

	// Streaming
	Stream        bool                          `json:"stream,omitempty"`
	StreamOptions *ChatCompletionsStreamOptions `json:"stream_options,omitempty"`

	// Generation controls
	MaxTokens           int      `json:"max_tokens,omitempty"`
	MaxCompletionTokens int      `json:"max_completion_tokens,omitempty"`
	Temperature         *float64 `json:"temperature,omitempty"`
	TopP                *float64 `json:"top_p,omitempty"`
	N                   int      `json:"n,omitempty"`
	Stop                any      `json:"stop,omitempty"` // string or []string
	PresencePenalty     *float64 `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64 `json:"frequency_penalty,omitempty"`
	LogitBias           any      `json:"logit_bias,omitempty"`
	User                string   `json:"user,omitempty"`
	Seed                *int     `json:"seed,omitempty"`
	Logprobs            *bool    `json:"logprobs,omitempty"`
	TopLogprobs         *int     `json:"top_logprobs,omitempty"`

	// Tools and function calling
	Tools            []ChatCompletionsTool `json:"tools,omitempty"`
	ToolChoice       any                   `json:"tool_choice,omitempty"`
	ParallelToolCall *bool                 `json:"parallel_tool_calls,omitempty"`

	// Output and formatting
	ResponseFormat *ChatCompletionsResponseFormat `json:"response_format,omitempty"`
}

// ================================================================================
// OpenAI Chat Completions API Response Types
// ================================================================================

// ChatCompletionsUsage represents token usage statistics
type ChatCompletionsUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletionsChoice represents a completion choice
type ChatCompletionsChoice struct {
	Index        int                     `json:"index"`
	Message      *ChatCompletionsMessage `json:"message,omitempty"`
	Delta        *ChatCompletionsMessage `json:"delta,omitempty"` // For streaming
	FinishReason string                  `json:"finish_reason,omitempty"`
	Logprobs     any                     `json:"logprobs,omitempty"`
}

// ChatCompletionsResponse represents a full Chat Completions API response
type ChatCompletionsResponse struct {
	ID                string                  `json:"id"`
	Object            string                  `json:"object"`
	Created           int64                   `json:"created"`
	Model             string                  `json:"model"`
	Choices           []ChatCompletionsChoice `json:"choices"`
	Usage             *ChatCompletionsUsage   `json:"usage,omitempty"`
	SystemFingerprint string                  `json:"system_fingerprint,omitempty"`
}

// ================================================================================
// Parsing Helpers
// ================================================================================

// ParseChatCompletionsRequest parses a request body into ChatCompletionsRequest
func ParseChatCompletionsRequest(reqBody []byte) (*ChatCompletionsRequest, error) {
	var req ChatCompletionsRequest
	if err := json.Unmarshal(reqBody, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// ParseChatCompletionsResponse parses a response body into ChatCompletionsResponse
func ParseChatCompletionsResponse(resBody []byte) (*ChatCompletionsResponse, error) {
	var res ChatCompletionsResponse
	if err := json.Unmarshal(resBody, &res); err != nil {
		return nil, err
	}
	return &res, nil
}
