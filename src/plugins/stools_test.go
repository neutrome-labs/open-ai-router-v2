package plugins

import (
	"encoding/json"
	"testing"
)

func TestIsToolMessageRaw(t *testing.T) {
	tests := []struct {
		name     string
		msg      message
		expected bool
	}{
		{
			name:     "regular user message",
			msg:      message{Role: "user", Content: json.RawMessage(`"Hello"`)},
			expected: false,
		},
		{
			name:     "regular assistant message",
			msg:      message{Role: "assistant", Content: json.RawMessage(`"Hi there"`)},
			expected: false,
		},
		{
			name:     "system message",
			msg:      message{Role: "system", Content: json.RawMessage(`"You are helpful"`)},
			expected: false,
		},
		{
			name: "assistant with tool calls",
			msg: message{
				Role:      "assistant",
				ToolCalls: json.RawMessage(`[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"NYC\"}"}}]`),
			},
			expected: true,
		},
		{
			name:     "tool response message",
			msg:      message{Role: "tool", ToolCallID: "call_1", Content: json.RawMessage(`"Sunny, 72F"`)},
			expected: true,
		},
		{
			name:     "message with tool_call_id",
			msg:      message{Role: "assistant", ToolCallID: "call_1", Content: json.RawMessage(`"Result"`)},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isToolMessageRaw(tt.msg)
			if result != tt.expected {
				t.Errorf("isToolMessageRaw() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestTruncateContentRaw(t *testing.T) {
	tests := []struct {
		name     string
		content  json.RawMessage
		maxLen   int
		expected string // expected string value after unmarshal
	}{
		{
			name:     "short string unchanged",
			content:  json.RawMessage(`"Hello"`),
			maxLen:   100,
			expected: "Hello",
		},
		{
			name:     "long string truncated",
			content:  json.RawMessage(`"This is a very long string that should be truncated to a shorter length with ellipsis at the end."`),
			maxLen:   50,
			expected: "This is a very long string that should be trunc...",
		},
		{
			name:     "exact length unchanged",
			content:  json.RawMessage(`"12345"`),
			maxLen:   5,
			expected: "12345",
		},
		{
			name:     "very short maxLen",
			content:  json.RawMessage(`"Hello"`),
			maxLen:   2,
			expected: "He",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateContentRaw(tt.content, tt.maxLen)
			var strResult string
			if err := json.Unmarshal(result, &strResult); err != nil {
				t.Fatalf("failed to unmarshal result: %v", err)
			}
			if strResult != tt.expected {
				t.Errorf("truncateContentRaw() = %v, expected %v", strResult, tt.expected)
			}
		})
	}
}

func TestTruncateToolCallsRaw(t *testing.T) {
	toolCalls := json.RawMessage(`[{"id":"call_1","type":"function","index":0,"function":{"name":"get_weather","arguments":"{\"city\": \"New York City\", \"units\": \"fahrenheit\", \"detailed\": true, \"forecast_days\": 7}"}}]`)

	result := truncateToolCallsRaw(toolCalls, 50)

	var resultData []map[string]any
	if err := json.Unmarshal(result, &resultData); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if len(resultData) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resultData))
	}
	fn := resultData[0]["function"].(map[string]any)
	if fn["name"] != "get_weather" {
		t.Errorf("name should be preserved, got %s", fn["name"])
	}
	args := fn["arguments"].(string)
	if len(args) > 50 {
		t.Errorf("arguments should be truncated to 50 chars, got %d", len(args))
	}
	if args[len(args)-3:] != "..." {
		t.Errorf("truncated arguments should end with ..., got %s", args)
	}
}

