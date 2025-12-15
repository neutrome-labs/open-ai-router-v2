package plugins

import (
	"testing"

	"github.com/neutrome-labs/open-ai-router/src/formats"
)

func TestZip_Name(t *testing.T) {
	tests := []struct {
		plugin   Zip
		expected string
	}{
		{Zip{PreserveFirst: false, DisableCache: false}, "zip"},
		{Zip{PreserveFirst: true, DisableCache: false}, "zipc"},
		{Zip{PreserveFirst: false, DisableCache: true}, "zips"},
		{Zip{PreserveFirst: true, DisableCache: true}, "zipsc"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.plugin.Name() != tt.expected {
				t.Errorf("Name() = %s, want %s", tt.plugin.Name(), tt.expected)
			}
		})
	}
}

func TestZip_ExtractMessages(t *testing.T) {
	z := &Zip{PreserveFirst: false}

	messages := []formats.Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "First question"},
		{Role: "assistant", Content: "First answer"},
		{Role: "user", Content: "Second question"},
		{Role: "assistant", Content: "Second answer"},
		{Role: "user", Content: "Last question"},
	}

	system, compactable, lastInput, firstUser := z.extractMessages(messages)

	// System should be extracted
	if len(system) != 1 {
		t.Errorf("Expected 1 system message, got %d", len(system))
	}
	if system[0].Content != "You are helpful" {
		t.Error("System content wrong")
	}

	// Last user message preserved
	if len(lastInput) != 1 {
		t.Errorf("Expected 1 last input, got %d", len(lastInput))
	}
	if lastInput[0].Content != "Last question" {
		t.Error("Last input wrong")
	}

	// First user not preserved when PreserveFirst=false
	if len(firstUser) != 0 {
		t.Errorf("Expected 0 first user, got %d", len(firstUser))
	}

	// Compactable should be middle messages
	if len(compactable) != 4 {
		t.Errorf("Expected 4 compactable messages, got %d", len(compactable))
	}
}

func TestZip_ExtractMessages_PreserveFirst(t *testing.T) {
	z := &Zip{PreserveFirst: true}

	messages := []formats.Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "First question"},
		{Role: "assistant", Content: "First answer"},
		{Role: "user", Content: "Second question"},
		{Role: "assistant", Content: "Second answer"},
		{Role: "user", Content: "Last question"},
	}

	system, compactable, lastInput, firstUser := z.extractMessages(messages)

	// System extracted
	if len(system) != 1 {
		t.Errorf("Expected 1 system, got %d", len(system))
	}

	// First user preserved
	if len(firstUser) != 1 {
		t.Errorf("Expected 1 first user, got %d", len(firstUser))
	}
	if firstUser[0].Content != "First question" {
		t.Error("First user content wrong")
	}

	// Last preserved
	if len(lastInput) != 1 {
		t.Errorf("Expected 1 last input, got %d", len(lastInput))
	}

	// Compactable is middle (without first user)
	if len(compactable) != 3 {
		t.Errorf("Expected 3 compactable, got %d", len(compactable))
	}
}

func TestZip_ExtractMessages_EmptyInput(t *testing.T) {
	z := &Zip{}

	system, compactable, lastInput, firstUser := z.extractMessages(nil)

	if system != nil || compactable != nil || lastInput != nil || firstUser != nil {
		t.Error("Empty input should return all nils")
	}
}

func TestZip_ExtractMessages_OnlySystem(t *testing.T) {
	z := &Zip{}

	messages := []formats.Message{
		{Role: "system", Content: "You are helpful"},
	}

	system, compactable, lastInput, firstUser := z.extractMessages(messages)

	if len(system) != 1 {
		t.Errorf("Expected 1 system, got %d", len(system))
	}
	if compactable != nil || lastInput != nil || firstUser != nil {
		t.Error("Should only have system")
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text     string
		expected int
	}{
		{"", 0},
		{"hi", 1},          // 2 chars -> ~0.5 tokens, rounds to 1
		{"hello", 2},       // 5 chars -> ~1.25 tokens, rounds to 2
		{"hello world", 3}, // 11 chars -> ~2.75 tokens, rounds to 3
	}

	for _, tt := range tests {
		result := estimateTokens(tt.text)
		if result != tt.expected {
			t.Errorf("estimateTokens(%q) = %d, want %d", tt.text, result, tt.expected)
		}
	}
}

func TestEstimateMessagesTokens(t *testing.T) {
	messages := []formats.Message{
		{Role: "user", Content: "Hello"},       // ~2 + 4 overhead = 6
		{Role: "assistant", Content: "Hi"},     // ~1 + 4 overhead = 5
		{Role: "user", Content: "How are you"}, // ~3 + 4 overhead = 7
	}

	tokens := estimateMessagesTokens(messages)

	// Should be reasonable estimate
	if tokens < 10 || tokens > 30 {
		t.Errorf("Token estimate seems off: %d", tokens)
	}
}

func TestHashMessages(t *testing.T) {
	messages1 := []formats.Message{
		{Role: "user", Content: "Hello"},
	}
	messages2 := []formats.Message{
		{Role: "user", Content: "Hello"},
	}
	messages3 := []formats.Message{
		{Role: "user", Content: "Hi"},
	}

	hash1 := hashMessages(messages1)
	hash2 := hashMessages(messages2)
	hash3 := hashMessages(messages3)

	// Same content should produce same hash
	if hash1 != hash2 {
		t.Error("Same messages should produce same hash")
	}

	// Different content should produce different hash
	if hash1 == hash3 {
		t.Error("Different messages should produce different hash")
	}

	// Hash should be reasonable length (32 hex chars for 16 bytes)
	if len(hash1) != 32 {
		t.Errorf("Hash length wrong: %d", len(hash1))
	}
}
