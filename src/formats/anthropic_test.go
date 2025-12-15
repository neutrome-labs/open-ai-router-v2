package formats

import (
	"encoding/json"
	"testing"
)

func TestAnthropicRequest_FromJSON(t *testing.T) {
	jsonInput := `{
		"model": "claude-3-sonnet",
		"messages": [
			{"role": "user", "content": "Hello!"},
			{"role": "assistant", "content": "Hi!"},
			{"role": "user", "content": "How are you?"}
		],
		"system": "You are a helpful assistant.",
		"max_tokens": 500,
		"temperature": 0.8,
		"tools": [{
			"name": "get_weather",
			"description": "Get weather",
			"input_schema": {"type": "object", "properties": {"location": {"type": "string"}}}
		}],
		"custom_anthropic_field": "value"
	}`

	req := &AnthropicRequest{}
	if err := req.FromJSON([]byte(jsonInput)); err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	if req.Model != "claude-3-sonnet" {
		t.Errorf("Model wrong: %s", req.Model)
	}
	if len(req.Messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(req.Messages))
	}
	if req.System != "You are a helpful assistant." {
		t.Errorf("System wrong: %v", req.System)
	}
	if req.MaxTokens != 500 {
		t.Errorf("MaxTokens wrong: %d", req.MaxTokens)
	}
	if len(req.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(req.Tools))
	}

	// Extras
	extras := req.GetRawExtras()
	if _, ok := extras["custom_anthropic_field"]; !ok {
		t.Error("custom_anthropic_field not in extras")
	}
}

func TestAnthropicRequest_Clone(t *testing.T) {
	original := &AnthropicRequest{
		Model:     "claude-3",
		System:    "Be helpful",
		MaxTokens: 200,
		Messages: []Message{
			{Role: "user", Content: "Test"},
		},
	}

	cloned := original.Clone()
	clonedAnth := cloned.(*AnthropicRequest)

	clonedAnth.Model = "claude-2"
	clonedAnth.System = "Be brief"

	if original.Model != "claude-3" {
		t.Error("Original model was modified")
	}
	if original.System != "Be helpful" {
		t.Error("Original system was modified")
	}
}

func TestAnthropicRequest_ToJSON(t *testing.T) {
	temp := 0.7
	req := &AnthropicRequest{
		Model:       "claude-3-sonnet",
		MaxTokens:   100,
		System:      "Be concise",
		Temperature: &temp,
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Tools: []AnthropicTool{
			{Name: "search", Description: "Search web"},
		},
	}

	data, err := req.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if result["model"] != "claude-3-sonnet" {
		t.Error("Model wrong")
	}
	if result["max_tokens"] != float64(100) {
		t.Error("max_tokens wrong")
	}
	if result["system"] != "Be concise" {
		t.Error("system wrong")
	}
}

func TestAnthropicResponse_ToOpenAIChat(t *testing.T) {
	resp := &AnthropicResponse{
		ID:         "msg_123",
		Type:       "message",
		Role:       "assistant",
		Model:      "claude-3-sonnet",
		StopReason: "end_turn",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "Hello! How can I help you?"},
		},
		Usage: &AnthropicUsage{
			InputTokens:  10,
			OutputTokens: 8,
		},
	}

	chatResp := resp.ToOpenAIChat()

	if chatResp.ID == "" {
		t.Error("ID should not be empty")
	}
	if chatResp.Model != "claude-3-sonnet" {
		t.Errorf("Model wrong: %s", chatResp.Model)
	}
	if len(chatResp.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(chatResp.Choices))
	}

	choice := chatResp.Choices[0]
	if choice.Message.Role != "assistant" {
		t.Error("Role should be assistant")
	}
	if choice.Message.Content != "Hello! How can I help you?" {
		t.Errorf("Content wrong: %v", choice.Message.Content)
	}
	if choice.FinishReason != "stop" {
		t.Errorf("FinishReason wrong: %s", choice.FinishReason)
	}

	if chatResp.Usage == nil {
		t.Fatal("Usage should not be nil")
	}
	if chatResp.Usage.PromptTokens != 10 {
		t.Errorf("PromptTokens wrong: %d", chatResp.Usage.PromptTokens)
	}
	if chatResp.Usage.CompletionTokens != 8 {
		t.Errorf("CompletionTokens wrong: %d", chatResp.Usage.CompletionTokens)
	}
}

func TestAnthropicResponse_ToolUse_ToOpenAIChat(t *testing.T) {
	resp := &AnthropicResponse{
		ID:         "msg_456",
		Role:       "assistant",
		Model:      "claude-3",
		StopReason: "tool_use",
		Content: []AnthropicContentBlock{
			{
				Type: "tool_use",
				ID:   "tool_call_1",
				Name: "get_weather",
				Input: map[string]any{
					"location": "Tokyo",
				},
			},
		},
	}

	chatResp := resp.ToOpenAIChat()

	if len(chatResp.Choices) == 0 {
		t.Fatal("No choices")
	}

	message := chatResp.Choices[0].Message
	if len(message.ToolCalls) == 0 {
		t.Fatal("No tool calls")
	}

	tc := message.ToolCalls[0]
	if tc.ID != "tool_call_1" {
		t.Errorf("Tool call ID wrong: %s", tc.ID)
	}
	if tc.Type != "function" {
		t.Errorf("Tool call type wrong: %s", tc.Type)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("Function name wrong: %s", tc.Function.Name)
	}

	// Arguments should be JSON string
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Errorf("Failed to parse arguments: %v", err)
	}
	if args["location"] != "Tokyo" {
		t.Errorf("Arguments wrong: %v", args)
	}
}

func TestAnthropicRequest_FromOpenAIChat(t *testing.T) {
	temp := 0.7
	openaiReq := &OpenAIChatRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hi"},
			{Role: "assistant", Content: "Hello!"},
			{Role: "user", Content: "How are you?"},
		},
		MaxTokens:   100,
		Temperature: &temp,
		Tools: []Tool{
			{
				Type: "function",
				Function: &ToolFunction{
					Name:        "get_weather",
					Description: "Get weather",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
	}

	anthropicReq := &AnthropicRequest{}
	anthropicReq.FromOpenAIChat(openaiReq)

	if anthropicReq.Model != "gpt-4" {
		t.Errorf("Model wrong: %s", anthropicReq.Model)
	}

	// System should be extracted
	if anthropicReq.System != "You are helpful." {
		t.Errorf("System wrong: %v", anthropicReq.System)
	}

	// Messages should not contain system
	for _, msg := range anthropicReq.Messages {
		if msg.Role == "system" {
			t.Error("System should not be in messages")
		}
	}
	if len(anthropicReq.Messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(anthropicReq.Messages))
	}

	// Tools converted
	if len(anthropicReq.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(anthropicReq.Tools))
	}
	if anthropicReq.Tools[0].Name != "get_weather" {
		t.Error("Tool name wrong")
	}
}
