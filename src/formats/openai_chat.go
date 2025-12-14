package formats

import (
	"encoding/json"
)

// OpenAIChatRequest represents OpenAI chat completions request format
type OpenAIChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`

	// Streaming
	Stream        bool           `json:"stream,omitempty"`
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`

	// Generation controls
	MaxTokens           int                `json:"max_tokens,omitempty"`
	MaxCompletionTokens int                `json:"max_completion_tokens,omitempty"`
	Temperature         *float64           `json:"temperature,omitempty"`
	TopP                *float64           `json:"top_p,omitempty"`
	N                   int                `json:"n,omitempty"`
	Stop                any                `json:"stop,omitempty"`
	PresencePenalty     *float64           `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64           `json:"frequency_penalty,omitempty"`
	LogitBias           map[string]float64 `json:"logit_bias,omitempty"`
	Logprobs            bool               `json:"logprobs,omitempty"`
	TopLogprobs         int                `json:"top_logprobs,omitempty"`

	// Tools and function calling
	Tools             []Tool `json:"tools,omitempty"`
	ToolChoice        any    `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool  `json:"parallel_tool_calls,omitempty"`

	// Output and formatting
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`

	// Misc
	User            string `json:"user,omitempty"`
	Seed            *int64 `json:"seed,omitempty"`
	ServiceTier     string `json:"service_tier,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`

	// Extra fields for passthrough (provider-specific)
	extras map[string]json.RawMessage
}

func (r *OpenAIChatRequest) GetModel() string           { return r.Model }
func (r *OpenAIChatRequest) SetModel(model string)      { r.Model = model }
func (r *OpenAIChatRequest) GetMessages() []Message     { return r.Messages }
func (r *OpenAIChatRequest) SetMessages(msgs []Message) { r.Messages = msgs }
func (r *OpenAIChatRequest) IsStreaming() bool          { return r.Stream }

func (r *OpenAIChatRequest) GetRawExtras() map[string]json.RawMessage       { return r.extras }
func (r *OpenAIChatRequest) SetRawExtras(extras map[string]json.RawMessage) { r.extras = extras }

