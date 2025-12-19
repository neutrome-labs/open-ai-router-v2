package plugins

import (
	"net/http"
	"strings"
	"sync"

	"github.com/neutrome-labs/open-ai-router/src/drivers"
	"github.com/neutrome-labs/open-ai-router/src/plugin"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/styles"
	"go.uber.org/zap"
)

// Fuzz provides fuzzy model name matching
type Fuzz struct {
	knownModelsCache sync.Map
}

func (f *Fuzz) Name() string { return "fuzz" }

func (f *Fuzz) Before(params string, p *services.ProviderService, r *http.Request, reqJson styles.PartialJSON) (styles.PartialJSON, error) {
	// Fuzz requires a provider to list models - skip if not provided
	if p == nil {
		Logger.Debug("fuzz plugin skipped - no provider context")
		return reqJson, nil
	}

	model := styles.TryGetFromPartialJSON[string](reqJson, "model")
	cacheKey := p.Name + "_" + model

	Logger.Debug("fuzz plugin before hook",
		zap.String("provider", p.Name),
		zap.String("requestedModel", model))

	// Check cache first
	if cachedModel, ok := f.knownModelsCache.Load(cacheKey); ok {
		Logger.Debug("fuzz cache HIT",
			zap.String("cacheKey", cacheKey),
			zap.String("resolvedModel", cachedModel.(string)))
		return reqJson.CloneWith("model", cachedModel.(string))
	}
	Logger.Debug("fuzz cache MISS", zap.String("cacheKey", cacheKey))

	// Try to find matching model from provider
	if len(p.Commands) == 0 {
		Logger.Debug("fuzz no commands available")
		return reqJson, nil
	}

	listCmd, ok := p.Commands["list_models"]
	if !ok {
		Logger.Debug("fuzz list_models command not available")
		return reqJson, nil
	}

	type ListModelsCommand interface {
		DoListModels(p *services.ProviderService, r *http.Request) ([]drivers.ListModelsModel, error)
	}

	cmd, ok := listCmd.(ListModelsCommand)
	if !ok {
		Logger.Debug("fuzz list_models command type assertion failed")
		return reqJson, nil
	}

	Logger.Debug("fuzz fetching models from provider")
	models, err := cmd.DoListModels(p, r)
	if err != nil {
		Logger.Debug("fuzz failed to list models", zap.Error(err))
		return reqJson, nil
	}
	Logger.Debug("fuzz fetched models", zap.Int("count", len(models)))

	// Find matching model
	for _, m := range models {
		if strings.Contains(m.ID, model) {
			Logger.Debug("fuzz found matching model",
				zap.String("requestedModel", model),
				zap.String("resolvedModel", m.ID))
			f.knownModelsCache.Store(cacheKey, m.ID)
			return reqJson.CloneWith("model", m.ID)
		}
	}

	Logger.Debug("fuzz no matching model found",
		zap.String("requestedModel", model))
	return reqJson, nil
}

var (
	_ plugin.BeforePlugin = (*Fuzz)(nil)
)
