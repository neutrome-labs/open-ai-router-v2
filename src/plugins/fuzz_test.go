package plugins

import (
	"net/http/httptest"
	"testing"

	"github.com/neutrome-labs/open-ai-router/src/formats"
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

	req := &formats.OpenAIChatRequest{
		Model: "gpt-4",
	}

	httpReq := httptest.NewRequest("POST", "/v1/chat/completions", nil)

	// With nil provider, should pass through unchanged
	result, err := f.Before("", nil, httpReq, req)
	if err != nil {
		t.Fatalf("Before failed: %v", err)
	}

	if result.GetModel() != "gpt-4" {
		t.Errorf("Model changed: %s", result.GetModel())
	}
}

func TestFuzz_Before_NoCommands(t *testing.T) {
	f := &Fuzz{}

	req := &formats.OpenAIChatRequest{
		Model: "gpt-4",
	}

	httpReq := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	provider := &services.ProviderImpl{
		Name:     "test",
		Commands: nil, // No commands
	}

	result, err := f.Before("", provider, httpReq, req)
	if err != nil {
		t.Fatalf("Before failed: %v", err)
	}

	// Should pass through unchanged
	if result.GetModel() != "gpt-4" {
		t.Errorf("Model changed: %s", result.GetModel())
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
