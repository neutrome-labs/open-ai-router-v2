package commands

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router-v2/src/service"
)

type ChatCompletionsStreamResponseChunk struct {
	RuntimeError error
	Data         map[string]any
}

type ChatCompletionsCommand interface {
	DoChatCompletions(p *service.ProviderImpl, body []byte, r *http.Request) (*http.Response, map[string]any, error)
	DoChatCompletionsStream(p *service.ProviderImpl, body []byte, r *http.Request) (*http.Response, chan ChatCompletionsStreamResponseChunk, error)
}
