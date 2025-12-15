package plugins

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neutrome-labs/open-ai-router/src/formats"
	"github.com/neutrome-labs/open-ai-router/src/services"
)

func TestPluginRegistry(t *testing.T) {
	// Test all expected plugins are registered
	expectedPlugins := []string{"posthog", "models", "fuzz", "zip", "zipc", "zips", "zipsc"}

	for _, name := range expectedPlugins {
		p, ok := GetPlugin(name)
		if !ok {
			t.Errorf("Plugin %q not found in registry", name)
			continue
		}
		if p.Name() != name {
			t.Errorf("Plugin name mismatch: got %q, want %q", p.Name(), name)
		}
	}
}

func TestPluginChain_Add(t *testing.T) {
	chain := NewPluginChain()

	p, _ := GetPlugin("models")
	chain.Add(p, "test-param")

	if len(chain.plugins) != 1 {
		t.Errorf("Expected 1 plugin, got %d", len(chain.plugins))
	}

	if chain.plugins[0].Params != "test-param" {
		t.Errorf("Params wrong: %s", chain.plugins[0].Params)
	}
}

func TestPluginChain_RunBefore(t *testing.T) {
	chain := NewPluginChain()

	p, _ := GetPlugin("models")
	chain.Add(p, "")

	req := &formats.OpenAIChatRequest{
		Model: "gpt-4",
		Messages: []formats.Message{
			{Role: "user", Content: "Hello"},
		},
	}

	httpReq := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	provider := &services.ProviderImpl{Name: "test"}

	result, err := chain.RunBefore(provider, httpReq, req)
	if err != nil {
		t.Fatalf("RunBefore failed: %v", err)
	}

	if result.GetModel() != "gpt-4" {
		t.Errorf("Model changed unexpectedly: %s", result.GetModel())
	}
}

func TestPluginChain_RunAfter(t *testing.T) {
	chain := NewPluginChain()

	p, _ := GetPlugin("models")
	chain.Add(p, "")

	req := &formats.OpenAIChatRequest{
		Model: "gpt-4",
	}
	resp := &formats.OpenAIChatResponse{
		Model: "gpt-4",
		Choices: []formats.Choice{
			{Message: &formats.Message{Role: "assistant", Content: "Hi"}},
		},
	}

	httpReq := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	httpResp := &http.Response{StatusCode: 200}
	provider := &services.ProviderImpl{Name: "test"}

	result, err := chain.RunAfter(provider, httpReq, req, httpResp, resp)
	if err != nil {
		t.Fatalf("RunAfter failed: %v", err)
	}

	if result.GetModel() != "gpt-4" {
		t.Errorf("Model wrong: %s", result.GetModel())
	}
}

func TestMandatoryPlugins(t *testing.T) {
	// Verify mandatory plugins exist and are valid
	for _, mp := range HeadPlugins {
		name := mp[0]
		p, ok := GetPlugin(name)
		if !ok {
			t.Errorf("Mandatory plugin %q not found", name)
			continue
		}
		if p == nil {
			t.Errorf("Mandatory plugin %q is nil", name)
		}
	}

	for _, mp := range TailPlugins {
		name := mp[0]
		p, ok := GetPlugin(name)
		if !ok {
			t.Errorf("Mandatory plugin %q not found", name)
			continue
		}
		if p == nil {
			t.Errorf("Mandatory plugin %q is nil", name)
		}
	}
}
