package chatcompletionsplugins

import (
	"github.com/neutrome-labs/open-ai-router-v2/src/formats"
	"github.com/neutrome-labs/open-ai-router-v2/src/service"
)

type Fuzz struct{}

func (f *Fuzz) Before(params string, p *service.ProviderImpl, req *formats.ChatCompletionsRequest) error {
	return nil
}

func (f *Fuzz) After(params string, p *service.ProviderImpl, req *formats.ChatCompletionsRequest, res *formats.ChatCompletionsResponse) error {
	return nil
}
