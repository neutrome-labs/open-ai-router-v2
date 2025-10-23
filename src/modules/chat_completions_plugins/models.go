package chatcompletionsplugins

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router-v2/src/formats"
	"github.com/neutrome-labs/open-ai-router-v2/src/service"
)

type Models struct{}

func (f *Models) Before(params string, p *service.ProviderImpl, r *http.Request, req *formats.ChatCompletionsRequest) error {
	return nil
}

func (f *Models) After(params string, p *service.ProviderImpl, r *http.Request, req *formats.ChatCompletionsRequest, hres *http.Response, res *formats.ChatCompletionsResponse) error {
	return nil
}
