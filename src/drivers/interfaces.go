// Package drivers provides command interfaces for provider interactions.
package drivers

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/styles"
)

// ListModelsModel represents a model from a provider
type ListModelsModel struct {
	Object  string `json:"object,omitempty"`
	ID      string `json:"id,omitempty"`
	Name    string `json:"name,omitempty"`
	Created int64  `json:"created,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}

// ListModelsCommand lists available models from a provider
type ListModelsCommand interface {
	DoListModels(p *services.ProviderService, r *http.Request) ([]ListModelsModel, error)
}

// InferenceStreamChunk represents a streaming response chunk
type InferenceStreamChunk struct {
	Data         styles.PartialJSON
	RuntimeError error
}

// InferenceCommand is the unified interface for all inference APIs.
// Each driver (Chat Completions, Responses, Anthropic Messages, etc.)
// implements this interface to handle requests in their native format.
// The module handles format conversion at the boundary.
type InferenceCommand interface {
	// DoInference sends a non-streaming inference request
	DoInference(p *services.ProviderService, reqJson styles.PartialJSON, r *http.Request) (*http.Response, styles.PartialJSON, error)
	// DoInferenceStream sends a streaming inference request
	DoInferenceStream(p *services.ProviderService, reqJson styles.PartialJSON, r *http.Request) (*http.Response, chan InferenceStreamChunk, error)
}
