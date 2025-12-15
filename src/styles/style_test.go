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
		{"openai-responses", StyleOpenAIResponses},
		{"responses", StyleOpenAIResponses},
		{"anthropic-messages", StyleAnthropic},
		{"anthropic", StyleAnthropic},
		{"google-genai", StyleGoogleGenAI},
		{"google", StyleGoogleGenAI},
		{"cloudflare-workers-ai", StyleCfWorkersAi},
		{"cloudflare", StyleCfWorkersAi},
		{"cf", StyleCfWorkersAi},
		{"unknown", StyleOpenAIChat}, // defaults to OpenAI
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseStyle(tt.input)
			if result != tt.expected {
				t.Errorf("ParseStyle(%q) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStyleAuthHeader(t *testing.T) {
	tests := []struct {
		style    Style
		expected string
	}{
		{StyleOpenAIChat, "Authorization"},
		{StyleOpenAIResponses, "Authorization"},
		{StyleAnthropic, "x-api-key"},
		{StyleGoogleGenAI, "Authorization"},
	}

	for _, tt := range tests {
		t.Run(string(tt.style), func(t *testing.T) {
			result := StyleAuthHeader(tt.style)
			if result != tt.expected {
				t.Errorf("StyleAuthHeader(%s) = %s, want %s", tt.style, result, tt.expected)
			}
		})
	}
}

func TestStyleAuthFormat(t *testing.T) {
	key := "sk-test123"

	// OpenAI style adds Bearer prefix
	result := StyleAuthFormat(StyleOpenAIChat, key)
	if result != "Bearer sk-test123" {
		t.Errorf("OpenAI auth format wrong: %s", result)
	}

	// Anthropic uses raw key
	result = StyleAuthFormat(StyleAnthropic, key)
	if result != "sk-test123" {
		t.Errorf("Anthropic auth format wrong: %s", result)
	}
}

func TestStyleRequiresVersion(t *testing.T) {
	// Anthropic requires version header
	header, value, required := StyleRequiresVersion(StyleAnthropic)
	if !required {
		t.Error("Anthropic should require version")
	}
	if header != "anthropic-version" {
		t.Errorf("Header wrong: %s", header)
	}
	if value != "2023-06-01" {
		t.Errorf("Version wrong: %s", value)
	}

	// OpenAI doesn't require version
	_, _, required = StyleRequiresVersion(StyleOpenAIChat)
	if required {
		t.Error("OpenAI should not require version")
	}
}
