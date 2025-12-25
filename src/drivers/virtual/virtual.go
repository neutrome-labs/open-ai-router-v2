// Package virtual provides a virtual driver for model aliasing and presets.
// Virtual providers don't connect to external APIs - they delegate to other
// providers via the recursive handler plugin pattern, allowing named model presets.
package virtual

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/neutrome-labs/open-ai-router/src/drivers"
	"github.com/neutrome-labs/open-ai-router/src/plugin"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/styles"
	"go.uber.org/zap"
)

// Logger for virtual driver - can be set by modules
var Logger *zap.Logger = zap.NewNop()

// VirtualPlugin implements RecursiveHandlerPlugin for virtual providers.
// It intercepts requests for virtual models and redirects them to real providers.
type VirtualPlugin struct {
	// ProviderName is the name of this virtual provider
	ProviderName string
	// ModelMappings maps virtual model names to target model specs (e.g., "provider/model+plugins")
	ModelMappings map[string]string
}

// Name returns the plugin name
func (v *VirtualPlugin) Name() string {
	return "virtual:" + v.ProviderName
}

// RecursiveHandler intercepts requests for virtual models and redirects them.
func (v *VirtualPlugin) RecursiveHandler(
	params string,
	invoker plugin.HandlerInvoker,
	reqJson styles.PartialJSON,
	w http.ResponseWriter,
	r *http.Request,
) (handled bool, err error) {
	modelName := styles.TryGetFromPartialJSON[string](reqJson, "model")

	// Check if this is targeting our virtual provider
	// Format: "virtualProvider/modelName" or "virtualProvider/modelName+plugins"
	providerPrefix := ""
	actualModel := modelName
	if idx := strings.Index(modelName, "/"); idx >= 0 {
		providerPrefix = strings.ToLower(modelName[:idx])
		actualModel = modelName[idx+1:]
	}

	// Only handle if explicitly targeting this virtual provider
	if providerPrefix != v.ProviderName {
		return false, nil
	}

	// Extract plugin suffix from the model name (e.g., "model+plugin1:arg+plugin2" -> "model", "+plugin1:arg+plugin2")
	baseModel := actualModel
	pluginSuffix := ""
	if plusIdx := strings.IndexByte(actualModel, '+'); plusIdx >= 0 {
		baseModel = actualModel[:plusIdx]
		pluginSuffix = actualModel[plusIdx:] // includes the leading '+'
	}

	// Look up the target model for this virtual model (using base model without plugins)
	targetModel, ok := v.ModelMappings[baseModel]
	if !ok || targetModel == "" {
		return false, nil // Model not in our mappings, let normal flow handle it
	}

	// Merge plugins: target plugins come first, then user plugins
	// Example: target="openai/gpt-4+logger", user suffix="+skill:kitty"
	// Result: "openai/gpt-4+logger+skill:kitty"
	finalModel := targetModel + pluginSuffix

	Logger.Debug("VirtualPlugin handling request",
		zap.String("provider", v.ProviderName),
		zap.String("virtual_model", baseModel),
		zap.String("user_plugins", pluginSuffix),
		zap.String("target_model", targetModel),
		zap.String("final_model", finalModel))

	// Rewrite model in request to the target with merged plugins
	err = reqJson.Set("model", finalModel)
	if err != nil {
		return true, err
	}

	// Marshal the modified request back to JSON
	newReqBody, err := reqJson.Marshal()
	if err != nil {
		return true, err
	}

	// Create a new request with the rewritten body
	newReq := r.Clone(r.Context())
	newReq.Body = io.NopCloser(bytes.NewReader(newReqBody))
	newReq.ContentLength = int64(len(newReqBody))

	// Invoke the handler - this will write directly to w for streaming
	err = invoker.InvokeHandler(w, newReq)
	if err != nil {
		Logger.Error("VirtualPlugin target failed",
			zap.String("target", targetModel),
			zap.Error(err))
		return true, err
	}

	Logger.Debug("VirtualPlugin succeeded", zap.String("target", targetModel))
	return true, nil
}

// VirtualListModels implements ListModelsCommand for virtual providers.
// It returns the configured virtual model names.
type VirtualListModels struct {
	// ProviderName is the name of the virtual provider
	ProviderName string
	// ModelMappings contains the virtual model names
	ModelMappings map[string]string
}

// DoListModels returns the list of virtual models.
func (v *VirtualListModels) DoListModels(p *services.ProviderService, r *http.Request) ([]drivers.ListModelsModel, error) {
	Logger.Debug("VirtualListModels.DoListModels", zap.String("provider", p.Name))

	var models []drivers.ListModelsModel
	for modelName := range v.ModelMappings {
		models = append(models, drivers.ListModelsModel{
			Object:  "model",
			ID:      modelName,
			Name:    modelName,
			OwnedBy: v.ProviderName,
		})
	}

	return models, nil
}
