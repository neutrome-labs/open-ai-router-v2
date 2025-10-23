package formats

import (
	"encoding/json"
)

type ChatCompletionsRequest struct {
	Model    string `json:"model,omitempty"`
	Stream   bool   `json:"stream,omitempty"`
	Messages []any  `json:"messages,omitempty"`
}

type SimpleChatMessage struct {
	Role       string     `json:"role,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Content    string     `json:"content,omitempty"`
	Refusal    string     `json:"refusal,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type MultimodalChatMessage struct {
	Role    string `json:"role,omitempty"`
	Content []struct {
		Type       string `json:"type,omitempty"`
		Text       string `json:"text,omitempty"`
		ImageURL   string `json:"image_url,omitempty"`
		InputAudio string `json:"input_audio,omitempty"`
		File       string `json:"file,omitempty"`
		Refusal    string `json:"refusal,omitempty"`
	} `json:"content,omitempty"`
	Refusal   string     `json:"refusal,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Custom   string `json:"custom,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments []any  `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

func (r *ChatCompletionsRequest) FromJson(data []byte) error {
	err := json.Unmarshal(data, r)
	if err != nil {
		return err
	}
	return nil
}

func (r *ChatCompletionsRequest) ToJson() ([]byte, error) {
	return json.Marshal(r)
}

type ChatCompletionsResponse struct {
	ID      string       `json:"id,omitempty"`
	Object  string       `json:"object,omitempty"`
	Created int64        `json:"created,omitempty"`
	Choices []ChatChoice `json:"choices,omitempty"`
	Usage   ChatUsage    `json:"usage,omitempty"`
}

type ChatChoice struct {
	Error        *ChatChoiceError `json:"error,omitempty"`
	Index        int              `json:"index,omitempty"`
	Message      *any             `json:"message,omitempty"`
	Delta        *any             `json:"delta,omitempty"`
	FinishReason string           `json:"finish_reason,omitempty"`
}

type ChatChoiceError struct {
	Code    int    `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

func (r *ChatCompletionsResponse) FromJson(data []byte) error {
	err := json.Unmarshal(data, r)
	if err != nil {
		return err
	}
	return nil
}

func (r *ChatCompletionsResponse) ToJson() ([]byte, error) {
	return json.Marshal(r)
}

type ChatCompletionsStreamResponseChunk struct {
	RuntimeError error        `json:"-"`
	ID           string       `json:"id,omitempty"`
	Object       string       `json:"object,omitempty"`
	Created      int64        `json:"created,omitempty"`
	Choices      []ChatChoice `json:"choices,omitempty"`
	Usage        ChatUsage    `json:"usage,omitempty"`
}

func (r *ChatCompletionsStreamResponseChunk) FromJson(data []byte) error {
	err := json.Unmarshal(data, r)
	if err != nil {
		return err
	}
	return nil
}

func (r *ChatCompletionsStreamResponseChunk) ToJson() ([]byte, error) {
	return json.Marshal(r)
}
