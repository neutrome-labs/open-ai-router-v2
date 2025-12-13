package formats

import (
	"encoding/json"
)

// OpenAIResponsesRequest represents OpenAI Responses API request format
type OpenAIResponsesRequest struct {
	Model        string `json:"model"`
	Input        any    `json:"input"` // string, []Message, or []InputItem
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

	// Context
	PreviousResponseID string `json:"previous_response_id,omitempty"`

	// Output
	Text           *ResponsesTextConfig `json:"text,omitempty"`
	ResponseFormat *ResponseFormat      `json:"response_format,omitempty"`

	// Misc
	User            string   `json:"user,omitempty"`
	Store           bool     `json:"store,omitempty"`
	Metadata        any      `json:"metadata,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
	Truncation      string   `json:"truncation,omitempty"`
	Include         []string `json:"include,omitempty"`

	extras map[string]json.RawMessage
}

// ResponsesTool represents a tool in the Responses API
type ResponsesTool struct {
	Type        string        `json:"type"` // function, file_search, code_interpreter, computer_use, etc.
	Function    *ToolFunction `json:"function,omitempty"`
	FileSearch  any           `json:"file_search,omitempty"`
	ComputerUse any           `json:"computer_use,omitempty"`
}

// ResponsesTextConfig configures text output
type ResponsesTextConfig struct {
	Format *ResponseFormat `json:"format,omitempty"`
}

func (r *OpenAIResponsesRequest) GetModel() string      { return r.Model }
func (r *OpenAIResponsesRequest) SetModel(model string) { r.Model = model }
func (r *OpenAIResponsesRequest) IsStreaming() bool     { return r.Stream }

// GetMessages converts Input to Messages if possible
func (r *OpenAIResponsesRequest) GetMessages() []Message {
	// Input can be: string, []Message, or []InputItem
	if msgs, ok := r.Input.([]Message); ok {
		return msgs
	}
	if items, ok := r.Input.([]any); ok {
		var msgs []Message
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				msg := Message{}
				if role, ok := m["role"].(string); ok {
					msg.Role = role
				}
				if content, ok := m["content"]; ok {
					msg.Content = content
				}
				msgs = append(msgs, msg)
			}
		}
		return msgs
	}
	// String input becomes a user message
	if str, ok := r.Input.(string); ok {
		return []Message{{Role: "user", Content: str}}
	}
	return nil
}

func (r *OpenAIResponsesRequest) SetMessages(msgs []Message) {
	r.Input = msgs
}

func (r *OpenAIResponsesRequest) GetRawExtras() map[string]json.RawMessage       { return r.extras }
func (r *OpenAIResponsesRequest) SetRawExtras(extras map[string]json.RawMessage) { r.extras = extras }

// Clone creates a deep copy of the request
func (r *OpenAIResponsesRequest) Clone() ManagedRequest {
	clone := &OpenAIResponsesRequest{
		Model:              r.Model,
		Input:              r.Input,
		Instructions:       r.Instructions,
		Stream:             r.Stream,
		MaxOutputTokens:    r.MaxOutputTokens,
		Tools:              r.Tools,
		ToolChoice:         r.ToolChoice,
		PreviousResponseID: r.PreviousResponseID,
		Text:               r.Text,
		ResponseFormat:     r.ResponseFormat,
		User:               r.User,
		Store:              r.Store,
		Metadata:           r.Metadata,
		ReasoningEffort:    r.ReasoningEffort,
		Truncation:         r.Truncation,
	}

	// Copy slices
	if r.Include != nil {
		clone.Include = make([]string, len(r.Include))
		copy(clone.Include, r.Include)
	}

	// Copy pointer fields
	if r.Temperature != nil {
		v := *r.Temperature
		clone.Temperature = &v
	}
	if r.TopP != nil {
		v := *r.TopP
		clone.TopP = &v
	}

	// Deep copy extras
	if r.extras != nil {
		clone.extras = make(map[string]json.RawMessage, len(r.extras))
		for k, v := range r.extras {
			clone.extras[k] = v
		}
	}

	return clone
}

func (r *OpenAIResponsesRequest) FromJSON(data []byte) error {
	if err := json.Unmarshal(data, r); err != nil {
		return err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.extras = make(map[string]json.RawMessage)
	for k, v := range raw {
		if !openAIResponsesKnownFields[k] {
			r.extras[k] = v
		}
	}

	return nil
}

// openAIResponsesKnownFields lists fields handled by OpenAIResponsesRequest struct
var openAIResponsesKnownFields = map[string]bool{
	"model": true, "input": true, "instructions": true, "stream": true,
	"max_output_tokens": true, "temperature": true, "top_p": true,
	"tools": true, "tool_choice": true, "previous_response_id": true,
	"text": true, "response_format": true, "user": true, "store": true,
	"metadata": true, "reasoning_effort": true, "truncation": true, "include": true,
}

func (r *OpenAIResponsesRequest) ToJSON() ([]byte, error) {
	out := make(map[string]any)

	out["model"] = r.Model
	out["input"] = r.Input

	if r.Instructions != "" {
		out["instructions"] = r.Instructions
	}
	if r.Stream {
		out["stream"] = r.Stream
	}
	if r.MaxOutputTokens > 0 {
		out["max_output_tokens"] = r.MaxOutputTokens
	}
	if r.Temperature != nil {
		out["temperature"] = *r.Temperature
	}
	if r.TopP != nil {
		out["top_p"] = *r.TopP
	}
	if len(r.Tools) > 0 {
		out["tools"] = r.Tools
	}
	if r.ToolChoice != nil {
		out["tool_choice"] = r.ToolChoice
	}
	if r.PreviousResponseID != "" {
		out["previous_response_id"] = r.PreviousResponseID
	}
	if r.Text != nil {
		out["text"] = r.Text
	}
	if r.ResponseFormat != nil {
		out["response_format"] = r.ResponseFormat
	}
	if r.User != "" {
		out["user"] = r.User
	}
	if r.Store {
		out["store"] = r.Store
	}
	if r.Metadata != nil {
		out["metadata"] = r.Metadata
	}
	if r.ReasoningEffort != "" {
		out["reasoning_effort"] = r.ReasoningEffort
	}
	if r.Truncation != "" {
		out["truncation"] = r.Truncation
	}
	if len(r.Include) > 0 {
		out["include"] = r.Include
	}

	for k, v := range r.extras {
		var val any
		if err := json.Unmarshal(v, &val); err == nil {
			out[k] = val
		}
	}

	return json.Marshal(out)
}

func (r *OpenAIResponsesRequest) MergeFrom(raw []byte) error {
	var incoming map[string]json.RawMessage
	if err := json.Unmarshal(raw, &incoming); err != nil {
		return err
	}

	if r.extras == nil {
		r.extras = make(map[string]json.RawMessage)
	}
	for k, v := range incoming {
		if openAIResponsesKnownFields[k] {
			continue // Skip known fields - they are managed separately
		}
		if _, exists := r.extras[k]; !exists {
			r.extras[k] = v
		}
	}
	return nil
}

// OpenAIResponsesResponse represents OpenAI Responses API response format
type OpenAIResponsesResponse struct {
	ID                 string                     `json:"id,omitempty"`
	Object             string                     `json:"object,omitempty"`
	CreatedAt          int64                      `json:"created_at,omitempty"`
	Model              string                     `json:"model,omitempty"`
	Status             string                     `json:"status,omitempty"`
	StatusDetails      any                        `json:"status_details,omitempty"`
	Output             []ResponsesOutputItem      `json:"output,omitempty"`
	Usage              *ResponsesUsage            `json:"usage,omitempty"`
	Metadata           any                        `json:"metadata,omitempty"`
	IncompleteDetails  any                        `json:"incomplete_details,omitempty"`
	Instructions       string                     `json:"instructions,omitempty"`
	PreviousResponseID string                     `json:"previous_response_id,omitempty"`
	Reasoning          *ResponsesReasoningContent `json:"reasoning,omitempty"`
	Error              any                        `json:"error,omitempty"`

	extras map[string]json.RawMessage
}

// ResponsesOutputItem represents an output item in the response
type ResponsesOutputItem struct {
	Type    string                 `json:"type"`
	ID      string                 `json:"id,omitempty"`
	Status  string                 `json:"status,omitempty"`
	Role    string                 `json:"role,omitempty"`
	Content []ResponsesContentPart `json:"content,omitempty"`

	// For function calls
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

// ResponsesContentPart represents a content part in the output
type ResponsesContentPart struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Annotations []any  `json:"annotations,omitempty"`
}

// ResponsesUsage represents usage in the Responses API
type ResponsesUsage struct {
	InputTokens         int                  `json:"input_tokens,omitempty"`
	OutputTokens        int                  `json:"output_tokens,omitempty"`
	TotalTokens         int                  `json:"total_tokens,omitempty"`
	OutputTokensDetails *OutputTokensDetails `json:"output_tokens_details,omitempty"`
	InputTokensDetails  *InputTokensDetails  `json:"input_tokens_details,omitempty"`
}

// OutputTokensDetails provides detailed output token breakdown
type OutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// InputTokensDetails provides detailed input token breakdown
type InputTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
}

// ResponsesReasoningContent represents reasoning content
type ResponsesReasoningContent struct {
	ID      string                 `json:"id,omitempty"`
	Content []ResponsesContentPart `json:"content,omitempty"`
}

func (r *OpenAIResponsesResponse) GetModel() string {
	return r.Model
}

func (r *OpenAIResponsesResponse) GetUsage() *Usage {
	if r.Usage == nil {
		return nil
	}
	return &Usage{
		PromptTokens:     r.Usage.InputTokens,
		CompletionTokens: r.Usage.OutputTokens,
		TotalTokens:      r.Usage.TotalTokens,
	}
}

func (r *OpenAIResponsesResponse) SetUsage(usage *Usage) {
	if usage == nil {
		r.Usage = nil
		return
	}
	r.Usage = &ResponsesUsage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
		TotalTokens:  usage.TotalTokens,
	}
}

func (r *OpenAIResponsesResponse) GetChoices() []Choice {
	// Convert Responses API output items to OpenAI-style choices
	var choices []Choice

	for i, item := range r.Output {
		if item.Type == "message" {
			var content string
			for _, part := range item.Content {
				if part.Type == "output_text" || part.Type == "text" {
					content += part.Text
				}
			}
			choices = append(choices, Choice{
				Index: i,
				Message: &Message{
					Role:    item.Role,
					Content: content,
				},
				FinishReason: r.Status,
			})
		}
	}

	return choices
}

func (r *OpenAIResponsesResponse) IsChunk() bool                            { return false } // Streaming uses different event types
func (r *OpenAIResponsesResponse) GetRawExtras() map[string]json.RawMessage { return r.extras }

func (r *OpenAIResponsesResponse) FromJSON(data []byte) error {
	if err := json.Unmarshal(data, r); err != nil {
		return err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	knownFields := map[string]bool{
		"id": true, "object": true, "created_at": true, "model": true,
		"status": true, "status_details": true, "output": true, "usage": true,
		"metadata": true, "incomplete_details": true, "instructions": true,
		"previous_response_id": true, "reasoning": true, "error": true,
	}

	r.extras = make(map[string]json.RawMessage)
	for k, v := range raw {
		if !knownFields[k] {
			r.extras[k] = v
		}
	}

	return nil
}

func (r *OpenAIResponsesResponse) ToJSON() ([]byte, error) {
	out := make(map[string]any)

	if r.ID != "" {
		out["id"] = r.ID
	}
	if r.Object != "" {
		out["object"] = r.Object
	}
	if r.CreatedAt != 0 {
		out["created_at"] = r.CreatedAt
	}
	if r.Model != "" {
		out["model"] = r.Model
	}
	if r.Status != "" {
		out["status"] = r.Status
	}
	if r.StatusDetails != nil {
		out["status_details"] = r.StatusDetails
	}
	if len(r.Output) > 0 {
		out["output"] = r.Output
	}
	if r.Usage != nil {
		out["usage"] = r.Usage
	}
	if r.Metadata != nil {
		out["metadata"] = r.Metadata
	}
	if r.IncompleteDetails != nil {
		out["incomplete_details"] = r.IncompleteDetails
	}
	if r.Instructions != "" {
		out["instructions"] = r.Instructions
	}
	if r.PreviousResponseID != "" {
		out["previous_response_id"] = r.PreviousResponseID
	}
	if r.Reasoning != nil {
		out["reasoning"] = r.Reasoning
	}
	if r.Error != nil {
		out["error"] = r.Error
	}

	for k, v := range r.extras {
		var val any
		if err := json.Unmarshal(v, &val); err == nil {
			out[k] = val
		}
	}

	return json.Marshal(out)
}
