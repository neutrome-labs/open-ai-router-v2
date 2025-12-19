// Package plugins provides the v3 plugin system for chat completions.
// V3 upgrade: Plugins have separate methods for streaming vs non-streaming,
// plus StreamEnd handlers for finalizing when no usage data is available in stream.
package plugin

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/styles"
	"go.uber.org/zap"
)

// Logger for plugin chain - can be set by modules
var Logger *zap.Logger = zap.NewNop()

// Context keys
type contextKey string

const (
	traceIDKey contextKey = "trace_id"
	userIDKey  contextKey = "user_id"
	keyIDKey   contextKey = "key_id"
)

// ContextTraceID returns the trace ID context key
func ContextTraceID() contextKey { return traceIDKey }

// ContextUserID returns the user ID context key
func ContextUserID() contextKey { return userIDKey }

// ContextKeyID returns the key ID context key
func ContextKeyID() contextKey { return keyIDKey }

// Plugin is the base interface for all chat completion plugins
type Plugin interface {
	// Name returns the plugin's identifier
	Name() string
}

// HandlerInvoker allows plugins to invoke the outer handler recursively.
// This is used by plugins like "models" (fallback) and "parallel" (fan-out).
type HandlerInvoker interface {
	// InvokeHandler invokes the outer handler with the given request.
	// The request should already have the model set appropriately.
	// Returns nil on success, error on failure.
	InvokeHandler(w http.ResponseWriter, r *http.Request) error

	// InvokeHandlerCapture invokes the handler and captures the response instead of writing to w.
	// Used by parallel plugin to capture multiple responses for merging.
	// Returns the captured response on success, or error on failure.
	InvokeHandlerCapture(r *http.Request) (styles.PartialJSON, error)
}

// BeforePlugin processes requests before sending to provider
type BeforePlugin interface {
	Plugin
	// Before is called before the request is sent to the provider
	// Returns the modified request body
	Before(params string, p *services.ProviderService, r *http.Request, reqJson styles.PartialJSON) (styles.PartialJSON, error)
}

// AfterPlugin processes non-streaming responses
type AfterPlugin interface {
	Plugin
	// After is called after receiving a complete (non-streaming) response
	After(params string, p *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, res *http.Response, resJson styles.PartialJSON) (styles.PartialJSON, error)
}

// StreamChunkPlugin processes individual streaming chunks
type StreamChunkPlugin interface {
	Plugin
	// AfterChunk is called for each streaming chunk
	AfterChunk(params string, p *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, res *http.Response, chunk styles.PartialJSON) (styles.PartialJSON, error)
}

// StreamEndPlugin handles stream completion, useful for finalization when no usage data in stream
type StreamEndPlugin interface {
	Plugin
	// StreamEnd is called when the stream completes
	// lastChunk may be nil if no chunks were received or the last chunk had no usage
	// This allows plugins to finalize state, compute estimated usage, etc.
	StreamEnd(params string, p *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, res *http.Response, lastChunk styles.PartialJSON) error
}

// ErrorPlugin handles errors from provider calls
type ErrorPlugin interface {
	Plugin
	// OnError is called when a provider call fails
	// res may be nil if the error occurred before receiving a response
	// providerErr is the error returned by the provider
	OnError(params string, p *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, res *http.Response, providerErr error) error
}

// RecursiveHandlerPlugin can intercept the request flow and invoke the handler recursively.
// This is used for plugins that need to make multiple calls (fallback, parallel, etc.).
// When RecursiveHandler returns handled=true, the module should not proceed with normal flow.
type RecursiveHandlerPlugin interface {
	Plugin
	// RecursiveHandler is called before normal provider iteration.
	// If handled is true, the plugin has handled the request (either success or error).
	// If handled is false, normal provider iteration should proceed.
	RecursiveHandler(
		params string,
		invoker HandlerInvoker,
		reqJson styles.PartialJSON,
		w http.ResponseWriter,
		r *http.Request,
	) (handled bool, err error)
}

// PluginInstance represents a plugin with its parameters
type PluginInstance struct {
	Plugin Plugin
	Params string
}
