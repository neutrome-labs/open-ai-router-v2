package chatcompletionsplugins

import (
	"github.com/neutrome-labs/open-ai-router-v2/src/formats"
	"github.com/neutrome-labs/open-ai-router-v2/src/service"
)

type ChatCompletionsPlugin interface {
	Before(params string, p *service.ProviderImpl, req *formats.ChatCompletionsRequest) error
	After(params string, p *service.ProviderImpl, req *formats.ChatCompletionsRequest, res *formats.ChatCompletionsResponse) error
}
