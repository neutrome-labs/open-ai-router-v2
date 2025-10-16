package cfai

import (
	"fmt"
	"net/http"

	"github.com/neutrome-labs/open-ai-router-v2/src/commands"
	"github.com/neutrome-labs/open-ai-router-v2/src/service"
)

type ListModels struct {
}

func (c *ListModels) DoListModels(p *service.ProviderImpl, r *http.Request) ([]commands.ListModelsModel, error) {
	return nil, fmt.Errorf("not implemented")
}
