package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/neutrome-labs/open-ai-router-v2/src/commands"
	"github.com/neutrome-labs/open-ai-router-v2/src/service"
)

type ChatCompletions struct {
}

func (c *ChatCompletions) createChatCompletionsRequest(p *service.ProviderImpl, data map[string]any, r *http.Request) (*http.Request, error) {
	targetUrl := p.ParsedURL
	targetUrl.Path += "/chat/completions"

	targetHeader := r.Header.Clone()
	targetHeader.Del("Accept-Encoding")
	targetHeader.Set("Content-Type", "application/json")
	if data != nil && data["stream"] == true {
		// Hint the origin we expect an event-stream when streaming
		targetHeader.Set("Accept", "text/event-stream")
	}

	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

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

func (c *ChatCompletions) DoChatCompletions(p *service.ProviderImpl, data map[string]any, r *http.Request) (*http.Response, map[string]any, error) {
	req, err := c.createChatCompletionsRequest(p, data, r)
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

func (c *ChatCompletions) DoChatCompletionsStream(p *service.ProviderImpl, data map[string]any, r *http.Request) (*http.Response, chan commands.ChatCompletionsStreamResponseChunk, error) {
	req, err := c.createChatCompletionsRequest(p, data, r)
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

		// Prefer SSE parsing when content-type indicates event-stream or when request asked for streaming
		ct := res.Header.Get("Content-Type")
		isSSE := strings.HasPrefix(strings.ToLower(ct), "text/event-stream") || data["stream"] == true

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

		// SSE parser: accumulate data: lines until a blank line, then emit one event
		scanner := bufio.NewScanner(res.Body)
		// Increase max token size to handle larger payloads safely (up to 1MB per event)
		buf := make([]byte, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		var eventData bytes.Buffer
		emitEvent := func(payload string) bool {
			// returns true to continue, false to stop
			if payload == "" {
				return true
			}
			if payload == "[DONE]" {
				return false
			}
			var respJson map[string]any
			if err := json.Unmarshal([]byte(payload), &respJson); err != nil {
				chunks <- commands.ChatCompletionsStreamResponseChunk{RuntimeError: err}
				return false
			}
			chunks <- commands.ChatCompletionsStreamResponseChunk{Data: respJson}
			return true
		}

		for scanner.Scan() {
			line := scanner.Text()
			// Trim trailing CR just in case (Windows style newlines over proxies)
			line = strings.TrimRight(line, "\r")
			if strings.HasPrefix(line, ":") {
				// comment/heartbeat line per SSE spec; ignore
				continue
			}
			if strings.HasPrefix(line, "data:") {
				// strip field name and optional space
				val := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if eventData.Len() > 0 {
					eventData.WriteByte('\n')
				}
				eventData.WriteString(val)
				continue
			}
			if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "id:") || strings.HasPrefix(line, "retry:") {
				// not used by OpenAI; ignore but keep collecting until blank line
				continue
			}
			if strings.TrimSpace(line) == "" {
				// blank line indicates end of an event
				if eventData.Len() > 0 {
					payload := eventData.String()
					eventData.Reset()
					if ok := emitEvent(payload); !ok {
						return
					}
				}
				continue
			}
			// Unknown line content; ignore
		}

		// Flush last event if stream ended without trailing blank line
		if eventData.Len() > 0 {
			if ok := emitEvent(eventData.String()); !ok {
				return
			}
		}

		if err := scanner.Err(); err != nil && err != io.EOF {
			chunks <- commands.ChatCompletionsStreamResponseChunk{RuntimeError: err}
			return
		}
	}()

	return res, chunks, nil
}
