package formats

import (
	"encoding/json"
)

// AnthropicRequest represents Anthropic messages API request format
type AnthropicRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens"`

	// Optional
	System        any      `json:"system,omitempty"` // string or []ContentBlock
	StopSequences []string `json:"stop_sequences,omitempty"`
	Stream        bool     `json:"stream,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"top_p,omitempty"`
	TopK          *int     `json:"top_k,omitempty"`

	// Tools
	Tools      []AnthropicTool `json:"tools,omitempty"`
	ToolChoice any             `json:"tool_choice,omitempty"`

	// Metadata
	Metadata *struct {
		UserID string `json:"user_id,omitempty"`
	} `json:"metadata,omitempty"`

	extras map[string]json.RawMessage
}

// AnthropicTool represents an Anthropic tool definition
type AnthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

func (r *AnthropicRequest) GetModel() string           { return r.Model }
func (r *AnthropicRequest) SetModel(model string)      { r.Model = model }
func (r *AnthropicRequest) GetMessages() []Message     { return r.Messages }
func (r *AnthropicRequest) SetMessages(msgs []Message) { r.Messages = msgs }
func (r *AnthropicRequest) IsStreaming() bool          { return r.Stream }

func (r *AnthropicRequest) GetRawExtras() map[string]json.RawMessage       { return r.extras }
func (r *AnthropicRequest) SetRawExtras(extras map[string]json.RawMessage) { r.extras = extras }

