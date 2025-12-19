package plugins

import (
	"encoding/json"
	"testing"

	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/styles"
)

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

		result, err := stools.Before("", &services.ProviderService{
			Style: styles.StyleOpenAIChat,
		}, nil, req)

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

		result, err := stools.Before("", &services.ProviderService{
			Style: styles.StyleOpenAIChat,
		}, nil, req)

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

		result, err := stools.Before("", &services.ProviderService{
			Style: styles.StyleOpenAIChat,
		}, nil, req)

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
