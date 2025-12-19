// Package virtual provides a virtual driver for model aliasing and presets.
// Virtual providers don't connect to external APIs - they delegate to other
// providers via the recursive handler plugin pattern, allowing named model presets.
package virtual

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/neutrome-labs/open-ai-router/src/drivers"
	"github.com/neutrome-labs/open-ai-router/src/plugin"
	"github.com/neutrome-labs/open-ai-router/src/services"
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
	reqBody []byte,
	w http.ResponseWriter,
	r *http.Request,
) (handled bool, err error) {
	// Parse request to get model name
	var reqMap map[string]json.RawMessage
	if err := json.Unmarshal(reqBody, &reqMap); err != nil {
		return false, nil // Let normal flow handle parse errors
	}

	var modelName string
	if modelRaw, ok := reqMap["model"]; ok {
		if err := json.Unmarshal(modelRaw, &modelName); err != nil {
			return false, nil // Let normal flow handle parse errors
		}
	} else {
		return false, nil // No model field, let normal flow handle
	}

	// Check if this is targeting our virtual provider
	// Format: "virtualProvider/modelName"
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

	// Look up the target model for this virtual model
	targetModel, ok := v.ModelMappings[actualModel]
	if !ok || targetModel == "" {
		return false, nil // Model not in our mappings, let normal flow handle it
	}

	Logger.Debug("VirtualPlugin handling request",
		zap.String("provider", v.ProviderName),
		zap.String("virtual_model", actualModel),
		zap.String("target_model", targetModel))

	// Rewrite model in request to the target
	reqMap["model"], err = json.Marshal(targetModel)
	if err != nil {
		return true, err
	}

	// Marshal the modified request back to JSON
	newReqBody, err := json.Marshal(reqMap)
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