// Clone creates a deep copy of the request
func (r *AnthropicRequest) Clone() ManagedRequest {
	clone := &AnthropicRequest{
		Model:      r.Model,
		MaxTokens:  r.MaxTokens,
		System:     r.System,
		Stream:     r.Stream,
		Tools:      r.Tools,
		ToolChoice: r.ToolChoice,
		Metadata:   r.Metadata,
	}

	// Deep copy messages
	if r.Messages != nil {
		clone.Messages = make([]Message, len(r.Messages))
		copy(clone.Messages, r.Messages)
	}

	// Copy slices
	if r.StopSequences != nil {
		clone.StopSequences = make([]string, len(r.StopSequences))
		copy(clone.StopSequences, r.StopSequences)
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
	if r.TopK != nil {
		v := *r.TopK
		clone.TopK = &v
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

func (r *AnthropicRequest) FromJSON(data []byte) error {
	if err := json.Unmarshal(data, r); err != nil {
		return err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.extras = make(map[string]json.RawMessage)
	for k, v := range raw {
		if !anthropicKnownFields[k] {
			r.extras[k] = v
		}
	}

	return nil
}

// anthropicKnownFields lists fields handled by AnthropicRequest struct
var anthropicKnownFields = map[string]bool{
	"model": true, "messages": true, "max_tokens": true, "system": true,
	"stop_sequences": true, "stream": true, "temperature": true,
	"top_p": true, "top_k": true, "tools": true, "tool_choice": true,
	"metadata": true,
}

func (r *AnthropicRequest) ToJSON() ([]byte, error) {
	out := make(map[string]any)

	out["model"] = r.Model
	out["messages"] = r.Messages
	out["max_tokens"] = r.MaxTokens

	if r.System != nil {
		out["system"] = r.System
	}
	if len(r.StopSequences) > 0 {
		out["stop_sequences"] = r.StopSequences
	}
	if r.Stream {
		out["stream"] = r.Stream
	}
	if r.Temperature != nil {
		out["temperature"] = *r.Temperature
	}
	if r.TopP != nil {
		out["top_p"] = *r.TopP
	}
	if r.TopK != nil {
		out["top_k"] = *r.TopK
	}
	if len(r.Tools) > 0 {
		out["tools"] = r.Tools
	}
	if r.ToolChoice != nil {
		out["tool_choice"] = r.ToolChoice
	}
	if r.Metadata != nil {
		out["metadata"] = r.Metadata
	}

	for k, v := range r.extras {
		var val any
		if err := json.Unmarshal(v, &val); err == nil {
			out[k] = val
		}
	}

	return json.Marshal(out)
}

func (r *AnthropicRequest) MergeFrom(raw []byte) error {
	var incoming map[string]json.RawMessage
	if err := json.Unmarshal(raw, &incoming); err != nil {
		return err
	}

	if r.extras == nil {
		r.extras = make(map[string]json.RawMessage)
	}
	for k, v := range incoming {
		if anthropicKnownFields[k] {
			continue // Skip known fields - they are managed separately
		}
		if _, exists := r.extras[k]; !exists {
			r.extras[k] = v
		}
	}
	return nil
}

// AnthropicResponse represents Anthropic messages API response format
type AnthropicResponse struct {
	ID           string `json:"id,omitempty"`
	Type         string `json:"type,omitempty"`
	Role         string `json:"role,omitempty"`
	Model        string `json:"model,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`

	Content []AnthropicContentBlock `json:"content,omitempty"`
	Usage   *AnthropicUsage         `json:"usage,omitempty"`

	extras map[string]json.RawMessage
}

// AnthropicContentBlock represents a content block in the response
type AnthropicContentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
}

// AnthropicUsage represents Anthropic usage information
type AnthropicUsage struct {
	InputTokens              int `json:"input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

func (r *AnthropicResponse) GetModel() string {
	return r.Model
}

func (r *AnthropicResponse) GetUsage() *Usage {
	if r.Usage == nil {
		return nil
	}
	return &Usage{
		PromptTokens:             r.Usage.InputTokens,
		CompletionTokens:         r.Usage.OutputTokens,
		TotalTokens:              r.Usage.InputTokens + r.Usage.OutputTokens,
		CacheReadInputTokens:     r.Usage.CacheReadInputTokens,
		CacheCreationInputTokens: r.Usage.CacheCreationInputTokens,
	}
}

func (r *AnthropicResponse) SetUsage(usage *Usage) {
	if usage == nil {
		r.Usage = nil
		return
	}
	r.Usage = &AnthropicUsage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
	}
}

func (r *AnthropicResponse) GetChoices() []Choice {
	// Convert Anthropic content blocks to OpenAI-style choices
	var textContent string
	var toolCalls []ToolCall

	for _, block := range r.Content {
		switch block.Type {
		case "text":
			textContent += block.Text
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: &struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				}{
					Name:      block.Name,
					Arguments: string(args),
				},
			})
		}
	}

	return []Choice{
		{
			Index: 0,
			Message: &Message{
				Role:      "assistant",
				Content:   textContent,
				ToolCalls: toolCalls,
			},
			FinishReason: r.StopReason,
		},
	}
}

func (r *AnthropicResponse) IsChunk() bool {
	return r.Type == "content_block_delta" || r.Type == "content_block_start" || r.Type == "message_delta"
}

func (r *AnthropicResponse) GetRawExtras() map[string]json.RawMessage { return r.extras }

func (r *AnthropicResponse) FromJSON(data []byte) error {
	if err := json.Unmarshal(data, r); err != nil {
		return err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	knownFields := map[string]bool{
		"id": true, "type": true, "role": true, "model": true,
		"stop_reason": true, "stop_sequence": true, "content": true, "usage": true,
	}

	r.extras = make(map[string]json.RawMessage)
	for k, v := range raw {
		if !knownFields[k] {
			r.extras[k] = v
		}
	}

	return nil
}

func (r *AnthropicResponse) ToJSON() ([]byte, error) {
	out := make(map[string]any)

	if r.ID != "" {
		out["id"] = r.ID
	}
	if r.Type != "" {
		out["type"] = r.Type
	}
	if r.Role != "" {
		out["role"] = r.Role
	}
	if r.Model != "" {
		out["model"] = r.Model
	}
	if r.StopReason != "" {
		out["stop_reason"] = r.StopReason
	}
	if r.StopSequence != "" {
		out["stop_sequence"] = r.StopSequence
	}
	if len(r.Content) > 0 {
		out["content"] = r.Content
	}
	if r.Usage != nil {
		out["usage"] = r.Usage
	}

	for k, v := range r.extras {
		var val any
		if err := json.Unmarshal(v, &val); err == nil {
			out[k] = val
		}
	}

	return json.Marshal(out)
}

// ToOpenAIChat converts Anthropic response to OpenAI chat response format
func (r *AnthropicResponse) ToOpenAIChat() *OpenAIChatResponse {
	response := &OpenAIChatResponse{
		ID:     r.ID,
		Object: "chat.completion",
		Model:  r.Model,
	}

	// Convert content blocks to message
	var textContent string
	var toolCalls []ToolCall

	for _, block := range r.Content {
		switch block.Type {
		case "text":
			textContent += block.Text
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: &struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				}{
					Name:      block.Name,
					Arguments: string(args),
				},
			})
		}
	}

	msg := &Message{
		Role:      "assistant",
		Content:   textContent,
		ToolCalls: toolCalls,
	}

	// Map stop reason
	finishReason := ""
	switch r.StopReason {
	case "end_turn":
		finishReason = "stop"
	case "max_tokens":
		finishReason = "length"
	case "stop_sequence":
		finishReason = "stop"
	case "tool_use":
		finishReason = "tool_calls"
	}

	response.Choices = []Choice{{
		Index:        0,
		Message:      msg,
		FinishReason: finishReason,
	}}

	if r.Usage != nil {
		response.Usage = &Usage{
			PromptTokens:     r.Usage.InputTokens,
			CompletionTokens: r.Usage.OutputTokens,
			TotalTokens:      r.Usage.InputTokens + r.Usage.OutputTokens,
		}
	}

	return response
}

// FromOpenAIChat converts OpenAI chat request to Anthropic request format
func (r *AnthropicRequest) FromOpenAIChat(req *OpenAIChatRequest) {
	r.Model = req.Model
	r.Stream = req.Stream

	// Handle messages - extract system message if present
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			r.System = msg.Content
		} else {
			r.Messages = append(r.Messages, msg)
		}
	}

	// Map max_tokens
	if req.MaxTokens > 0 {
		r.MaxTokens = req.MaxTokens
	} else if req.MaxCompletionTokens > 0 {
		r.MaxTokens = req.MaxCompletionTokens
	} else {
		r.MaxTokens = 4096 // Default for Anthropic
	}

	r.Temperature = req.Temperature
	r.TopP = req.TopP

	// Convert tools
	for _, tool := range req.Tools {
		if tool.Function != nil {
			r.Tools = append(r.Tools, AnthropicTool{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				InputSchema: tool.Function.Parameters,
			})
		}
	}

	// Map stop sequences
	if req.Stop != nil {
		switch v := req.Stop.(type) {
		case string:
			r.StopSequences = []string{v}
		case []interface{}:
			for _, s := range v {
				if str, ok := s.(string); ok {
					r.StopSequences = append(r.StopSequences, str)
				}
			}
		}
	}

	// Map user
	if req.User != "" {
		r.Metadata = &struct {
			UserID string `json:"user_id,omitempty"`
		}{UserID: req.User}
	}
}
