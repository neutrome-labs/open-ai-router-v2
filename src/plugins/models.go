package plugins

import (
	"net/http"
	"strings"

	"github.com/neutrome-labs/open-ai-router/src/formats"
	"go.uber.org/zap"
)

// Models handles comma-separated models in the model field for fallback ordering.
// Example: model="gpt-5,gpt-4.1,deepseek-r1" will try all providers for gpt-5 first,
// and only if all providers fail, try gpt-4.1 across all providers, etc.
//
// This plugin implements RecursiveHandlerPlugin to recursively invoke the outer handler
// with the next model when all providers for the current model fail.
type Models struct{}

func (m *Models) Name() string { return "models" }

// RecursiveHandler implements the fallback logic by trying models in sequence.
// For each model, all providers are tried (via the handler). Only when all providers
// fail for a model does it move to the next model in the fallback list.
func (m *Models) RecursiveHandler(
	params string,
	invoker HandlerInvoker,
	w http.ResponseWriter,
	r *http.Request,
	req formats.ManagedRequest,
) (handled bool, err error) {
	model := req.GetModel()

	// Parse comma-separated models (strip plugin suffix first)
	models, pluginSuffix := parseModelListForFallback(model)
	if len(models) <= 1 {
		// Single model or no models - let normal flow handle it
		return false, nil
	}

	if Logger != nil {
		Logger.Debug("models plugin starting fallback chain",
			zap.Strings("models", models),
			zap.String("plugin_suffix", pluginSuffix))
	}

	var lastErr error
	for i, currentModel := range models {
		if Logger != nil {
			Logger.Debug("models plugin trying model (all providers)",
				zap.Int("index", i),
				zap.String("model", currentModel))
		}

		// Clone request and set current model (WITHOUT plugin suffix to avoid re-parsing)
		clonedReq := req.Clone()
		clonedReq.SetModel(currentModel)

		// Invoke the handler with the single model
		// This will try ALL providers for this model
		err := invoker.InvokeHandler(w, r, clonedReq)
		if err == nil {
			// Success - one of the providers worked!
			return true, nil
		}

		lastErr = err
		if Logger != nil {
			Logger.Debug("models plugin: all providers failed for model, trying next model",
				zap.String("model", currentModel),
				zap.Error(err))
		}
	}

	// All models (and all their providers) failed
	if Logger != nil {
		Logger.Error("models plugin: all models exhausted",
			zap.Strings("models", models),
			zap.Error(lastErr))
	}

	return true, lastErr
}

// parseModelListForFallback parses a comma-separated model string into a list.
// Returns the list of models (without plugin suffix) and the plugin suffix separately.
// This ensures recursive calls don't re-parse the models.
func parseModelListForFallback(model string) ([]string, string) {
	// First, extract plugin suffix if present
	plusIdx := strings.IndexByte(model, '+')
	modelPart := model
	pluginSuffix := ""
	if plusIdx >= 0 {
		modelPart = model[:plusIdx]
		pluginSuffix = model[plusIdx:]
	}

	if !strings.Contains(modelPart, ",") {
		return []string{model}, pluginSuffix // Return original (may include plugin suffix)
	}

	parts := strings.Split(modelPart, ",")
	var models []string
	for _, m := range parts {
		m = strings.TrimSpace(m)
		if m != "" {
			// Return individual models WITHOUT plugin suffix
			// (plugin suffix is already parsed and plugins are in the chain)
			models = append(models, m)
		}
	}

	return models, pluginSuffix
}

// ParseModelList is exported for other packages that need to detect comma-separated models.
// Returns true if the model string contains comma-separated models.
func ParseModelList(model string) []string {
	models, _ := parseModelListForFallback(model)
	return models
}
