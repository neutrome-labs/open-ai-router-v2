package plugin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/neutrome-labs/open-ai-router/src/modules"
	"github.com/neutrome-labs/open-ai-router/src/plugin"
	"github.com/neutrome-labs/open-ai-router/src/services"
)

func TestPluginRegistry(t *testing.T) {
	// Test all expected plugins are registered
	expectedPlugins := []string{"posthog", "models", "fuzz"}

	for _, name := range expectedPlugins {
		p, ok := plugin.GetPlugin(name)
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
	chain := plugin.NewPluginChain()

	p, _ := plugin.GetPlugin("models")
	chain.Add(p, "test-param")

	if len(chain.GetPlugins()) != 1 {
		t.Errorf("Expected 1 plugin, got %d", len(chain.GetPlugins()))
	}

	if chain.GetPlugins()[0].Params != "test-param" {
		t.Errorf("Params wrong: %s", chain.GetPlugins()[0].Params)
	}
}

func TestPluginChain_RunBefore(t *testing.T) {
	chain := plugin.NewPluginChain()

	p, _ := plugin.GetPlugin("models")
	chain.Add(p, "")

	req := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`)

	httpReq := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	provider := &services.ProviderService{Name: "test"}

	result, err := chain.RunBefore(provider, httpReq, req)
	if err != nil {
		t.Fatalf("RunBefore failed: %v", err)
	}

	var data struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(result, &data); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if data.Model != "gpt-4" {
		t.Errorf("Model changed unexpectedly: %s", data.Model)
	}
}

func TestPluginChain_RunAfter(t *testing.T) {
	chain := plugin.NewPluginChain()

	p, _ := plugin.GetPlugin("models")
	chain.Add(p, "")

	req := []byte(`{"model":"gpt-4"}`)
	resp := []byte(`{"model":"gpt-4","choices":[{"message":{"role":"assistant","content":"Hi"}}]}`)

	httpReq := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	httpResp := &http.Response{StatusCode: 200}
	provider := &services.ProviderService{Name: "test"}

	result, err := chain.RunAfter(provider, httpReq, req, httpResp, resp)
	if err != nil {
		t.Fatalf("RunAfter failed: %v", err)
	}

	var data struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(result, &data); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if data.Model != "gpt-4" {
		t.Errorf("Model wrong: %s", data.Model)
	}
}

func TestMandatoryPlugins(t *testing.T) {
	// Verify mandatory plugins exist and are valid
	for _, mp := range plugin.HeadPlugins {
		name := mp[0]
		p, ok := plugin.GetPlugin(name)
		if !ok {
			t.Errorf("Mandatory plugin %q not found", name)
			continue
		}
		if p == nil {
			t.Errorf("Mandatory plugin %q is nil", name)
		}
	}

	for _, mp := range plugin.TailPlugins {
		name := mp[0]
		p, ok := plugin.GetPlugin(name)
		if !ok {
			t.Errorf("Mandatory plugin %q not found", name)
			continue
		}
		if p == nil {
			t.Errorf("Mandatory plugin %q is nil", name)
		}
	}
}