func TestFindLastToolInteractionBoundaryRaw(t *testing.T) {
	tests := []struct {
		name     string
		messages []message
		expected int
	}{
		{
			name: "no tool messages",
			messages: []message{
				{Role: "user", Content: json.RawMessage(`"Hello"`)},
				{Role: "assistant", Content: json.RawMessage(`"Hi"`)},
			},
			expected: -1,
		},
		{
			name: "single tool interaction at end - active",
			messages: []message{
				{Role: "user", Content: json.RawMessage(`"What's the weather?"`)},
				{Role: "assistant", ToolCalls: json.RawMessage(`[{"id":"call_1"}]`)},
				{Role: "tool", ToolCallID: "call_1", Content: json.RawMessage(`"Sunny"`)},
			},
			expected: 1, // preserve from index 1 (assistant with tool calls)
		},
		{
			name: "tool interaction followed by text - all completed",
			messages: []message{
				{Role: "user", Content: json.RawMessage(`"What's the weather?"`)},
				{Role: "assistant", ToolCalls: json.RawMessage(`[{"id":"call_1"}]`)},
				{Role: "tool", ToolCallID: "call_1", Content: json.RawMessage(`"Sunny"`)},
				{Role: "assistant", Content: json.RawMessage(`"The weather is sunny."`)},
				{Role: "user", Content: json.RawMessage(`"Thanks!"`)},
			},
			expected: -1, // all completed, truncate everything
		},
		{
			name: "multiple tool interactions - last one active",
			messages: []message{
				{Role: "user", Content: json.RawMessage(`"Get weather"`)},
				{Role: "assistant", ToolCalls: json.RawMessage(`[{"id":"call_1"}]`)},
				{Role: "tool", ToolCallID: "call_1", Content: json.RawMessage(`"Sunny"`)},
				{Role: "assistant", Content: json.RawMessage(`"It's sunny."`)},
				{Role: "user", Content: json.RawMessage(`"Now get stock price"`)},
				{Role: "assistant", ToolCalls: json.RawMessage(`[{"id":"call_2"}]`)},
				{Role: "tool", ToolCallID: "call_2", Content: json.RawMessage(`"150.00"`)},
			},
			expected: 5, // preserve from index 5 (second tool call)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findLastToolInteractionBoundaryRaw(tt.messages)
			if result != tt.expected {
				t.Errorf("findLastToolInteractionBoundaryRaw() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestStoolsBefore(t *testing.T) {
	stools := &Stools{}

	t.Run("truncates earlier tool interactions", func(t *testing.T) {
		longContent := "This is a very long tool response that contains lots of detailed information about the weather including temperature, humidity, wind speed, precipitation chances, UV index, and more detailed forecast data."
		longContentJSON, _ := json.Marshal(longContent)
		longArgsJSON := longContent // as raw string for arguments

		req := json.RawMessage(`{
			"model": "gpt-4",
			"messages": [
				{"role": "user", "content": "Get weather"},
				{"role": "assistant", "tool_calls": [{"id": "call_1", "function": {"name": "get_weather", "arguments": "` + longArgsJSON + `"}}]},
				{"role": "tool", "tool_call_id": "call_1", "content": ` + string(longContentJSON) + `},
				{"role": "assistant", "content": "It's sunny."},
				{"role": "user", "content": "Now get stock"},
				{"role": "assistant", "tool_calls": [{"id": "call_2", "function": {"name": "get_stock", "arguments": "{\"symbol\":\"AAPL\"}"}}]},
				{"role": "tool", "tool_call_id": "call_2", "content": "Stock price is $150.00"}
			]
		}`)

		result, err := stools.Before("", nil, nil, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var data struct {
			Messages []struct {
				Role       string          `json:"role"`
				Content    json.RawMessage `json:"content"`
				ToolCallID string          `json:"tool_call_id"`
				ToolCalls  []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(result, &data); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}

		// First tool response (index 2) should be truncated
		var firstToolContent string
		if err := json.Unmarshal(data.Messages[2].Content, &firstToolContent); err != nil {
			t.Fatal("expected string content")
		}
		if len(firstToolContent) > 100 {
			t.Errorf("first tool content should be truncated to 100 chars, got %d", len(firstToolContent))
		}

		// First tool call arguments should be truncated
		if len(data.Messages[1].ToolCalls[0].Function.Arguments) > 100 {
			t.Errorf("first tool call args should be truncated, got %d chars", len(data.Messages[1].ToolCalls[0].Function.Arguments))
		}

		// Last tool response (index 6) should NOT be truncated
		var lastToolContent string
		if err := json.Unmarshal(data.Messages[6].Content, &lastToolContent); err != nil {
			t.Fatal("expected string content for last tool")
		}
		if lastToolContent != "Stock price is $150.00" {
			t.Errorf("last tool content should be preserved, got %s", lastToolContent)
		}

		// Last tool call arguments should NOT be truncated
		if data.Messages[5].ToolCalls[0].Function.Arguments != `{"symbol":"AAPL"}` {
			t.Errorf("last tool call args should be preserved, got %s", data.Messages[5].ToolCalls[0].Function.Arguments)
		}
	})

	t.Run("no tool messages - unchanged", func(t *testing.T) {
		req := json.RawMessage(`{
			"model": "gpt-4",
			"messages": [
				{"role": "user", "content": "Hello"},
				{"role": "assistant", "content": "Hi there!"}
			]
		}`)

		result, err := stools.Before("", nil, nil, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var data struct {
			Messages []struct {
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(result, &data); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if len(data.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(data.Messages))
		}
		var content string
		if err := json.Unmarshal(data.Messages[0].Content, &content); err != nil {
			t.Fatal("expected string content")
		}
		if content != "Hello" {
			t.Errorf("content should be unchanged")
		}
	})

	t.Run("all tools executed with follow-up - all truncated", func(t *testing.T) {
		longContent := "Very long tool response with lots of data that should definitely be truncated to save context space"
		doubleContent := longContent + longContent
		doubleContentJSON, _ := json.Marshal(doubleContent)

		req := json.RawMessage(`{
			"model": "gpt-4",
			"messages": [
				{"role": "user", "content": "Get weather"},
				{"role": "assistant", "tool_calls": [{"id": "call_1", "function": {"name": "get_weather", "arguments": "` + doubleContent + `"}}]},
				{"role": "tool", "tool_call_id": "call_1", "content": ` + string(doubleContentJSON) + `},
				{"role": "assistant", "content": "The weather is sunny today."},
				{"role": "user", "content": "Thanks for letting me know!"}
			]
		}`)

		result, err := stools.Before("", nil, nil, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var data struct {
			Messages []struct {
				Role       string          `json:"role"`
				Content    json.RawMessage `json:"content"`
				ToolCallID string          `json:"tool_call_id"`
				ToolCalls  []struct {
					Function struct {
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(result, &data); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		// Tool response should be truncated (tool interaction is not the last one)
		var toolContent string
		if err := json.Unmarshal(data.Messages[2].Content, &toolContent); err != nil {
			t.Fatal("expected string content")
		}
		if len(toolContent) > 100 {
			t.Errorf("tool content should be truncated to 100 chars, got %d", len(toolContent))
		}

		// Tool call arguments should be truncated
		if len(data.Messages[1].ToolCalls[0].Function.Arguments) > 100 {
			t.Errorf("tool call args should be truncated, got %d chars", len(data.Messages[1].ToolCalls[0].Function.Arguments))
		}
	})
}
