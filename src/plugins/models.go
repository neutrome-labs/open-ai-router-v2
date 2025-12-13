package plugins

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router/src/formats"
	"github.com/neutrome-labs/open-ai-router/src/services"
)

// Models handles models body param to make parallel requests to different models
type Models struct{}

func (m *Models) Name() string { return "models" }

func (m *Models) Before(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest) (formats.ManagedRequest, error) {
	// TODO: Implement additional reqests firing based on models param
	return req, nil
}

func (m *Models) After(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, res formats.ManagedResponse) (formats.ManagedResponse, error) {
	return res, nil
}

func (m *Models) AfterChunk(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, chunk formats.ManagedResponse) (formats.ManagedResponse, error) {
	return chunk, nil
}

func (m *Models) StreamEnd(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, lastChunk formats.ManagedResponse) error {
	return nil
}
