package styles

import (
	"testing"
)

func TestParseStyle(t *testing.T) {
	tests := []struct {
		input    string
		expected Style
	}{
		{"openai-chat-completions", StyleOpenAIChat},
		{"openai", StyleOpenAIChat},
		{"", StyleOpenAIChat},
		{"openai-responses", StyleUnknown},
		{"responses", StyleUnknown},
		{"anthropic-messages", StyleUnknown},
		{"anthropic", StyleUnknown},
		{"google-genai", StyleUnknown},
		{"google", StyleUnknown},
		{"cloudflare-workers-ai", StyleUnknown},
		{"cloudflare", StyleUnknown},
		{"cf", StyleUnknown},
		{"unknown", StyleUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, _ := ParseStyle(tt.input)
			if result != tt.expected {
				t.Errorf("ParseStyle(%q) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}
