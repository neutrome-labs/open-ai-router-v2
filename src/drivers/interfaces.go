// Package drivers provides command interfaces for provider interactions.
package drivers

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router/src/formats"
	"github.com/neutrome-labs/open-ai-router/src/services"
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
	DoListModels(p *services.ProviderImpl, r *http.Request) ([]ListModelsModel, error)
}

// ChatCompletionsStreamChunk represents a streaming response chunk
type ChatCompletionsStreamChunk struct {
	RuntimeError error
	Data         formats.ManagedResponse
}

// ChatCompletionsCommand handles chat completions requests
type ChatCompletionsCommand interface {
	// DoChatCompletions sends a non-streaming request
	DoChatCompletions(p *services.ProviderImpl, req formats.ManagedRequest, r *http.Request) (*http.Response, formats.ManagedResponse, error)
	// DoChatCompletionsStream sends a streaming request
	DoChatCompletionsStream(p *services.ProviderImpl, req formats.ManagedRequest, r *http.Request) (*http.Response, chan ChatCompletionsStreamChunk, error)
}

// ResponsesCommand handles OpenAI Responses API requests
type ResponsesCommand interface {
	// DoResponses sends a non-streaming request
	DoResponses(p *services.ProviderImpl, req formats.ManagedRequest, r *http.Request) (*http.Response, formats.ManagedResponse, error)
	// DoResponsesStream sends a streaming request
	DoResponsesStream(p *services.ProviderImpl, req formats.ManagedRequest, r *http.Request) (*http.Response, chan ChatCompletionsStreamChunk, error)
}

// MessagesCommand handles Anthropic Messages API requests
type MessagesCommand interface {
	// DoMessages sends a non-streaming request
	DoMessages(p *services.ProviderImpl, req formats.ManagedRequest, r *http.Request) (*http.Response, formats.ManagedResponse, error)
	// DoMessagesStream sends a streaming request
	DoMessagesStream(p *services.ProviderImpl, req formats.ManagedRequest, r *http.Request) (*http.Response, chan ChatCompletionsStreamChunk, error)
}
