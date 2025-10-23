package chatcompletionsplugins

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router-v2/src/formats"
	"github.com/neutrome-labs/open-ai-router-v2/src/service"
)

type TSTools struct{}

func (f *TSTools) Before(params string, p *service.ProviderImpl, r *http.Request, req *formats.ChatCompletionsRequest) error {
	return nil
}

func (f *TSTools) After(params string, p *service.ProviderImpl, r *http.Request, req *formats.ChatCompletionsRequest, hres *http.Response, res *formats.ChatCompletionsResponse) error {
	return nil
}
