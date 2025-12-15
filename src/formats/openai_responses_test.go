package formats

import (
	"encoding/json"
	"testing"
)

func TestOpenAIResponsesRequest_FromJSON(t *testing.T) {
	jsonInput := `{
		"model": "gpt-4o",
		"input": "What is the capital of France?",
		"instructions": "Answer briefly.",
		"max_output_tokens": 100,
		"temperature": 0.5,
		"stream": true,
		"tools": [{
			"type": "function",
			"name": "search",
			"description": "Search the web",
			"parameters": {"type": "object"}
		}],
		"custom_field": "value"
	}`

	req := &OpenAIResponsesRequest{}
	if err := req.FromJSON([]byte(jsonInput)); err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	if req.Model != "gpt-4o" {
		t.Errorf("Model wrong: %s", req.Model)
	}
	if req.Input != "What is the capital of France?" {
		t.Errorf("Input wrong: %v", req.Input)
	}
	if req.Instructions != "Answer briefly." {
		t.Errorf("Instructions wrong: %s", req.Instructions)
	}
	if req.MaxOutputTokens != 100 {
		t.Errorf("MaxOutputTokens wrong: %d", req.MaxOutputTokens)
	}
	if !req.Stream {
		t.Error("Stream should be true")
	}
	if len(req.Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(req.Tools))
	}

	// Check tool structure (flat format for Responses API)
	tool := req.Tools[0]
	if tool.Name != "search" {
		t.Errorf("Tool name wrong: %s", tool.Name)
	}
	if tool.Description != "Search the web" {
		t.Errorf("Tool description wrong: %s", tool.Description)
	}

	// Extras
	extras := req.GetRawExtras()
	if _, ok := extras["custom_field"]; !ok {
		t.Error("custom_field not in extras")
	}
}

func TestOpenAIResponsesRequest_GetMessages_StringInput(t *testing.T) {
	req := &OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: "Hello",
	}

	msgs := req.GetMessages()
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Error("Role should be user")
	}
	if msgs[0].Content != "Hello" {
		t.Error("Content wrong")
	}
}

func TestOpenAIResponsesRequest_GetMessages_ArrayInput(t *testing.T) {
	req := &OpenAIResponsesRequest{
		Model: "gpt-4o",
		Input: []Message{
			{Role: "user", Content: "Hi"},
			{Role: "assistant", Content: "Hello!"},
		},
	}

	msgs := req.GetMessages()
	if len(msgs) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(msgs))
	}
}

func TestOpenAIResponsesRequest_Clone(t *testing.T) {
	original := &OpenAIResponsesRequest{
		Model:           "gpt-4o",
		Input:           "Test",
		Instructions:    "Be brief",
		MaxOutputTokens: 50,
	}

	cloned := original.Clone()
	clonedResp := cloned.(*OpenAIResponsesRequest)

	clonedResp.Model = "gpt-3.5"
	clonedResp.Input = "Modified"

	if original.Model != "gpt-4o" {
		t.Error("Original model was modified")
	}
	if original.Input != "Test" {
		t.Error("Original input was modified")
	}
}

func TestOpenAIResponsesRequest_ToJSON(t *testing.T) {
	temp := 0.7
	req := &OpenAIResponsesRequest{
		Model:           "gpt-4o",
		Input:           "Hello",
		Instructions:    "Be helpful",
		MaxOutputTokens: 100,
		Temperature:     &temp,
		Stream:          true,
		Tools: []ResponsesTool{
			{Type: "function", Name: "search", Description: "Search"},
		},
	}

	data, err := req.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if result["model"] != "gpt-4o" {
		t.Error("Model wrong")
	}
	if result["input"] != "Hello" {
		t.Error("Input wrong")
	}
	if result["instructions"] != "Be helpful" {
		t.Error("Instructions wrong")
	}
	if result["stream"] != true {
		t.Error("Stream wrong")
	}
}

func TestOpenAIResponsesResponse_ToJSON(t *testing.T) {
	resp := &OpenAIResponsesResponse{
		ID:     "resp_123",
		Object: "response",
		Model:  "gpt-4o",
		Output: []ResponsesOutputItem{
			{
				Type:    "message",
				ID:      "msg_1",
				Status:  "completed",
				Role:    "assistant",
				Content: []ResponsesContentPart{{Type: "output_text", Text: "Hello!"}},
			},
		},
		Usage: &ResponsesUsage{
			InputTokens:  10,
			OutputTokens: 5,
			TotalTokens:  15,
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

	if result["id"] != "resp_123" {
		t.Error("ID wrong")
	}
	if result["model"] != "gpt-4o" {
		t.Error("Model wrong")
	}
}
