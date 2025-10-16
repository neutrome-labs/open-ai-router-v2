package cfai

import (
	"fmt"
	"net/http"

	"github.com/neutrome-labs/open-ai-router-v2/src/formats"
	"github.com/neutrome-labs/open-ai-router-v2/src/service"
)

type ChatCompletions struct {
}

func (c *ChatCompletions) DoChatCompletions(p *service.ProviderImpl, data *formats.ChatCompletionsRequest, r *http.Request) (formats.ChatCompletionsResponse, error) {
	return formats.ChatCompletionsResponse{}, fmt.Errorf("not implemented")
}

func (c *ChatCompletions) DoChatCompletionsStream(p *service.ProviderImpl, data *formats.ChatCompletionsRequest, r *http.Request, w http.ResponseWriter) error {
	return fmt.Errorf("not implemented")
}
