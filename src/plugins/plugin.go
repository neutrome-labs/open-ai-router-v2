// Package plugins provides the v3 plugin system for chat completions.
// V3 upgrade: Plugins have separate methods for streaming vs non-streaming,
// plus StreamEnd handlers for finalizing when no usage data is available in stream.
package plugins

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router/src/formats"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"go.uber.org/zap"
)

// Logger for plugin chain - can be set by modules
var Logger *zap.Logger

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
	InvokeHandler(w http.ResponseWriter, r *http.Request, req formats.ManagedRequest) error

	// InvokeHandlerCapture invokes the handler and captures the response instead of writing to w.
	// Used by parallel plugin to capture multiple responses for merging.
	// Returns the captured response on success, or error on failure.
	InvokeHandlerCapture(r *http.Request, req formats.ManagedRequest) (formats.ManagedResponse, error)
}

// BeforePlugin processes requests before sending to provider
type BeforePlugin interface {
	Plugin
	// Before is called before the request is sent to the provider
	// Returns the modified request body
	Before(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest) (formats.ManagedRequest, error)
}

// AfterPlugin processes non-streaming responses
type AfterPlugin interface {
	Plugin
	// After is called after receiving a complete (non-streaming) response
	After(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, res formats.ManagedResponse) (formats.ManagedResponse, error)
}

// StreamChunkPlugin processes individual streaming chunks
type StreamChunkPlugin interface {
	Plugin
	// AfterChunk is called for each streaming chunk
	AfterChunk(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, chunk formats.ManagedResponse) (formats.ManagedResponse, error)
}

// StreamEndPlugin handles stream completion, useful for finalization when no usage data in stream
type StreamEndPlugin interface {
	Plugin
	// StreamEnd is called when the stream completes
	// lastChunk may be nil if no chunks were received or the last chunk had no usage
	// This allows plugins to finalize state, compute estimated usage, etc.
	StreamEnd(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, lastChunk formats.ManagedResponse) error
}

// ErrorPlugin handles errors from provider calls
type ErrorPlugin interface {
	Plugin
	// OnError is called when a provider call fails
	// hres may be nil if the error occurred before receiving a response
	// providerErr is the error returned by the provider
	OnError(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, providerErr error) error
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
		w http.ResponseWriter,
		r *http.Request,
		req formats.ManagedRequest,
	) (handled bool, err error)
}

// FullPlugin implements all plugin interfaces
type FullPlugin interface {
	BeforePlugin
	AfterPlugin
	StreamChunkPlugin
	StreamEndPlugin
	ErrorPlugin
}

// PluginInstance represents a plugin with its parameters
type PluginInstance struct {
	Plugin Plugin
	Params string
}

// PluginChain manages the execution of plugins
type PluginChain struct {
	plugins []PluginInstance
}

// NewPluginChain creates a new plugin chain
func NewPluginChain() *PluginChain {
	return &PluginChain{
		plugins: make([]PluginInstance, 0),
	}
}

// Add adds a plugin to the chain
func (c *PluginChain) Add(p Plugin, params string) {
	c.plugins = append(c.plugins, PluginInstance{Plugin: p, Params: params})
}

// RunBefore executes all BeforePlugin implementations
func (c *PluginChain) RunBefore(p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest) (formats.ManagedRequest, error) {
	if Logger != nil {
		Logger.Debug("RunBefore starting", zap.Int("plugin_count", len(c.plugins)), zap.String("model", req.GetModel()))
	}
	current := req
	for _, pi := range c.plugins {
		if bp, ok := pi.Plugin.(BeforePlugin); ok {
			if Logger != nil {
				Logger.Debug("Running Before plugin", zap.String("plugin", pi.Plugin.Name()), zap.String("params", pi.Params))
			}
			next, err := bp.Before(pi.Params, p, r, current)
			if err != nil {
				if Logger != nil {
					Logger.Error("Before plugin failed", zap.String("plugin", pi.Plugin.Name()), zap.Error(err))
				}
				return nil, err
			}
			current = next
		}
	}
	if Logger != nil {
		Logger.Debug("RunBefore completed", zap.String("model", current.GetModel()))
	}
	return current, nil
}

// RunAfter executes all AfterPlugin implementations
func (c *PluginChain) RunAfter(p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, res formats.ManagedResponse) (formats.ManagedResponse, error) {
	if Logger != nil {
		Logger.Debug("RunAfter starting", zap.Int("plugin_count", len(c.plugins)))
	}
	current := res
	for _, pi := range c.plugins {
		if ap, ok := pi.Plugin.(AfterPlugin); ok {
			if Logger != nil {
				Logger.Debug("Running After plugin", zap.String("plugin", pi.Plugin.Name()), zap.String("params", pi.Params))
			}
			next, err := ap.After(pi.Params, p, r, req, hres, current)
			if err != nil {
				if Logger != nil {
					Logger.Error("After plugin failed", zap.String("plugin", pi.Plugin.Name()), zap.Error(err))
				}
				return nil, err
			}
			current = next
		}
	}
	if Logger != nil {
		Logger.Debug("RunAfter completed")
	}
	return current, nil
}

