package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/neutrome-labs/open-ai-router/src/plugin"
	"github.com/neutrome-labs/open-ai-router/src/services"
)

// streamAccumulator holds accumulated content from streaming chunks
type streamAccumulator struct {
	mu       sync.Mutex
	choices  map[int]*choiceAccum // indexed by choice index
	model    string
	systemFP string
}

type choiceAccum struct {
	role         string
	content      strings.Builder
	toolCalls    []map[string]any
	finishReason string
}

func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{
		choices: make(map[int]*choiceAccum),
	}
}

// accumulate merges a streaming chunk into the accumulator (works with raw JSON)
func (sa *streamAccumulator) accumulate(chunk json.RawMessage) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	var data struct {
		Model   string `json:"model"`
		Choices []struct {
			Index        int    `json:"index"`
			FinishReason string `json:"finish_reason"`
			Delta        *struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function *struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(chunk, &data); err != nil {
		return
	}

	if data.Model != "" {
		sa.model = data.Model
	}

	for _, choice := range data.Choices {
		idx := choice.Index

		accum, exists := sa.choices[idx]
		if !exists {
			accum = &choiceAccum{}
			sa.choices[idx] = accum
		}

		if choice.FinishReason != "" {
			accum.finishReason = choice.FinishReason
		}

		if choice.Delta != nil {
			if choice.Delta.Role != "" {
				accum.role = choice.Delta.Role
			}
			accum.content.WriteString(choice.Delta.Content)

			// accumulate tool calls
			for _, tc := range choice.Delta.ToolCalls {
				tcIdx := tc.Index

				// extend slice if needed
				for len(accum.toolCalls) <= tcIdx {
					accum.toolCalls = append(accum.toolCalls, map[string]any{})
				}

				existing := accum.toolCalls[tcIdx]
				if tc.ID != "" {
					existing["id"] = tc.ID
				}
				if tc.Type != "" {
					existing["type"] = tc.Type
				}
				if tc.Function != nil {
					existingFn, _ := existing["function"].(map[string]any)
					if existingFn == nil {
						existingFn = map[string]any{}
						existing["function"] = existingFn
					}
					if tc.Function.Name != "" {
						existingFn["name"] = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						prevArgs, _ := existingFn["arguments"].(string)
						existingFn["arguments"] = prevArgs + tc.Function.Arguments
					}
				}
			}
		}
	}
}

// buildChoices constructs the final choices array for the event
func (sa *streamAccumulator) buildChoices() []map[string]any {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	result := make([]map[string]any, 0, len(sa.choices))
	for idx := 0; idx < len(sa.choices); idx++ {
		accum, ok := sa.choices[idx]
		if !ok {
			continue
		}

		message := map[string]any{
			"role":    accum.role,
			"content": accum.content.String(),
		}
		if len(accum.toolCalls) > 0 {
			message["tool_calls"] = accum.toolCalls
		}

		result = append(result, map[string]any{
			"index":         idx,
			"message":       message,
			"finish_reason": accum.finishReason,
		})
	}
	return result
}

// Posthog provides observability via PostHog
type Posthog struct{}

func (p *Posthog) Name() string { return "posthog" }

func (p *Posthog) Before(params string, provider *services.ProviderService, r *http.Request, req json.RawMessage) (json.RawMessage, error) {
	ctx := r.Context()
	ctx = context.WithValue(ctx, posthogTimeStartKey, time.Now())
	ctx = context.WithValue(ctx, posthogStreamAccumKey, newStreamAccumulator())
	*r = *r.WithContext(ctx)
	return req, nil
}

func (p *Posthog) After(params string, provider *services.ProviderService, r *http.Request, req json.RawMessage, hres *http.Response, res json.RawMessage) (json.RawMessage, error) {
	p.fireEvent(provider, r, req, hres, res, false, nil)
	return res, nil
}

func (p *Posthog) AfterChunk(params string, provider *services.ProviderService, r *http.Request, req json.RawMessage, hres *http.Response, chunk json.RawMessage) (json.RawMessage, error) {
	// Accumulate chunk content for final event
	ctx := r.Context()
	if accumVal := ctx.Value(posthogStreamAccumKey); accumVal != nil {
		if accum, ok := accumVal.(*streamAccumulator); ok {
			accum.accumulate(chunk)
		}
	}
	// Don't fire events for intermediate chunks
	return chunk, nil
}

func (p *Posthog) StreamEnd(params string, provider *services.ProviderService, r *http.Request, req json.RawMessage, hres *http.Response, lastChunk json.RawMessage) error {
	p.fireEvent(provider, r, req, hres, lastChunk, true, nil)
	return nil
}

func (p *Posthog) OnError(params string, provider *services.ProviderService, r *http.Request, req json.RawMessage, hres *http.Response, providerErr error) error {
	isStreaming := getStreamFromRaw(req)
	p.fireEvent(provider, r, req, hres, nil, isStreaming, providerErr)
	return nil
}

// getStreamFromRaw extracts the stream field from raw JSON request
func getStreamFromRaw(req json.RawMessage) bool {
	var data struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(req, &data); err != nil {
		return false
	}
	return data.Stream
}

func (p *Posthog) fireEvent(provider *services.ProviderService, r *http.Request, req json.RawMessage, hres *http.Response, res json.RawMessage, isStreaming bool, providerErr error) {
	ctx := r.Context()
	traceId, _ := ctx.Value(plugin.ContextTraceID()).(string)
	startTime, _ := ctx.Value(posthogTimeStartKey).(time.Time)
	userId, _ := ctx.Value(plugin.ContextUserID()).(string)

	// Get stream accumulator
	var accum *streamAccumulator
	if accumVal := ctx.Value(posthogStreamAccumKey); accumVal != nil {
		accum, _ = accumVal.(*streamAccumulator)
	}

	var latency float64
	if !startTime.IsZero() {
		latency = time.Since(startTime).Seconds()
	}

	// Determine error state
	isError := false
	var errorMessage string
	httpStatus := 0

	// Check for explicit provider error first
	if providerErr != nil {
		isError = true
		errorMessage = providerErr.Error()
	}

	if hres != nil {
		httpStatus = hres.StatusCode
		if httpStatus >= 400 {
			isError = true
		}
	} else if providerErr != nil {
		// No response but we have an error - likely network/connection failure
		isError = true
	}

	if res == nil && providerErr == nil {
		// No response and no explicit error - something unexpected happened
		isError = true
	}

	// Extract provider info safely
	providerName := ""
	providerBaseURL := ""
	if provider != nil {
		providerName = provider.Name
		providerBaseURL = provider.ParsedURL.String()
	}

	// Extract request fields
	var reqData struct {
		Model       string          `json:"model"`
		Stream      bool            `json:"stream"`
		Temperature *float64        `json:"temperature"`
		MaxTokens   int             `json:"max_tokens"`
		Messages    json.RawMessage `json:"messages"`
		Tools       json.RawMessage `json:"tools"`
	}
	_ = json.Unmarshal(req, &reqData)

	props := map[string]any{
		"$ai_trace_id":    traceId,
		"$ai_model":       reqData.Model,
		"$ai_provider":    providerName,
		"$ai_latency":     latency,
		"$ai_base_url":    providerBaseURL,
		"$ai_request_url": r.URL.String(),
		"$ai_is_error":    isError,
		"$ai_stream":      reqData.Stream,
		"$ai_http_status": httpStatus,
	}

	if errorMessage != "" {
		props["$ai_error_message"] = errorMessage
	}

	// Extract usage from response
	if res != nil {
		var resData struct {
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
			Choices json.RawMessage `json:"choices"`
		}
		if err := json.Unmarshal(res, &resData); err == nil {
			if resData.Usage != nil {
				props["$ai_input_tokens"] = resData.Usage.PromptTokens
				props["$ai_output_tokens"] = resData.Usage.CompletionTokens
			}
		}
	}

	// Extract optional fields
	if reqData.Temperature != nil {
		props["$ai_temperature"] = *reqData.Temperature
	}
	if reqData.MaxTokens > 0 {
		props["$ai_max_tokens"] = reqData.MaxTokens
	}

	// Include content if enabled
	if services.PosthogIncludeContent {
		if reqData.Messages != nil {
			var messages []any
			if err := json.Unmarshal(reqData.Messages, &messages); err == nil {
				props["$ai_input"] = messages
			}
		}
		if reqData.Tools != nil {
			var tools []any
			if err := json.Unmarshal(reqData.Tools, &tools); err == nil && len(tools) > 0 {
				props["$ai_tools"] = tools
			}
		}
	}

	// Build output choices - use accumulated data for streaming
	if services.PosthogIncludeContent {
		if isStreaming && accum != nil {
			props["$ai_output_choices"] = accum.buildChoices()
		} else if res != nil {
			var resData struct {
				Choices []any `json:"choices"`
			}
			if err := json.Unmarshal(res, &resData); err == nil {
				props["$ai_output_choices"] = resData.Choices
			}
		}
	}

	_ = services.FireObservabilityEvent(userId, "", "$ai_generation", props)
}

// Context keys
type contextKey string

const (
	posthogTimeStartKey   contextKey = "posthog_time_start"
	posthogStreamAccumKey contextKey = "posthog_stream_accum"
)
