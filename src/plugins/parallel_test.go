package plugins

import (
	"testing"
)

func TestParseParallelModelList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single model",
			input:    "gpt-4",
			expected: []string{"gpt-4"},
		},
		{
			name:     "two models pipe-separated",
			input:    "gpt-4|claude-3",
			expected: []string{"gpt-4", "claude-3"},
		},
		{
			name:     "three models with spaces",
			input:    "gpt-4| claude-3 |gemini-pro",
			expected: []string{"gpt-4", "claude-3", "gemini-pro"},
		},
		{
			name:     "models with plugin suffix - suffix stripped",
			input:    "gpt-4|claude-3+zip",
			expected: []string{"gpt-4", "claude-3"},
		},
		{
			name:     "single model with plugin",
			input:    "gpt-4+zip:arg",
			expected: []string{"gpt-4+zip:arg"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{""},
		},
		{
			name:     "comma-separated should NOT split (parallel uses pipe)",
			input:    "gpt-4,claude-3",
			expected: []string{"gpt-4,claude-3"},
		},
		{
			name:     "mixed with plugin - suffix stripped from all",
			input:    "gpt-4|claude-3|gemini+fuzz",
			expected: []string{"gpt-4", "claude-3", "gemini"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseParallelModelList(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("ParseParallelModelList(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("ParseParallelModelList(%q)[%d] = %q, want %q", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}
