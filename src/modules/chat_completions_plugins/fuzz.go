package chatcompletionsplugins

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/neutrome-labs/open-ai-router-v2/src/commands"
	"github.com/neutrome-labs/open-ai-router-v2/src/services"
)

type Fuzz struct {
	knownModelsCache sync.Map
}

func (f *Fuzz) Before(params string, p *services.ProviderImpl, r *http.Request, body []byte) ([]byte, error) {
	var req map[string]any
	err := json.Unmarshal(body, &req)
	if err != nil {
		return body, nil
	}

	cacheKey := p.Name + "_" + req["model"].(string)
	if model, ok := f.knownModelsCache.Load(cacheKey); ok {
		req["model"] = model
		return json.Marshal(req)
	}

	if p.Commands == nil || len(p.Commands) == 0 {
		return body, nil
	}

	if _, ok := p.Commands["list_models"]; !ok {
		return body, nil
	}

	listModelsCmd, ok := p.Commands["list_models"].(commands.ListModelsCommand)
	if !ok {
		return body, nil
	}

	models, err := listModelsCmd.DoListModels(p, r)
	if err != nil {
		return body, nil
	}

	for _, model := range models {
		if strings.Contains(model.ID, req["model"].(string)) {
			req["model"] = model.ID
			f.knownModelsCache.Store(cacheKey, req["model"])
			return json.Marshal(req)
		}
	}

	return body, nil
}

func (f *Fuzz) After(params string, p *services.ProviderImpl, r *http.Request, body []byte, hres *http.Response, res map[string]any) (map[string]any, error) {
	return res, nil
}
