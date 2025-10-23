package commands

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router-v2/src/formats"
	"github.com/neutrome-labs/open-ai-router-v2/src/service"
)

type ChatCompletionsCommand interface {
	DoChatCompletions(p *service.ProviderImpl, data *formats.ChatCompletionsRequest, r *http.Request) (*http.Response, formats.ChatCompletionsResponse, error)
	DoChatCompletionsStream(p *service.ProviderImpl, data *formats.ChatCompletionsRequest, r *http.Request) (*http.Response, chan formats.ChatCompletionsStreamResponseChunk, error)
}
