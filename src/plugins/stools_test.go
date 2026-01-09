package plugins

import (
	"encoding/json"
	"testing"

	"github.com/neutrome-labs/open-ai-router/src/styles"
)

func TestStripTools(t *testing.T) {
	tests := []struct {
		name          string
		messages      []styles.ChatCompletionsMessage
		expectedCount int
	}{
		{
			name:          "no messages",
			messages:      []styles.ChatCompletionsMessage{},
			expectedCount: 0,
		},
		{
			name: "no tool calls - unchanged",
			messages: []styles.ChatCompletionsMessage{
				{Role: "system", Content: "You are helpful"},
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
			},
			expectedCount: 3,
		},
		{
			name: "single tool interaction - unchanged",
			messages: []styles.ChatCompletionsMessage{
				{Role: "user", Content: "What's the weather?"},
				{Role: "assistant", ToolCalls: []styles.ChatCompletionsToolCall{{ID: "call_1"}}},
				{Role: "tool", ToolCallID: "call_1", Content: "Sunny, 72F"},
				{Role: "assistant", Content: "The weather is sunny!"},
			},
			expectedCount: 4,
		},
		{
			name: "two tool interactions - first stripped",
			messages: []styles.ChatCompletionsMessage{
				{Role: "user", Content: "What's the weather?"},
				{Role: "assistant", ToolCalls: []styles.ChatCompletionsToolCall{{ID: "call_1"}}},
				{Role: "tool", ToolCallID: "call_1", Content: "Sunny, 72F"},
				{Role: "assistant", Content: "The weather is sunny. Need anything else?"},
				{Role: "user", Content: "What about LA?"},
				{Role: "assistant", ToolCalls: []styles.ChatCompletionsToolCall{{ID: "call_2"}}},
				{Role: "tool", ToolCallID: "call_2", Content: "Cloudy, 65F"},
			},
			expectedCount: 5, // user, assistant(text only), user, assistant+toolcall, tool
		},
		{
			name: "three tool interactions - first two stripped",
			messages: []styles.ChatCompletionsMessage{
				{Role: "user", Content: "Start"},
				{Role: "assistant", ToolCalls: []styles.ChatCompletionsToolCall{{ID: "c1"}}},
				{Role: "tool", ToolCallID: "c1", Content: "Result 1"},
				{Role: "assistant", ToolCalls: []styles.ChatCompletionsToolCall{{ID: "c2"}}},
				{Role: "tool", ToolCallID: "c2", Content: "Result 2"},
				{Role: "assistant", ToolCalls: []styles.ChatCompletionsToolCall{{ID: "c3"}}},
				{Role: "tool", ToolCallID: "c3", Content: "Result 3"},
			},
			expectedCount: 3, // user, assistant+toolcall, tool
		},
		{
			name: "tool interaction with content preserved",
			messages: []styles.ChatCompletionsMessage{
				{Role: "user", Content: "Question"},
				{Role: "assistant", Content: "Let me check...", ToolCalls: []styles.ChatCompletionsToolCall{{ID: "c1"}}},
				{Role: "tool", ToolCallID: "c1", Content: "Data"},
				{Role: "assistant", ToolCalls: []styles.ChatCompletionsToolCall{{ID: "c2"}}},
				{Role: "tool", ToolCallID: "c2", Content: "More data"},
			},
			expectedCount: 4, // user, assistant(content only), assistant+toolcall, tool
		},
		{
			name: "multiple tool responses in one interaction",
			messages: []styles.ChatCompletionsMessage{
				{Role: "user", Content: "Do multiple things"},
				{Role: "assistant", ToolCalls: []styles.ChatCompletionsToolCall{{ID: "c1"}, {ID: "c2"}}},
				{Role: "tool", ToolCallID: "c1", Content: "Result 1"},
				{Role: "tool", ToolCallID: "c2", Content: "Result 2"},
				{Role: "assistant", Content: "Done with first batch"},
				{Role: "assistant", ToolCalls: []styles.ChatCompletionsToolCall{{ID: "c3"}}},
				{Role: "tool", ToolCallID: "c3", Content: "Result 3"},
			},
			expectedCount: 4, // user, assistant(text), assistant+toolcall, tool
		},
	}

	plugin := &StripTools{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqData := map[string]any{
				"model":    "gpt-4",
				"messages": tt.messages,
			}
			reqBytes, err := json.Marshal(reqData)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			reqJson, err := styles.ParsePartialJSON(reqBytes)
			if err != nil {
				t.Fatalf("Failed to parse partial JSON: %v", err)
			}

			result, err := plugin.Before("", nil, nil, reqJson)
			if err != nil {
				t.Fatalf("Plugin returned error: %v", err)
			}

			resultMessages, err := styles.GetFromPartialJSON[[]styles.ChatCompletionsMessage](result, "messages")
			if err != nil {
				t.Fatalf("Failed to get result messages: %v", err)
			}

			if len(resultMessages) != tt.expectedCount {
				t.Errorf("Expected %d messages, got %d", tt.expectedCount, len(resultMessages))
				for i, m := range resultMessages {
					t.Logf("  [%d] role=%s, content=%v, toolCalls=%d", i, m.Role, m.Content, len(m.ToolCalls))
				}
			}
		})
	}
}
