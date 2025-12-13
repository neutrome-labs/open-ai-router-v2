package openai

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/neutrome-labs/open-ai-router/src/drivers"
	"github.com/neutrome-labs/open-ai-router/src/formats"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/sse"
	"go.uber.org/zap"
)

// Logger for OpenAI driver - can be set by modules
var Logger *zap.Logger

// ChatCompletions implements chat completions for OpenAI-compatible APIs
type ChatCompletions struct{}

func (c *ChatCompletions) createRequest(p *services.ProviderImpl, req formats.ManagedRequest, r *http.Request, endpoint string) (*http.Request, error) {
	targetUrl := p.ParsedURL
	targetUrl.Path += endpoint

	body, err := req.ToJSON()
	if err != nil {
		return nil, err
	}

	targetHeader := r.Header.Clone()
	targetHeader.Del("Accept-Encoding")
	targetHeader.Set("Content-Type", "application/json")

	httpReq := &http.Request{
		Method: "POST",
		URL:    &targetUrl,
		Header: targetHeader,
		Body:   io.NopCloser(bytes.NewReader(body)),
	}
	httpReq = httpReq.WithContext(r.Context())

	authVal, err := p.Router.AuthManager.CollectTargetAuth("chat_completions", p, r, httpReq)
	if err != nil {
		return nil, err
	}
	if authVal != "" {
		httpReq.Header.Set("Authorization", "Bearer "+authVal)
	}

	return httpReq, nil
}

func (c *ChatCompletions) DoChatCompletions(p *services.ProviderImpl, req formats.ManagedRequest, r *http.Request) (*http.Response, formats.ManagedResponse, error) {
	if Logger != nil {
		Logger.Debug("DoChatCompletions starting",
			zap.String("provider", p.Name),
			zap.String("model", req.GetModel()),
			zap.String("base_url", p.ParsedURL.String()))
	}

	httpReq, err := c.createRequest(p, req, r, "/chat/completions")
	if err != nil {
		if Logger != nil {
			Logger.Error("DoChatCompletions createRequest failed", zap.Error(err))
		}
		return nil, nil, err
	}

	if Logger != nil {
		Logger.Debug("DoChatCompletions sending request", zap.String("url", httpReq.URL.String()))
	}

	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		if Logger != nil {
			Logger.Error("DoChatCompletions HTTP request failed", zap.Error(err))
		}
		return nil, nil, err
	}
	defer res.Body.Close()

	if Logger != nil {
		Logger.Debug("DoChatCompletions response received", zap.Int("status", res.StatusCode))
	}

	respData, _ := io.ReadAll(res.Body)

	if res.StatusCode != 200 {
		if Logger != nil {
			Logger.Error("DoChatCompletions non-200 response",
				zap.Int("status", res.StatusCode),
				zap.String("body", string(respData)))
		}
		return res, nil, fmt.Errorf("%s", string(respData))
	}

	result := &formats.OpenAIChatResponse{}
	if err := result.FromJSON(respData); err != nil {
		if Logger != nil {
			Logger.Error("DoChatCompletions failed to parse response", zap.Error(err))
		}
		return res, nil, err
	}

	return res, result, nil
}

func (c *ChatCompletions) DoChatCompletionsStream(p *services.ProviderImpl, req formats.ManagedRequest, r *http.Request) (*http.Response, chan drivers.ChatCompletionsStreamChunk, error) {
	if Logger != nil {
		Logger.Debug("DoChatCompletionsStream starting",
			zap.String("provider", p.Name),
			zap.String("model", req.GetModel()))
	}

	httpReq, err := c.createRequest(p, req, r, "/chat/completions")
	if err != nil {
		if Logger != nil {
			Logger.Error("DoChatCompletionsStream createRequest failed", zap.Error(err))
		}
		return nil, nil, err
	}

	if Logger != nil {
		Logger.Debug("DoChatCompletionsStream sending request", zap.String("url", httpReq.URL.String()))
	}

	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		if Logger != nil {
			Logger.Error("DoChatCompletionsStream HTTP request failed", zap.Error(err))
		}
		return nil, nil, err
	}

	if Logger != nil {
		Logger.Debug("DoChatCompletionsStream response received",
			zap.Int("status", res.StatusCode),
			zap.String("content_type", res.Header.Get("Content-Type")))
	}

	chunks := make(chan drivers.ChatCompletionsStreamChunk)

	go func() {
		defer close(chunks)
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			respData, _ := io.ReadAll(res.Body)
			if Logger != nil {
				Logger.Error("DoChatCompletionsStream non-200 response",
					zap.Int("status", res.StatusCode),
					zap.String("body", string(respData)))
			}
			chunks <- drivers.ChatCompletionsStreamChunk{
				RuntimeError: fmt.Errorf("%s - %s", res.Status, string(respData)),
			}
			return
		}

		ct := res.Header.Get("Content-Type")
		isSSE := strings.HasPrefix(strings.ToLower(ct), "text/event-stream")

		if !isSSE {
			respData, err := io.ReadAll(res.Body)
			if err != nil {
				chunks <- drivers.ChatCompletionsStreamChunk{RuntimeError: err}
				return
			}
			result := &formats.OpenAIChatResponse{}
			if err := result.FromJSON(respData); err != nil {
				chunks <- drivers.ChatCompletionsStreamChunk{RuntimeError: err}
				return
			}
			chunks <- drivers.ChatCompletionsStreamChunk{Data: result}
			return
		}

		reader := sse.NewDefaultReader(res.Body)
		for event := range reader.ReadEvents() {
			if event.Error != nil {
				chunks <- drivers.ChatCompletionsStreamChunk{RuntimeError: event.Error}
				return
			}
			if event.Done {
				return
			}
			if event.Data != nil {
				result := &formats.OpenAIChatResponse{}
				if id, ok := event.Data["id"].(string); ok {
					result.ID = id
				}
				if obj, ok := event.Data["object"].(string); ok {
					result.Object = obj
				}
				if model, ok := event.Data["model"].(string); ok {
					result.Model = model
				}
				if choices, ok := event.Data["choices"].([]interface{}); ok {
					for _, ch := range choices {
						if chMap, ok := ch.(map[string]interface{}); ok {
							choice := formats.Choice{}
							if idx, ok := chMap["index"].(float64); ok {
								choice.Index = int(idx)
							}
							if delta, ok := chMap["delta"].(map[string]interface{}); ok {
								msg := &formats.Message{}
								if role, ok := delta["role"].(string); ok {
									msg.Role = role
								}
								if content, ok := delta["content"].(string); ok {
									msg.Content = content
								}
								choice.Delta = msg
							}
							if fr, ok := chMap["finish_reason"].(string); ok {
								choice.FinishReason = fr
							}
							result.Choices = append(result.Choices, choice)
						}
					}
				}
				if usage, ok := event.Data["usage"].(map[string]interface{}); ok {
					result.Usage = &formats.Usage{}
					if pt, ok := usage["prompt_tokens"].(float64); ok {
						result.Usage.PromptTokens = int(pt)
					}
					if ct, ok := usage["completion_tokens"].(float64); ok {
						result.Usage.CompletionTokens = int(ct)
					}
					if tt, ok := usage["total_tokens"].(float64); ok {
						result.Usage.TotalTokens = int(tt)
					}
				}
				chunks <- drivers.ChatCompletionsStreamChunk{Data: result}
			}
		}
	}()

	return res, chunks, nil
}
