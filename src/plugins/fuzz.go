package plugins

import (
	"net/http"
	"strings"
	"sync"

	"github.com/neutrome-labs/open-ai-router/src/drivers"
	"github.com/neutrome-labs/open-ai-router/src/formats"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"go.uber.org/zap"
)

// Fuzz provides fuzzy model name matching
type Fuzz struct {
	knownModelsCache sync.Map
}

func (f *Fuzz) Name() string { return "fuzz" }

func (f *Fuzz) Before(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest) (formats.ManagedRequest, error) {
	// Fuzz requires a provider to list models - skip if not provided
	if p == nil {
		if Logger != nil {
			Logger.Debug("fuzz plugin skipped - no provider context")
		}
		return req, nil
	}

	model := req.GetModel()
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
		req.SetModel(cachedModel.(string))
		return req, nil
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
		DoListModels(p *services.ProviderImpl, r *http.Request) ([]drivers.ListModelsModel, error)
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
			req.SetModel(m.ID)
			f.knownModelsCache.Store(cacheKey, m.ID)
			return req, nil
		}
	}

	if Logger != nil {
		Logger.Debug("fuzz no matching model found",
			zap.String("requestedModel", model))
	}
	return req, nil
}

func (f *Fuzz) After(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, res formats.ManagedResponse) (formats.ManagedResponse, error) {
	return res, nil
}

func (f *Fuzz) AfterChunk(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, chunk formats.ManagedResponse) (formats.ManagedResponse, error) {
	return chunk, nil
}

func (f *Fuzz) StreamEnd(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, lastChunk formats.ManagedResponse) error {
	return nil
}
