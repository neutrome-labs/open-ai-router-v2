package openai

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/neutrome-labs/open-ai-router/src/drivers"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/sse"
	"go.uber.org/zap"
)

// Logger for OpenAI driver - can be set by modules
var Logger *zap.Logger

// ChatCompletions implements chat completions for OpenAI-compatible APIs
type ChatCompletions struct{}

func (c *ChatCompletions) createRequest(p *services.ProviderService, reqBody []byte, r *http.Request, endpoint string) (*http.Request, error) {
	targetUrl := p.ParsedURL
	targetUrl.Path += endpoint

	targetHeader := r.Header.Clone()
	targetHeader.Del("Accept-Encoding")
	targetHeader.Set("Content-Type", "application/json")

	httpReq := &http.Request{
		Method:        "POST",
		URL:           &targetUrl,
		Header:        targetHeader,
		Body:          io.NopCloser(bytes.NewReader(reqBody)),
		ContentLength: int64(len(reqBody)),
	}
	httpReq = httpReq.WithContext(r.Context())

	authVal, err := p.Router.Auth.CollectTargetAuth("chat_completions", p, r, httpReq)
	if err != nil {
		return nil, err
	}
	if authVal != "" {
		httpReq.Header.Set("Authorization", "Bearer "+authVal)
	}

	return httpReq, nil
}

// DoInference implements InferenceCommand for OpenAI Chat Completions API
func (c *ChatCompletions) DoInference(p *services.ProviderService, reqBody []byte, r *http.Request) (*http.Response, []byte, error) {
	if Logger != nil {
		Logger.Debug("DoInference (chat_completions) starting",
			zap.String("provider", p.Name),
			// zap.String("model", req.GetModel()), todo
			zap.String("base_url", p.ParsedURL.String()))
	}

	httpReq, err := c.createRequest(p, reqBody, r, "/chat/completions")
	if err != nil {
		if Logger != nil {
			Logger.Error("DoInference (chat_completions) createRequest failed", zap.Error(err))
		}
		return nil, nil, err
	}

	if Logger != nil {
		Logger.Debug("DoInference (chat_completions) sending request", zap.String("url", httpReq.URL.String()))
	}

	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		if Logger != nil {
			Logger.Error("DoInference (chat_completions) HTTP request failed", zap.Error(err))
		}
		return nil, nil, err
	}
	defer res.Body.Close()

	if Logger != nil {
		Logger.Debug("DoInference (chat_completions) response received", zap.Int("status", res.StatusCode))
	}

	respData, _ := io.ReadAll(res.Body)

	if res.StatusCode != 200 {
		if Logger != nil {
			Logger.Error("DoInference (chat_completions) non-200 response",
				zap.Int("status", res.StatusCode),
				zap.String("body", string(respData)))
		}
		return res, nil, fmt.Errorf("%s", string(respData))
	}

	return res, respData, nil
}

// DoInferenceStream implements InferenceCommand for streaming OpenAI Chat Completions
func (c *ChatCompletions) DoInferenceStream(p *services.ProviderService, reqBody []byte, r *http.Request) (*http.Response, chan drivers.InferenceStreamChunk, error) {
	if Logger != nil {
		Logger.Debug("DoInferenceStream (chat_completions) starting",
			zap.String("provider", p.Name))
		// zap.String("model", req.GetModel())) todo
	}

	httpReq, err := c.createRequest(p, reqBody, r, "/chat/completions")
	if err != nil {
		if Logger != nil {
			Logger.Error("DoInferenceStream (chat_completions) createRequest failed", zap.Error(err))
		}
		return nil, nil, err
	}

	if Logger != nil {
		Logger.Debug("DoInferenceStream (chat_completions) sending request", zap.String("url", httpReq.URL.String()))
	}

	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		if Logger != nil {
			Logger.Error("DoInferenceStream (chat_completions) HTTP request failed", zap.Error(err))
		}
		return nil, nil, err
	}

	if Logger != nil {
		Logger.Debug("DoInferenceStream (chat_completions) response received",
			zap.Int("status", res.StatusCode),
			zap.String("content_type", res.Header.Get("Content-Type")))
	}

	chunks := make(chan drivers.InferenceStreamChunk)

	go func() {
		defer close(chunks)
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			respData, _ := io.ReadAll(res.Body)
			if Logger != nil {
				Logger.Error("DoInferenceStream (chat_completions) non-200 response",
					zap.Int("status", res.StatusCode),
					zap.String("body", string(respData)))
			}
			chunks <- drivers.InferenceStreamChunk{
				RuntimeError: fmt.Errorf("%s - %s", res.Status, string(respData)),
			}
			return
		}

		ct := res.Header.Get("Content-Type")
		isSSE := strings.HasPrefix(strings.ToLower(ct), "text/event-stream")

		if !isSSE {
			respData, err := io.ReadAll(res.Body)
			if err != nil {
				chunks <- drivers.InferenceStreamChunk{RuntimeError: err}
				return
			}
			chunks <- drivers.InferenceStreamChunk{Data: respData}
			return
		}

		reader := sse.NewDefaultReader(res.Body)
		for event := range reader.ReadEvents() {
			if event.Error != nil {
				chunks <- drivers.InferenceStreamChunk{RuntimeError: event.Error}
				return
			}
			if event.Done {
				return
			}
			if event.RawData != nil {
				chunks <- drivers.InferenceStreamChunk{Data: event.RawData}
			}
		}
	}()

	return res, chunks, nil
}
