package formats

import (
	"encoding/json"
	"testing"
)

func TestOpenAIChatRequest_FromJSON(t *testing.T) {
	jsonInput := `{
		"model": "gpt-4",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello!"}
		],
		"max_tokens": 100,
		"temperature": 0.7,
		"stream": true,
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "Get weather info",
				"parameters": {"type": "object", "properties": {"location": {"type": "string"}}}
			}
		}],
		"custom_field": "custom_value",
		"provider_specific": 123
	}`

	req := &OpenAIChatRequest{}
	if err := req.FromJSON([]byte(jsonInput)); err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	// Verify known fields
	if req.Model != "gpt-4" {
		t.Errorf("Model wrong: %s", req.Model)
	}
	if len(req.Messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(req.Messages))
	}
	if req.MaxTokens != 100 {
		t.Errorf("MaxTokens wrong: %d", req.MaxTokens)
	}
	if req.Temperature == nil || *req.Temperature != 0.7 {
		t.Error("Temperature wrong")
	}
	if !req.Stream {
		t.Error("Stream should be true")
	}
	if len(req.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(req.Tools))
	}

	// Verify extras captured
	extras := req.GetRawExtras()
	if extras == nil {
		t.Fatal("Extras should not be nil")
	}
	if _, ok := extras["custom_field"]; !ok {
		t.Error("custom_field not in extras")
	}
	if _, ok := extras["provider_specific"]; !ok {
		t.Error("provider_specific not in extras")
	}
}

func TestOpenAIChatRequest_ToJSON_PreservesExtras(t *testing.T) {
	jsonInput := `{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": "Hi"}],
		"custom_field": "custom_value",
		"another_extra": 123
	}`

	req := &OpenAIChatRequest{}
	if err := req.FromJSON([]byte(jsonInput)); err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	output, err := req.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("Failed to parse output: %v", err)
	}

	// Verify extras preserved
	if result["custom_field"] != "custom_value" {
		t.Error("custom_field not preserved")
	}
	if result["another_extra"] != float64(123) {
		t.Error("another_extra not preserved")
	}
}

func TestOpenAIChatRequest_Clone(t *testing.T) {
	original := &OpenAIChatRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		MaxTokens: 100,
	}
	temp := 0.5
	original.Temperature = &temp

	cloned := original.Clone()
	clonedChat := cloned.(*OpenAIChatRequest)

	// Modify clone
	clonedChat.Model = "gpt-3.5"
	clonedChat.Messages[0].Content = "Modified"
	newTemp := 0.9
	clonedChat.Temperature = &newTemp

	// Original should be unchanged
	if original.Model != "gpt-4" {
		t.Error("Original model was modified")
	}
	if original.Messages[0].Content != "Hello" {
		t.Error("Original message was modified")
	}
	if *original.Temperature != 0.5 {
		t.Error("Original temperature was modified")
	}
}

func TestOpenAIChatRequest_MergeFrom(t *testing.T) {
	req := &OpenAIChatRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "user", Content: "Test"},
		},
	}

	// Merge in extra fields
	extraJSON := `{"provider_specific": true, "custom_param": "value"}`
	if err := req.MergeFrom([]byte(extraJSON)); err != nil {
		t.Fatalf("MergeFrom failed: %v", err)
	}

	output, _ := req.ToJSON()
	var result map[string]any
	json.Unmarshal(output, &result)

	if result["provider_specific"] != true {
		t.Error("provider_specific not merged")
	}
	if result["custom_param"] != "value" {
		t.Error("custom_param not merged")
	}
}

func TestOpenAIChatResponse_ToJSON(t *testing.T) {
	resp := &OpenAIChatResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4",
		Choices: []Choice{
			{
				Index:        0,
				Message:      &Message{Role: "assistant", Content: "Hello!"},
				FinishReason: "stop",
			},
		},
		Usage: &Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	data, err := resp.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if result["id"] != "chatcmpl-123" {
		t.Error("ID wrong")
	}
	if result["model"] != "gpt-4" {
		t.Error("Model wrong")
	}

	choices := result["choices"].([]any)
	if len(choices) != 1 {
		t.Error("Should have 1 choice")
	}
}