// Clone creates a deep copy of the request
func (r *OpenAIChatRequest) Clone() ManagedRequest {
	clone := &OpenAIChatRequest{
		Model:               r.Model,
		Stream:              r.Stream,
		MaxTokens:           r.MaxTokens,
		MaxCompletionTokens: r.MaxCompletionTokens,
		N:                   r.N,
		Stop:                r.Stop,
		Logprobs:            r.Logprobs,
		TopLogprobs:         r.TopLogprobs,
		Tools:               r.Tools,
		ToolChoice:          r.ToolChoice,
		ResponseFormat:      r.ResponseFormat,
		User:                r.User,
		ServiceTier:         r.ServiceTier,
		ReasoningEffort:     r.ReasoningEffort,
		LogitBias:           r.LogitBias,
	}

	// Deep copy messages
	if r.Messages != nil {
		clone.Messages = make([]Message, len(r.Messages))
		copy(clone.Messages, r.Messages)
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
	if r.PresencePenalty != nil {
		v := *r.PresencePenalty
		clone.PresencePenalty = &v
	}
	if r.FrequencyPenalty != nil {
		v := *r.FrequencyPenalty
		clone.FrequencyPenalty = &v
	}
	if r.ParallelToolCalls != nil {
		v := *r.ParallelToolCalls
		clone.ParallelToolCalls = &v
	}
	if r.Seed != nil {
		v := *r.Seed
		clone.Seed = &v
	}
	if r.StreamOptions != nil {
		clone.StreamOptions = &StreamOptions{IncludeUsage: r.StreamOptions.IncludeUsage}
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

func (r *OpenAIChatRequest) FromJSON(data []byte) error {
	// First unmarshal into struct
	if err := json.Unmarshal(data, r); err != nil {
		return err
	}

	// Then capture all raw fields for passthrough
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Store extras (fields not in our struct)
	r.extras = make(map[string]json.RawMessage)
	for k, v := range raw {
		if !openAIChatKnownFields[k] {
			r.extras[k] = v
		}
	}

	return nil
}

// openAIChatKnownFields lists fields handled by OpenAIChatRequest struct
var openAIChatKnownFields = map[string]bool{
	"model": true, "messages": true, "stream": true, "stream_options": true,
	"max_tokens": true, "max_completion_tokens": true, "temperature": true,
	"top_p": true, "n": true, "stop": true, "presence_penalty": true,
	"frequency_penalty": true, "logit_bias": true, "logprobs": true,
	"top_logprobs": true, "tools": true, "tool_choice": true,
	"parallel_tool_calls": true, "response_format": true, "user": true,
	"seed": true, "service_tier": true, "reasoning_effort": true,
}

func (r *OpenAIChatRequest) ToJSON() ([]byte, error) {
	// Create a map for output
	out := make(map[string]any)

	// Add known fields
	out["model"] = r.Model
	out["messages"] = r.Messages

	if r.Stream {
		out["stream"] = r.Stream
	}
	if r.StreamOptions != nil {
		out["stream_options"] = r.StreamOptions
	}
	if r.MaxTokens > 0 {
		out["max_tokens"] = r.MaxTokens
	}
	if r.MaxCompletionTokens > 0 {
		out["max_completion_tokens"] = r.MaxCompletionTokens
	}
	if r.Temperature != nil {
		out["temperature"] = *r.Temperature
	}
	if r.TopP != nil {
		out["top_p"] = *r.TopP
	}
	if r.N > 0 {
		out["n"] = r.N
	}
	if r.Stop != nil {
		out["stop"] = r.Stop
	}
	if r.PresencePenalty != nil {
		out["presence_penalty"] = *r.PresencePenalty
	}
	if r.FrequencyPenalty != nil {
		out["frequency_penalty"] = *r.FrequencyPenalty
	}
	if len(r.LogitBias) > 0 {
		out["logit_bias"] = r.LogitBias
	}
	if r.Logprobs {
		out["logprobs"] = r.Logprobs
	}
	if r.TopLogprobs > 0 {
		out["top_logprobs"] = r.TopLogprobs
	}
	if len(r.Tools) > 0 {
		out["tools"] = r.Tools
	}
	if r.ToolChoice != nil {
		out["tool_choice"] = r.ToolChoice
	}
	if r.ParallelToolCalls != nil {
		out["parallel_tool_calls"] = *r.ParallelToolCalls
	}
	if r.ResponseFormat != nil {
		out["response_format"] = r.ResponseFormat
	}
	if r.User != "" {
		out["user"] = r.User
	}
	if r.Seed != nil {
		out["seed"] = *r.Seed
	}
	if r.ServiceTier != "" {
		out["service_tier"] = r.ServiceTier
	}
	if r.ReasoningEffort != "" {
		out["reasoning_effort"] = r.ReasoningEffort
	}

	// Merge in extras for passthrough
	for k, v := range r.extras {
		var val any
		if err := json.Unmarshal(v, &val); err == nil {
			out[k] = val
		}
	}

	return json.Marshal(out)
}

func (r *OpenAIChatRequest) MergeFrom(raw []byte) error {
	var incoming map[string]json.RawMessage
	if err := json.Unmarshal(raw, &incoming); err != nil {
		return err
	}

	// Merge extras from original request, excluding known fields (which are managed separately)
	if r.extras == nil {
		r.extras = make(map[string]json.RawMessage)
	}
	for k, v := range incoming {
		if openAIChatKnownFields[k] {
			continue // Skip known fields - they are managed separately
		}
		if _, exists := r.extras[k]; !exists {
			r.extras[k] = v
		}
	}
	return nil
}

// OpenAIChatResponse represents OpenAI chat completions response format
type OpenAIChatResponse struct {
	ID                string   `json:"id,omitempty"`
	Object            string   `json:"object,omitempty"`
	Created           int64    `json:"created,omitempty"`
	Model             string   `json:"model,omitempty"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
	Choices           []Choice `json:"choices,omitempty"`
	Usage             *Usage   `json:"usage,omitempty"`
	ServiceTier       string   `json:"service_tier,omitempty"`

	extras map[string]json.RawMessage
}

func (r *OpenAIChatResponse) GetModel() string                         { return r.Model }
func (r *OpenAIChatResponse) GetUsage() *Usage                         { return r.Usage }
func (r *OpenAIChatResponse) SetUsage(usage *Usage)                    { r.Usage = usage }
func (r *OpenAIChatResponse) GetChoices() []Choice                     { return r.Choices }
func (r *OpenAIChatResponse) IsChunk() bool                            { return r.Object == "chat.completion.chunk" }
func (r *OpenAIChatResponse) GetRawExtras() map[string]json.RawMessage { return r.extras }

func (r *OpenAIChatResponse) FromJSON(data []byte) error {
	if err := json.Unmarshal(data, r); err != nil {
		return err
	}

	// Fix incorrect finish_reason when tool_calls are present
	// Some providers incorrectly return "stop" instead of "tool_calls"
	for i := range r.Choices {
		if r.Choices[i].Message != nil && len(r.Choices[i].Message.ToolCalls) > 0 {
			if r.Choices[i].FinishReason == "stop" || r.Choices[i].FinishReason == "" {
				r.Choices[i].FinishReason = "tool_calls"
			}
		}
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	knownFields := map[string]bool{
		"id": true, "object": true, "created": true, "model": true,
		"system_fingerprint": true, "choices": true, "usage": true,
		"service_tier": true,
	}

	r.extras = make(map[string]json.RawMessage)
	for k, v := range raw {
		if !knownFields[k] {
			r.extras[k] = v
		}
	}

	return nil
}

func (r *OpenAIChatResponse) ToJSON() ([]byte, error) {
	out := make(map[string]any)

	if r.ID != "" {
		out["id"] = r.ID
	}
	if r.Object != "" {
		out["object"] = r.Object
	}
	if r.Created != 0 {
		out["created"] = r.Created
	}
	if r.Model != "" {
		out["model"] = r.Model
	}
	if r.SystemFingerprint != "" {
		out["system_fingerprint"] = r.SystemFingerprint
	}
	if len(r.Choices) > 0 {
		out["choices"] = r.Choices
	}
	if r.Usage != nil {
		out["usage"] = r.Usage
	}
	if r.ServiceTier != "" {
		out["service_tier"] = r.ServiceTier
	}

	for k, v := range r.extras {
		var val any
		if err := json.Unmarshal(v, &val); err == nil {
			out[k] = val
		}
	}

	return json.Marshal(out)
}
