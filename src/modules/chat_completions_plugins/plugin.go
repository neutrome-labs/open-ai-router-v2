package chatcompletionsplugins

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router-v2/src/service"
)

type ChatCompletionsPlugin interface {
	Before(params string, p *service.ProviderImpl, r *http.Request, body []byte) ([]byte, error)
	After(params string, p *service.ProviderImpl, r *http.Request, body []byte, hres *http.Response, res map[string]any) (map[string]any, error)
}
