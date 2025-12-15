package styles

import (
	"testing"

	"github.com/neutrome-labs/open-ai-router/src/formats"
)

// =============================================================================
// OpenAI Chat <-> Anthropic Conversion Tests
// =============================================================================

func TestConvert_OpenAIChat_To_Anthropic(t *testing.T) {
	temp := 0.7
	input := &formats.OpenAIChatRequest{
		Model: "gpt-4",
		Messages: []formats.Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello!"},
			{Role: "assistant", Content: "Hi there!"},
			{Role: "user", Content: "How are you?"},
		},
		MaxTokens:   100,
		Temperature: &temp,
		Tools: []formats.Tool{
			{
				Type: "function",
				Function: &formats.ToolFunction{
					Name:        "get_weather",
					Description: "Get weather info",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}

	converter := &DefaultConverter{}
	result, err := converter.ConvertRequest(input, StyleOpenAIChat, StyleAnthropic)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	anthropicReq, ok := result.(*formats.AnthropicRequest)
	if !ok {
		t.Fatalf("Expected AnthropicRequest, got %T", result)
	}

	// Model preserved
	if anthropicReq.Model != "gpt-4" {
		t.Errorf("Model not preserved: got %s", anthropicReq.Model)
	}

	// System message extracted
	if anthropicReq.System == nil {
		t.Error("System message not extracted")
	} else if anthropicReq.System != "You are a helpful assistant." {
		t.Errorf("System message wrong: %v", anthropicReq.System)
	}

	// Messages don't include system
	for _, msg := range anthropicReq.Messages {
		if msg.Role == "system" {
			t.Error("System message should not be in messages array")
		}
	}

	// 3 messages without system
	if len(anthropicReq.Messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(anthropicReq.Messages))
	}

	// Tools converted
	if len(anthropicReq.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(anthropicReq.Tools))
	} else if anthropicReq.Tools[0].Name != "get_weather" {
		t.Errorf("Tool name wrong: %s", anthropicReq.Tools[0].Name)
	}

	// Temperature preserved
	if anthropicReq.Temperature == nil || *anthropicReq.Temperature != 0.7 {
		t.Error("Temperature not preserved")
	}
}

