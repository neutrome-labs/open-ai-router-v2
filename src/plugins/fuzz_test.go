package plugins

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/neutrome-labs/open-ai-router/src/services"
)

func TestFuzz_Name(t *testing.T) {
	f := &Fuzz{}
	if f.Name() != "fuzz" {
		t.Errorf("Name() = %s, want fuzz", f.Name())
	}
}

func TestFuzz_Before_NoProvider(t *testing.T) {
	f := &Fuzz{}

	req := json.RawMessage(`{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`)

	httpReq := httptest.NewRequest("POST", "/v1/chat/completions", nil)

	// With nil provider, should pass through unchanged
	result, err := f.Before("", nil, httpReq, req)
	if err != nil {
		t.Fatalf("Before failed: %v", err)
	}

	var data struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(result, &data); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}
	if data.Model != "gpt-4" {
		t.Errorf("Model changed: %s", data.Model)
	}
}

func TestFuzz_Before_NoCommands(t *testing.T) {
	f := &Fuzz{}

	req := json.RawMessage(`{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`)

	httpReq := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	provider := &services.ProviderService{
		Name:     "test",
		Commands: nil, // No commands
	}

	result, err := f.Before("", provider, httpReq, req)
	if err != nil {
		t.Fatalf("Before failed: %v", err)
	}

	var data struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(result, &data); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}
	// Should pass through unchanged
	if data.Model != "gpt-4" {
		t.Errorf("Model changed: %s", data.Model)
	}
}

func TestFuzz_CacheKey(t *testing.T) {
	f := &Fuzz{}

	// Test cache stores resolved models
	// Simulate caching by directly accessing knownModelsCache
	cacheKey := "openai_gpt-4"
	f.knownModelsCache.Store(cacheKey, "gpt-4-0125-preview")

	// Check it's stored
	if cached, ok := f.knownModelsCache.Load(cacheKey); ok {
		if cached.(string) != "gpt-4-0125-preview" {
			t.Errorf("Cached value wrong: %s", cached.(string))
		}
	} else {
		t.Error("Cache should have value")
	}
}
