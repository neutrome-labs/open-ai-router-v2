package plugins

import (
	"testing"

	"github.com/neutrome-labs/open-ai-router/src/formats"
)

func TestIsToolMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      formats.Message
		expected bool
	}{
		{
			name:     "regular user message",
			msg:      formats.Message{Role: "user", Content: "Hello"},
			expected: false,
		},
		{
			name:     "regular assistant message",
			msg:      formats.Message{Role: "assistant", Content: "Hi there"},
			expected: false,
		},
		{
			name:     "system message",
			msg:      formats.Message{Role: "system", Content: "You are helpful"},
			expected: false,
		},
		{
			name: "assistant with tool calls",
			msg: formats.Message{
				Role: "assistant",
				ToolCalls: []formats.ToolCall{
					{ID: "call_1", Type: "function", Function: &struct {
						Name      string `json:"name,omitempty"`
						Arguments string `json:"arguments,omitempty"`
					}{Name: "get_weather", Arguments: `{"city":"NYC"}`}},
				},
			},
			expected: true,
		},
		{
			name:     "tool response message",
			msg:      formats.Message{Role: "tool", ToolCallID: "call_1", Content: "Sunny, 72F"},
			expected: true,
		},
		{
			name:     "message with tool_call_id",
			msg:      formats.Message{Role: "assistant", ToolCallID: "call_1", Content: "Result"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isToolMessage(tt.msg)
			if result != tt.expected {
				t.Errorf("isToolMessage() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestTruncateContent(t *testing.T) {
	tests := []struct {
		name     string
		content  any
		maxLen   int
		expected any
	}{
		{
			name:     "short string unchanged",
			content:  "Hello",
			maxLen:   100,
			expected: "Hello",
		},
		{
			name:     "long string truncated",
			content:  "This is a very long string that should be truncated to a shorter length with ellipsis at the end.",
			maxLen:   50,
			expected: "This is a very long string that should be trunc...",
		},
		{
			name:     "exact length unchanged",
			content:  "12345",
			maxLen:   5,
			expected: "12345",
		},
		{
			name:     "very short maxLen",
			content:  "Hello",
			maxLen:   2,
			expected: "He",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateContent(tt.content, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateContent() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestTruncateToolCalls(t *testing.T) {
	toolCalls := []formats.ToolCall{
		{
			ID:    "call_1",
			Type:  "function",
			Index: 0,
			Function: &struct {
				Name      string `json:"name,omitempty"`
				Arguments string `json:"arguments,omitempty"`
			}{
				Name:      "get_weather",
				Arguments: `{"city": "New York City", "units": "fahrenheit", "detailed": true, "forecast_days": 7}`,
			},
		},
	}

	result := truncateToolCalls(toolCalls, 50)

	if len(result) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result))
	}
	if result[0].Function.Name != "get_weather" {
		t.Errorf("name should be preserved, got %s", result[0].Function.Name)
	}
	if len(result[0].Function.Arguments) > 50 {
		t.Errorf("arguments should be truncated to 50 chars, got %d", len(result[0].Function.Arguments))
	}
	if result[0].Function.Arguments[len(result[0].Function.Arguments)-3:] != "..." {
		t.Errorf("truncated arguments should end with ..., got %s", result[0].Function.Arguments)
	}
}

func TestFindLastToolInteractionBoundary(t *testing.T) {
	tests := []struct {
		name     string
		messages []formats.Message
		expected int
	}{
		{
			name: "no tool messages",
			messages: []formats.Message{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi"},
			},
			expected: -1,
		},
		{
			name: "single tool interaction at end - active",
			messages: []formats.Message{
				{Role: "user", Content: "What's the weather?"},
				{Role: "assistant", ToolCalls: []formats.ToolCall{{ID: "call_1"}}},
				{Role: "tool", ToolCallID: "call_1", Content: "Sunny"},
			},
			expected: 1, // preserve from index 1 (assistant with tool calls)
		},
		{
			name: "tool interaction followed by text - all completed",
			messages: []formats.Message{
				{Role: "user", Content: "What's the weather?"},
				{Role: "assistant", ToolCalls: []formats.ToolCall{{ID: "call_1"}}},
				{Role: "tool", ToolCallID: "call_1", Content: "Sunny"},
				{Role: "assistant", Content: "The weather is sunny."},
				{Role: "user", Content: "Thanks!"},
			},
			expected: -1, // all completed, truncate everything
		},
		{
			name: "multiple tool interactions - last one active",
			messages: []formats.Message{
				{Role: "user", Content: "Get weather"},
				{Role: "assistant", ToolCalls: []formats.ToolCall{{ID: "call_1"}}},
				{Role: "tool", ToolCallID: "call_1", Content: "Sunny"},
				{Role: "assistant", Content: "It's sunny."},
				{Role: "user", Content: "Now get stock price"},
				{Role: "assistant", ToolCalls: []formats.ToolCall{{ID: "call_2"}}},
				{Role: "tool", ToolCallID: "call_2", Content: "150.00"},
			},
			expected: 5, // preserve from index 5 (second tool call)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findLastToolInteractionBoundary(tt.messages)
			if result != tt.expected {
				t.Errorf("findLastToolInteractionBoundary() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestStoolsBefore(t *testing.T) {
	stools := &Stools{}

	t.Run("truncates earlier tool interactions", func(t *testing.T) {
		longContent := "This is a very long tool response that contains lots of detailed information about the weather including temperature, humidity, wind speed, precipitation chances, UV index, and more detailed forecast data."

		req := &formats.OpenAIChatRequest{
			Model: "gpt-4",
			Messages: []formats.Message{
				{Role: "user", Content: "Get weather"},
				{Role: "assistant", ToolCalls: []formats.ToolCall{{ID: "call_1", Function: &struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				}{Name: "get_weather", Arguments: longContent}}}},
				{Role: "tool", ToolCallID: "call_1", Content: longContent},
				{Role: "assistant", Content: "It's sunny."},
				{Role: "user", Content: "Now get stock"},
				{Role: "assistant", ToolCalls: []formats.ToolCall{{ID: "call_2", Function: &struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				}{Name: "get_stock", Arguments: `{"symbol":"AAPL"}`}}}},
				{Role: "tool", ToolCallID: "call_2", Content: "Stock price is $150.00"},
			},
		}

		result, err := stools.Before("", nil, nil, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		messages := result.GetMessages()

		// First tool response (index 2) should be truncated
		firstToolContent, ok := messages[2].Content.(string)
		if !ok {
			t.Fatal("expected string content")
		}
		if len(firstToolContent) > 100 {
			t.Errorf("first tool content should be truncated to 100 chars, got %d", len(firstToolContent))
		}

		// First tool call arguments should be truncated
		if len(messages[1].ToolCalls[0].Function.Arguments) > 100 {
			t.Errorf("first tool call args should be truncated, got %d chars", len(messages[1].ToolCalls[0].Function.Arguments))
		}

		// Last tool response (index 6) should NOT be truncated
		lastToolContent, ok := messages[6].Content.(string)
		if !ok {
			t.Fatal("expected string content for last tool")
		}
		if lastToolContent != "Stock price is $150.00" {
			t.Errorf("last tool content should be preserved, got %s", lastToolContent)
		}

		// Last tool call arguments should NOT be truncated
		if messages[5].ToolCalls[0].Function.Arguments != `{"symbol":"AAPL"}` {
			t.Errorf("last tool call args should be preserved, got %s", messages[5].ToolCalls[0].Function.Arguments)
		}
	})

	t.Run("no tool messages - unchanged", func(t *testing.T) {
		req := &formats.OpenAIChatRequest{
			Model: "gpt-4",
			Messages: []formats.Message{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
			},
		}

		result, err := stools.Before("", nil, nil, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		messages := result.GetMessages()
		if len(messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(messages))
		}
		if messages[0].Content != "Hello" {
			t.Errorf("content should be unchanged")
		}
	})

	t.Run("all tools executed with follow-up - all truncated", func(t *testing.T) {
		longContent := "Very long tool response with lots of data that should definitely be truncated to save context space"

		req := &formats.OpenAIChatRequest{
			Model: "gpt-4",
			Messages: []formats.Message{
				{Role: "user", Content: "Get weather"},
				{Role: "assistant", ToolCalls: []formats.ToolCall{{ID: "call_1", Function: &struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				}{Name: "get_weather", Arguments: longContent + longContent}}}},
				{Role: "tool", ToolCallID: "call_1", Content: longContent + longContent},
				{Role: "assistant", Content: "The weather is sunny today."},
				{Role: "user", Content: "Thanks for letting me know!"},
			},
		}

		result, err := stools.Before("", nil, nil, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		messages := result.GetMessages()

		// Tool response should be truncated (tool interaction is not the last one)
		toolContent, ok := messages[2].Content.(string)
		if !ok {
			t.Fatal("expected string content")
		}
		if len(toolContent) > 100 {
			t.Errorf("tool content should be truncated to 100 chars, got %d", len(toolContent))
		}

		// Tool call arguments should be truncated
		if len(messages[1].ToolCalls[0].Function.Arguments) > 100 {
			t.Errorf("tool call args should be truncated, got %d chars", len(messages[1].ToolCalls[0].Function.Arguments))
		}
	})
}
