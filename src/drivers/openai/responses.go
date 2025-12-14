package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/neutrome-labs/open-ai-router/src/drivers"
	"github.com/neutrome-labs/open-ai-router/src/formats"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/sse"
)

// Responses implements the OpenAI Responses API
type Responses struct{}

func (c *Responses) createRequest(p *services.ProviderImpl, req formats.ManagedRequest, r *http.Request) (*http.Request, error) {
	targetUrl := p.ParsedURL
	targetUrl.Path += "/responses"

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

	authVal, err := p.Router.AuthManager.CollectTargetAuth("responses", p, r, httpReq)
	if err != nil {
		return nil, err
	}
	if authVal != "" {
		httpReq.Header.Set("Authorization", "Bearer "+authVal)
	}

	return httpReq, nil
}

func (c *Responses) DoInference(p *services.ProviderImpl, req formats.ManagedRequest, r *http.Request) (*http.Response, formats.ManagedResponse, error) {
	httpReq, err := c.createRequest(p, req, r)
	if err != nil {
		return nil, nil, err
	}

	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()

	respData, _ := io.ReadAll(res.Body)

	if res.StatusCode != 200 {
		return res, nil, fmt.Errorf("%s", string(respData))
	}

	result := &formats.OpenAIResponsesResponse{}
	if err := result.FromJSON(respData); err != nil {
		return res, nil, err
	}

	return res, result, nil
}

func (c *Responses) DoInferenceStream(p *services.ProviderImpl, req formats.ManagedRequest, r *http.Request) (*http.Response, chan drivers.InferenceStreamChunk, error) {
	httpReq, err := c.createRequest(p, req, r)
	if err != nil {
		return nil, nil, err
	}

	res, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}

	chunks := make(chan drivers.InferenceStreamChunk)

	go func() {
		defer close(chunks)
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			respData, _ := io.ReadAll(res.Body)
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
			result := &formats.OpenAIResponsesResponse{}
			if err := result.FromJSON(respData); err != nil {
				chunks <- drivers.InferenceStreamChunk{RuntimeError: err}
				return
			}
			chunks <- drivers.InferenceStreamChunk{Data: result}
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
			if event.Data != nil {
				result := &formats.OpenAIResponsesResponse{}
				if err := result.FromJSON(mustMarshal(event.Data)); err == nil {
					chunks <- drivers.InferenceStreamChunk{Data: result}
				}
			}
		}
	}()

	return res, chunks, nil
}

func mustMarshal(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}
