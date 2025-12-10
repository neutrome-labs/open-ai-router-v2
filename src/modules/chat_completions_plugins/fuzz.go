package chatcompletionsplugins

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/neutrome-labs/open-ai-router-v2/src/commands"
	"github.com/neutrome-labs/open-ai-router-v2/src/services"
	"go.uber.org/zap"
)

type Fuzz struct {
	knownModelsCache sync.Map
}

func (f *Fuzz) Before(params string, p *services.ProviderImpl, r *http.Request, body []byte) ([]byte, error) {
	var req map[string]any
	err := json.Unmarshal(body, &req)
	if err != nil {
		Logger.Debug("fuzz failed to unmarshal body", zap.Error(err))
		return body, nil
	}

	requestedModel, _ := req["model"].(string)
	Logger.Debug("fuzz plugin before hook",
		zap.String("provider", p.Name),
		zap.String("requestedModel", requestedModel))

	cacheKey := p.Name + "_" + requestedModel
	if model, ok := f.knownModelsCache.Load(cacheKey); ok {
		Logger.Debug("fuzz cache HIT",
			zap.String("cacheKey", cacheKey),
			zap.String("resolvedModel", model.(string)))
		req["model"] = model
		return json.Marshal(req)
	}
	Logger.Debug("fuzz cache MISS", zap.String("cacheKey", cacheKey))

	if len(p.Commands) == 0 {
		Logger.Debug("fuzz no commands available")
		return body, nil
	}

	if _, ok := p.Commands["list_models"]; !ok {
		Logger.Debug("fuzz list_models command not available")
		return body, nil
	}

	listModelsCmd, ok := p.Commands["list_models"].(commands.ListModelsCommand)
	if !ok {
		Logger.Debug("fuzz list_models command type assertion failed")
		return body, nil
	}

	Logger.Debug("fuzz fetching models from provider")
	models, err := listModelsCmd.DoListModels(p, r)
	if err != nil {
		Logger.Debug("fuzz failed to list models", zap.Error(err))
		return body, nil
	}
	Logger.Debug("fuzz fetched models", zap.Int("count", len(models)))

	for _, model := range models {
		if strings.Contains(model.ID, requestedModel) {
			Logger.Debug("fuzz found matching model",
				zap.String("requestedModel", requestedModel),
				zap.String("resolvedModel", model.ID))
			req["model"] = model.ID
			f.knownModelsCache.Store(cacheKey, model.ID)
			return json.Marshal(req)
		}
	}

	Logger.Debug("fuzz no matching model found",
		zap.String("requestedModel", requestedModel))
	return body, nil
}

func (f *Fuzz) After(params string, p *services.ProviderImpl, r *http.Request, body []byte, hres *http.Response, res map[string]any) (map[string]any, error) {
	return res, nil
}
