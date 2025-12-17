package plugins

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/neutrome-labs/open-ai-router/src/drivers"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"go.uber.org/zap"
)

// Fuzz provides fuzzy model name matching
type Fuzz struct {
	knownModelsCache sync.Map
}

func (f *Fuzz) Name() string { return "fuzz" }

func (f *Fuzz) Before(params string, p *services.ProviderService, r *http.Request, req json.RawMessage) (json.RawMessage, error) {
	// Fuzz requires a provider to list models - skip if not provided
	if p == nil {
		if Logger != nil {
			Logger.Debug("fuzz plugin skipped - no provider context")
		}
		return req, nil
	}

	// Extract model from request
	var known struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(req, &known); err != nil {
		return req, nil
	}
	model := known.Model
	cacheKey := p.Name + "_" + model

	if Logger != nil {
		Logger.Debug("fuzz plugin before hook",
			zap.String("provider", p.Name),
			zap.String("requestedModel", model))
	}

	// Check cache first
	if cachedModel, ok := f.knownModelsCache.Load(cacheKey); ok {
		if Logger != nil {
			Logger.Debug("fuzz cache HIT",
				zap.String("cacheKey", cacheKey),
				zap.String("resolvedModel", cachedModel.(string)))
		}
		return setModelInRequest(req, cachedModel.(string)), nil
	}
	if Logger != nil {
		Logger.Debug("fuzz cache MISS", zap.String("cacheKey", cacheKey))
	}

	// Try to find matching model from provider
	if len(p.Commands) == 0 {
		if Logger != nil {
			Logger.Debug("fuzz no commands available")
		}
		return req, nil
	}

	listCmd, ok := p.Commands["list_models"]
	if !ok {
		if Logger != nil {
			Logger.Debug("fuzz list_models command not available")
		}
		return req, nil
	}

	type ListModelsCommand interface {
		DoListModels(p *services.ProviderService, r *http.Request) ([]drivers.ListModelsModel, error)
	}

	cmd, ok := listCmd.(ListModelsCommand)
	if !ok {
		if Logger != nil {
			Logger.Debug("fuzz list_models command type assertion failed")
		}
		return req, nil
	}

	if Logger != nil {
		Logger.Debug("fuzz fetching models from provider")
	}
	models, err := cmd.DoListModels(p, r)
	if err != nil {
		if Logger != nil {
			Logger.Debug("fuzz failed to list models", zap.Error(err))
		}
		return req, nil
	}
	if Logger != nil {
		Logger.Debug("fuzz fetched models", zap.Int("count", len(models)))
	}

	// Find matching model
	for _, m := range models {
		if strings.Contains(m.ID, model) {
			if Logger != nil {
				Logger.Debug("fuzz found matching model",
					zap.String("requestedModel", model),
					zap.String("resolvedModel", m.ID))
			}
			f.knownModelsCache.Store(cacheKey, m.ID)
			return setModelInRequest(req, m.ID), nil
		}
	}

	if Logger != nil {
		Logger.Debug("fuzz no matching model found",
			zap.String("requestedModel", model))
	}
	return req, nil
}

// setModelInRequest updates the model field in a JSON request
func setModelInRequest(req json.RawMessage, model string) json.RawMessage {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(req, &data); err != nil {
		return req
	}

	modelJSON, _ := json.Marshal(model)
	data["model"] = modelJSON

	result, err := json.Marshal(data)
	if err != nil {
		return req
	}
	return result
}
