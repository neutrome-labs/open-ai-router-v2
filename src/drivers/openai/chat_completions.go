package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/neutrome-labs/open-ai-router-v2/src/commands"
	"github.com/neutrome-labs/open-ai-router-v2/src/services"
	"github.com/neutrome-labs/open-ai-router-v2/src/sse"
)

type ChatCompletions struct {
}

func (c *ChatCompletions) createChatCompletionsRequest(p *services.ProviderImpl, body []byte, r *http.Request) (*http.Request, error) {
	targetUrl := p.ParsedURL
	targetUrl.Path += "/chat/completions"

	targetHeader := r.Header.Clone()
	targetHeader.Del("Accept-Encoding")
	targetHeader.Set("Content-Type", "application/json")

	req := &http.Request{
		Method: "POST",
		URL:    &targetUrl,
		Header: targetHeader,
		Body:   io.NopCloser(bytes.NewReader(body)),
	}
	// Propagate the original request context for cancellation/timeouts
	req = req.WithContext(r.Context())

	authVal, err := p.Router.AuthManager.CollectTargetAuth("chat_completions", p, r, req)
	if err != nil {
		return nil, err
	}
	if authVal != "" {
		req.Header.Set("Authorization", "Bearer "+authVal)
	}

	return req, nil
}

func (c *ChatCompletions) DoChatCompletions(p *services.ProviderImpl, body []byte, r *http.Request) (*http.Response, map[string]any, error) {
	req, err := c.createChatCompletionsRequest(p, body, r)
	if err != nil {
		return nil, nil, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()

	var respData []byte
	respData, _ = io.ReadAll(res.Body)

	var result map[string]any
	switch res.StatusCode {
	case 200:
		err = json.Unmarshal(respData, &result)
	// todo: retry on 4xx without Bearer eg. strings.Replace(authVal, "Bearer ", "", 1)
	default:
		err = fmt.Errorf("%s", string(respData))
	}

	return res, result, err
}

func (c *ChatCompletions) DoChatCompletionsStream(p *services.ProviderImpl, body []byte, r *http.Request) (*http.Response, chan commands.ChatCompletionsStreamResponseChunk, error) {
	req, err := c.createChatCompletionsRequest(p, body, r)
	if err != nil {
		return nil, nil, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}

	chunks := make(chan commands.ChatCompletionsStreamResponseChunk)

	go func() {
		defer close(chunks)
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			// Non-200 responses are not streamed; read the body once and emit as error
			respData, _ := io.ReadAll(res.Body)
			chunks <- commands.ChatCompletionsStreamResponseChunk{
				RuntimeError: fmt.Errorf("%s - %s", res.Status, string(respData)),
			}
			return
		}

		// Prefer SSE parsing when content-type indicates event-stream
		ct := res.Header.Get("Content-Type")
		isSSE := strings.HasPrefix(strings.ToLower(ct), "text/event-stream")

		if !isSSE {
			// Fallback: not an SSE response; read once and try to parse as a single chunk
			respData, err := io.ReadAll(res.Body)
			if err != nil {
				chunks <- commands.ChatCompletionsStreamResponseChunk{RuntimeError: err}
				return
			}
			var respJson map[string]any
			if err := json.Unmarshal(respData, &respJson); err != nil {
				chunks <- commands.ChatCompletionsStreamResponseChunk{RuntimeError: err}
				return
			}
			chunks <- commands.ChatCompletionsStreamResponseChunk{Data: respJson}
			return
		}

		// Use SSE reader to parse the event stream
		reader := sse.NewDefaultReader(res.Body)
		for event := range reader.ReadEvents() {
			if event.Error != nil {
				chunks <- commands.ChatCompletionsStreamResponseChunk{RuntimeError: event.Error}
				return
			}
			if event.Done {
				return
			}
			if event.Data != nil {
				chunks <- commands.ChatCompletionsStreamResponseChunk{Data: event.Data}
			}
		}
	}()

	return res, chunks, nil
}
