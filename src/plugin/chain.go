package plugin

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/styles"
	"go.uber.org/zap"
)

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
func (c *PluginChain) RunBefore(p *services.ProviderService, r *http.Request, reqJson styles.PartialJSON) (styles.PartialJSON, error) {
	Logger.Debug("RunBefore starting", zap.Int("plugin_count", len(c.plugins)) /*zap.String("model", req.GetModel())*/)
	current := reqJson
	for _, pi := range c.plugins {
		if bp, ok := pi.Plugin.(BeforePlugin); ok {
			Logger.Debug("Running Before plugin", zap.String("plugin", pi.Plugin.Name()), zap.String("params", pi.Params))
			next, err := bp.Before(pi.Params, p, r, current)
			if err != nil {
				Logger.Error("Before plugin failed", zap.String("plugin", pi.Plugin.Name()), zap.Error(err))
				return nil, err
			}
			current = next
		}
	}
	Logger.Debug("RunBefore completed" /*zap.String("model", current.GetModel())*/)
	return current, nil
}

// RunAfter executes all AfterPlugin implementations
func (c *PluginChain) RunAfter(p *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, res *http.Response, resJson styles.PartialJSON) (styles.PartialJSON, error) {
	Logger.Debug("RunAfter starting", zap.Int("plugin_count", len(c.plugins)))
	current := resJson
	for _, pi := range c.plugins {
		if ap, ok := pi.Plugin.(AfterPlugin); ok {
			Logger.Debug("Running After plugin", zap.String("plugin", pi.Plugin.Name()), zap.String("params", pi.Params))
			next, err := ap.After(pi.Params, p, r, reqJson, res, current)
			if err != nil {
				Logger.Error("After plugin failed", zap.String("plugin", pi.Plugin.Name()), zap.Error(err))
				return nil, err
			}
			current = next
		}
	}
	Logger.Debug("RunAfter completed")
	return current, nil
}

// RunAfterChunk executes all StreamChunkPlugin implementations
func (c *PluginChain) RunAfterChunk(p *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, res *http.Response, chunk styles.PartialJSON) (styles.PartialJSON, error) {
	current := chunk
	for _, pi := range c.plugins {
		if sp, ok := pi.Plugin.(StreamChunkPlugin); ok {
			next, err := sp.AfterChunk(pi.Params, p, r, reqJson, res, current)
			if err != nil {
				Logger.Error("AfterChunk plugin failed", zap.String("plugin", pi.Plugin.Name()), zap.Error(err))
				return nil, err
			}
			current = next
		}
	}
	return current, nil
}

// RunStreamEnd executes all StreamEndPlugin implementations
func (c *PluginChain) RunStreamEnd(p *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, res *http.Response, lastChunk styles.PartialJSON) error {
	Logger.Debug("RunStreamEnd starting", zap.Int("plugin_count", len(c.plugins)))
	for _, pi := range c.plugins {
		if sep, ok := pi.Plugin.(StreamEndPlugin); ok {
			Logger.Debug("Running StreamEnd plugin", zap.String("plugin", pi.Plugin.Name()), zap.String("params", pi.Params))
			if err := sep.StreamEnd(pi.Params, p, r, reqJson, res, lastChunk); err != nil {
				Logger.Error("StreamEnd plugin failed", zap.String("plugin", pi.Plugin.Name()), zap.Error(err))
				return err
			}
		}
	}
	Logger.Debug("RunStreamEnd completed")
	return nil
}

// RunError executes all ErrorPlugin implementations
func (c *PluginChain) RunError(p *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, res *http.Response, providerErr error) error {
	Logger.Debug("RunError starting", zap.Int("plugin_count", len(c.plugins)), zap.Error(providerErr))
	for _, pi := range c.plugins {
		if ep, ok := pi.Plugin.(ErrorPlugin); ok {
			Logger.Debug("Running Error plugin", zap.String("plugin", pi.Plugin.Name()), zap.String("params", pi.Params))
			if err := ep.OnError(pi.Params, p, r, reqJson, res, providerErr); err != nil {
				Logger.Error("Error plugin failed", zap.String("plugin", pi.Plugin.Name()), zap.Error(err))
				// Don't return - continue running other error plugins
			}
		}
	}
	Logger.Debug("RunError completed")
	return nil
}

// RunRecursiveHandlers executes all RecursiveHandlerPlugin implementations.
// Returns (true, nil) if a plugin handled the request successfully.
// Returns (true, err) if a plugin handled the request but failed.
// Returns (false, nil) if no plugin wants to handle the request recursively.
func (c *PluginChain) RunRecursiveHandlers(invoker HandlerInvoker, reqJson styles.PartialJSON, w http.ResponseWriter, r *http.Request) (bool, error) {
	Logger.Debug("RunRecursiveHandlers starting", zap.Int("plugin_count", len(c.plugins)))
	for _, pi := range c.plugins {
		if rh, ok := pi.Plugin.(RecursiveHandlerPlugin); ok {
			Logger.Debug("Running RecursiveHandler plugin", zap.String("plugin", pi.Plugin.Name()), zap.String("params", pi.Params))
			handled, err := rh.RecursiveHandler(pi.Params, invoker, reqJson, w, r)
			if handled {
				if err != nil {
					Logger.Debug("RecursiveHandler plugin handled with error", zap.String("plugin", pi.Plugin.Name()), zap.Error(err))
				} else {
					Logger.Debug("RecursiveHandler plugin handled successfully", zap.String("plugin", pi.Plugin.Name()))
				}
				return true, err
			}
		}
	}
	Logger.Debug("RunRecursiveHandlers completed - no plugin handled")
	return false, nil
}

// GetPlugins returns all plugins in the chain
func (c *PluginChain) GetPlugins() []PluginInstance {
	return c.plugins
}
