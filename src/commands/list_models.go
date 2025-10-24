package commands

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router-v2/src/services"
)

type ListModelsModel struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type ListModelsCommand interface {
	DoListModels(p *services.ProviderImpl, r *http.Request) ([]ListModelsModel, error)
}