func TestConvert_Anthropic_To_OpenAIChat(t *testing.T) {
	input := &formats.AnthropicRequest{
		Model:     "claude-3-sonnet",
		System:    "You are a code assistant.",
		MaxTokens: 500,
		Messages: []formats.Message{
			{Role: "user", Content: "Write a function"},
			{Role: "assistant", Content: "Here's a function..."},
			{Role: "user", Content: "Add error handling"},
		},
		Tools: []formats.AnthropicTool{
			{
				Name:        "run_code",
				Description: "Execute code",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"code": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	converter := &DefaultConverter{}
	result, err := converter.ConvertRequest(input, StyleAnthropic, StyleOpenAIChat)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	chatReq, ok := result.(*formats.OpenAIChatRequest)
	if !ok {
		t.Fatalf("Expected OpenAIChatRequest, got %T", result)
	}

	// System message added to beginning
	if len(chatReq.Messages) == 0 {
		t.Fatal("No messages")
	}
	if chatReq.Messages[0].Role != "system" {
		t.Error("First message should be system")
	}
	if chatReq.Messages[0].Content != "You are a code assistant." {
		t.Errorf("System content wrong: %v", chatReq.Messages[0].Content)
	}

	// Total: 1 system + 3 user/assistant
	if len(chatReq.Messages) != 4 {
		t.Errorf("Expected 4 messages, got %d", len(chatReq.Messages))
	}

	// Tools converted
	if len(chatReq.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(chatReq.Tools))
	} else {
		tool := chatReq.Tools[0]
		if tool.Type != "function" {
			t.Errorf("Tool type should be 'function', got %s", tool.Type)
		}
		if tool.Function.Name != "run_code" {
			t.Errorf("Tool function name wrong: %s", tool.Function.Name)
		}
	}
}

// =============================================================================
// OpenAI Chat <-> OpenAI Responses Conversion Tests
// =============================================================================

func TestConvert_OpenAIChat_To_OpenAIResponses(t *testing.T) {
	input := &formats.OpenAIChatRequest{
		Model: "gpt-4o",
		Messages: []formats.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
		MaxTokens: 200,
		Stream:    true,
		Tools: []formats.Tool{
			{
				Type: "function",
				Function: &formats.ToolFunction{
					Name:        "search",
					Description: "Search the web",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}

	converter := &DefaultConverter{}
	result, err := converter.ConvertRequest(input, StyleOpenAIChat, StyleOpenAIResponses)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	respReq, ok := result.(*formats.OpenAIResponsesRequest)
	if !ok {
		t.Fatalf("Expected OpenAIResponsesRequest, got %T", result)
	}

	if respReq.Model != "gpt-4o" {
		t.Errorf("Model wrong: %s", respReq.Model)
	}
	if !respReq.Stream {
		t.Error("Stream flag not preserved")
	}
	if respReq.Input == nil {
		t.Error("Input should not be nil")
	}

	// Tools converted to Responses format (flat structure)
	if len(respReq.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(respReq.Tools))
	} else {
		tool := respReq.Tools[0]
		if tool.Name != "search" {
			t.Errorf("Tool name wrong: %s", tool.Name)
		}
		if tool.Description != "Search the web" {
			t.Errorf("Tool description wrong: %s", tool.Description)
		}
	}
}

func TestConvert_OpenAIResponses_To_OpenAIChat(t *testing.T) {
	input := &formats.OpenAIResponsesRequest{
		Model:           "gpt-4o",
		Input:           "What is the capital of France?",
		Instructions:    "Answer briefly.",
		MaxOutputTokens: 100,
	}

	converter := &DefaultConverter{}
	result, err := converter.ConvertRequest(input, StyleOpenAIResponses, StyleOpenAIChat)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	chatReq, ok := result.(*formats.OpenAIChatRequest)
	if !ok {
		t.Fatalf("Expected OpenAIChatRequest, got %T", result)
	}

	// Instructions become system message
	if len(chatReq.Messages) < 2 {
		t.Fatalf("Expected at least 2 messages, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Role != "system" {
		t.Error("First message should be system")
	}
	if chatReq.Messages[0].Content != "Answer briefly." {
		t.Errorf("System content wrong: %v", chatReq.Messages[0].Content)
	}

	// String input becomes user message
	if chatReq.Messages[1].Role != "user" {
		t.Error("Second message should be user")
	}
	if chatReq.Messages[1].Content != "What is the capital of France?" {
		t.Errorf("User content wrong: %v", chatReq.Messages[1].Content)
	}
}

func TestConvert_OpenAIResponses_To_OpenAIChat_WithMessages(t *testing.T) {
	input := &formats.OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: []formats.Message{
			{Role: "user", Content: "Hi"},
			{Role: "assistant", Content: "Hello!"},
			{Role: "user", Content: "How are you?"},
		},
		MaxOutputTokens: 100,
	}

	converter := &DefaultConverter{}
	result, err := converter.ConvertRequest(input, StyleOpenAIResponses, StyleOpenAIChat)
	if err != nil {
		t.Fatalf("Conversion failed: %v", err)
	}

	chatReq := result.(*formats.OpenAIChatRequest)
	if len(chatReq.Messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(chatReq.Messages))
	}
}

// =============================================================================
// Passthrough Tests
// =============================================================================

func TestConvert_Passthrough_SameStyle(t *testing.T) {
	input := &formats.OpenAIChatRequest{
		Model: "gpt-4",
		Messages: []formats.Message{
			{Role: "user", Content: "Test"},
		},
	}

	converter := &DefaultConverter{}
	result, err := converter.ConvertRequest(input, StyleOpenAIChat, StyleOpenAIChat)
	if err != nil {
		t.Fatalf("Passthrough failed: %v", err)
	}

	// Same style should return same object
	if result != input {
		t.Error("Passthrough should return same object reference")
	}
}

// =============================================================================
// Response Conversion Tests
// =============================================================================

func TestConvertResponse_Anthropic_To_OpenAIChat(t *testing.T) {
	anthropicResp := &formats.AnthropicResponse{
		ID:         "msg_123",
		Type:       "message",
		Role:       "assistant",
		Model:      "claude-3-sonnet",
		StopReason: "end_turn",
		Content: []formats.AnthropicContentBlock{
			{Type: "text", Text: "Hello!"},
		},
		Usage: &formats.AnthropicUsage{
			InputTokens:  10,
			OutputTokens: 8,
		},
	}

	converter := &DefaultConverter{}
	result, err := converter.ConvertResponse(anthropicResp, StyleAnthropic, StyleOpenAIChat)
	if err != nil {
		t.Fatalf("Response conversion failed: %v", err)
	}

	chatResp, ok := result.(*formats.OpenAIChatResponse)
	if !ok {
		t.Fatalf("Expected OpenAIChatResponse, got %T", result)
	}

	if len(chatResp.Choices) == 0 {
		t.Fatal("Should have at least one choice")
	}

	choice := chatResp.Choices[0]
	if choice.Message.Role != "assistant" {
		t.Errorf("Role wrong: %s", choice.Message.Role)
	}
	if choice.Message.Content != "Hello!" {
		t.Errorf("Content wrong: %v", choice.Message.Content)
	}

	if chatResp.Usage == nil {
		t.Error("Usage should not be nil")
	} else {
		if chatResp.Usage.PromptTokens != 10 {
			t.Errorf("Prompt tokens wrong: %d", chatResp.Usage.PromptTokens)
		}
		if chatResp.Usage.CompletionTokens != 8 {
			t.Errorf("Completion tokens wrong: %d", chatResp.Usage.CompletionTokens)
		}
	}
}

func TestConvertResponse_Passthrough_SameStyle(t *testing.T) {
	resp := &formats.OpenAIChatResponse{
		ID:    "chat-123",
		Model: "gpt-4",
	}

	converter := &DefaultConverter{}
	result, err := converter.ConvertResponse(resp, StyleOpenAIChat, StyleOpenAIChat)
	if err != nil {
		t.Fatalf("Passthrough failed: %v", err)
	}

	if result != resp {
		t.Error("Passthrough should return same object")
	}
}
