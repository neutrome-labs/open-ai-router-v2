package chatcompletionsplugins

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router-v2/src/services"
)

type ChatCompletionsPlugin interface {
	Before(params string, p *services.ProviderImpl, r *http.Request, body []byte) ([]byte, error)
	After(params string, p *services.ProviderImpl, r *http.Request, body []byte, hres *http.Response, res map[string]any) (map[string]any, error)
}