// RunAfterChunk executes all StreamChunkPlugin implementations
func (c *PluginChain) RunAfterChunk(p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, chunk formats.ManagedResponse) (formats.ManagedResponse, error) {
	current := chunk
	for _, pi := range c.plugins {
		if sp, ok := pi.Plugin.(StreamChunkPlugin); ok {
			next, err := sp.AfterChunk(pi.Params, p, r, req, hres, current)
			if err != nil {
				if Logger != nil {
					Logger.Error("AfterChunk plugin failed", zap.String("plugin", pi.Plugin.Name()), zap.Error(err))
				}
				return nil, err
			}
			current = next
		}
	}
	return current, nil
}

// RunStreamEnd executes all StreamEndPlugin implementations
func (c *PluginChain) RunStreamEnd(p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, lastChunk formats.ManagedResponse) error {
	if Logger != nil {
		Logger.Debug("RunStreamEnd starting", zap.Int("plugin_count", len(c.plugins)))
	}
	for _, pi := range c.plugins {
		if sep, ok := pi.Plugin.(StreamEndPlugin); ok {
			if Logger != nil {
				Logger.Debug("Running StreamEnd plugin", zap.String("plugin", pi.Plugin.Name()), zap.String("params", pi.Params))
			}
			if err := sep.StreamEnd(pi.Params, p, r, req, hres, lastChunk); err != nil {
				if Logger != nil {
					Logger.Error("StreamEnd plugin failed", zap.String("plugin", pi.Plugin.Name()), zap.Error(err))
				}
				return err
			}
		}
	}
	if Logger != nil {
		Logger.Debug("RunStreamEnd completed")
	}
	return nil
}

// RunError executes all ErrorPlugin implementations
func (c *PluginChain) RunError(p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, providerErr error) error {
	if Logger != nil {
		Logger.Debug("RunError starting", zap.Int("plugin_count", len(c.plugins)), zap.Error(providerErr))
	}
	for _, pi := range c.plugins {
		if ep, ok := pi.Plugin.(ErrorPlugin); ok {
			if Logger != nil {
				Logger.Debug("Running Error plugin", zap.String("plugin", pi.Plugin.Name()), zap.String("params", pi.Params))
			}
			if err := ep.OnError(pi.Params, p, r, req, hres, providerErr); err != nil {
				if Logger != nil {
					Logger.Error("Error plugin failed", zap.String("plugin", pi.Plugin.Name()), zap.Error(err))
				}
				// Don't return - continue running other error plugins
			}
		}
	}
	if Logger != nil {
		Logger.Debug("RunError completed")
	}
	return nil
}

// RunRecursiveHandlers executes all RecursiveHandlerPlugin implementations.
// Returns (true, nil) if a plugin handled the request successfully.
// Returns (true, err) if a plugin handled the request but failed.
// Returns (false, nil) if no plugin wants to handle the request recursively.
func (c *PluginChain) RunRecursiveHandlers(invoker HandlerInvoker, w http.ResponseWriter, r *http.Request, req formats.ManagedRequest) (bool, error) {
	if Logger != nil {
		Logger.Debug("RunRecursiveHandlers starting", zap.Int("plugin_count", len(c.plugins)))
	}
	for _, pi := range c.plugins {
		if rh, ok := pi.Plugin.(RecursiveHandlerPlugin); ok {
			if Logger != nil {
				Logger.Debug("Running RecursiveHandler plugin", zap.String("plugin", pi.Plugin.Name()), zap.String("params", pi.Params))
			}
			handled, err := rh.RecursiveHandler(pi.Params, invoker, w, r, req)
			if handled {
				if Logger != nil {
					if err != nil {
						Logger.Debug("RecursiveHandler plugin handled with error", zap.String("plugin", pi.Plugin.Name()), zap.Error(err))
					} else {
						Logger.Debug("RecursiveHandler plugin handled successfully", zap.String("plugin", pi.Plugin.Name()))
					}
				}
				return true, err
			}
		}
	}
	if Logger != nil {
		Logger.Debug("RunRecursiveHandlers completed - no plugin handled")
	}
	return false, nil
}

// GetPlugins returns all plugins in the chain
func (c *PluginChain) GetPlugins() []PluginInstance {
	return c.plugins
}
