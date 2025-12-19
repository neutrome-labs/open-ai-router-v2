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
	"github.com/neutrome-labs/open-ai-router/src/styles"
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
	toolCalls    []styles.ChatCompletionsToolCall
	finishReason string
}

func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{
		choices: make(map[int]*choiceAccum),
	}
}

// accumulate merges a streaming chunk into the accumulator
func (sa *streamAccumulator) accumulate(chunk styles.PartialJSON) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	// Extract model if present
	model := styles.TryGetFromPartialJSON[string](chunk, "model")
	if model != "" {
		sa.model = model
	}

	// Extract choices
	choicesRaw, ok := chunk["choices"]
	if !ok {
		return
	}

	var choices []styles.ChatCompletionsChoice
	if err := json.Unmarshal(choicesRaw, &choices); err != nil {
		return
	}

	for _, choice := range choices {
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
			if content, ok := choice.Delta.Content.(string); ok {
				accum.content.WriteString(content)
			}

			// accumulate tool calls
			for _, tc := range choice.Delta.ToolCalls {
				tcIdx := tc.Index

				// extend slice if needed
				for len(accum.toolCalls) <= tcIdx {
					accum.toolCalls = append(accum.toolCalls, styles.ChatCompletionsToolCall{})
				}

				existing := &accum.toolCalls[tcIdx]
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Type != "" {
					existing.Type = tc.Type
				}
				if tc.Function != nil {
					if existing.Function == nil {
						existing.Function = &struct {
							Name      string `json:"name,omitempty"`
							Arguments string `json:"arguments,omitempty"`
						}{}
					}
					if tc.Function.Name != "" {
						existing.Function.Name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						existing.Function.Arguments += tc.Function.Arguments
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

func (p *Posthog) Before(params string, provider *services.ProviderService, r *http.Request, reqJson styles.PartialJSON) (styles.PartialJSON, error) {
	ctx := r.Context()
	ctx = context.WithValue(ctx, posthogTimeStartKey, time.Now())
	ctx = context.WithValue(ctx, posthogStreamAccumKey, newStreamAccumulator())
	*r = *r.WithContext(ctx)
	return reqJson, nil
}

func (p *Posthog) After(params string, provider *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, res *http.Response, resJson styles.PartialJSON) (styles.PartialJSON, error) {
	p.fireEvent(provider, r, reqJson, res, resJson, false, nil)
	return resJson, nil
}

func (p *Posthog) AfterChunk(params string, provider *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, hres *http.Response, chunk styles.PartialJSON) (styles.PartialJSON, error) {
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

func (p *Posthog) StreamEnd(params string, provider *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, res *http.Response, lastChunk styles.PartialJSON) error {
	p.fireEvent(provider, r, reqJson, res, lastChunk, true, nil)
	return nil
}

func (p *Posthog) OnError(params string, provider *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, res *http.Response, providerErr error) error {
	isStreaming := styles.TryGetFromPartialJSON[bool](reqJson, "stream")
	p.fireEvent(provider, r, reqJson, res, nil, isStreaming, providerErr)
	return nil
}

func (p *Posthog) fireEvent(provider *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, hres *http.Response, resJson styles.PartialJSON, isStreaming bool, providerErr error) {
	ctx := r.Context()
	userId, _ := ctx.Value(plugin.ContextUserID()).(string)

	// Extract common props
	props := p.extractCommonProps(provider, r, reqJson, hres, resJson, isStreaming, providerErr)

	// Extract chat completions specific props
	p.extractChatCompletionsProps(props, reqJson, resJson, isStreaming, ctx)

	_ = services.FireObservabilityEvent(userId, "", "$ai_generation", props)
}

func (p *Posthog) extractCommonProps(provider *services.ProviderService, r *http.Request, reqJson styles.PartialJSON, hres *http.Response, resJson styles.PartialJSON, isStreaming bool, providerErr error) map[string]any {
	ctx := r.Context()
	traceId, _ := ctx.Value(plugin.ContextTraceID()).(string)
	startTime, _ := ctx.Value(posthogTimeStartKey).(time.Time)

	var latency float64
	if !startTime.IsZero() {
		latency = time.Since(startTime).Seconds()
	}

	// Determine error state
	isError := false
	var errorMessage string
	httpStatus := 0

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
		isError = true
	}

	if resJson == nil && providerErr == nil && !isStreaming {
		isError = true
	}

	providerName := ""
	providerBaseURL := ""
	if provider != nil {
		providerName = provider.Name
		providerBaseURL = provider.ParsedURL.String()
	}

	model := styles.TryGetFromPartialJSON[string](reqJson, "model")
	stream := styles.TryGetFromPartialJSON[bool](reqJson, "stream")
	temp := styles.TryGetFromPartialJSON[*float64](reqJson, "temperature")
	maxTokens := styles.TryGetFromPartialJSON[int](reqJson, "max_tokens")

	props := map[string]any{
		"$ai_trace_id":    traceId,
		"$ai_model":       model,
		"$ai_provider":    providerName,
		"$ai_latency":     latency,
		"$ai_base_url":    providerBaseURL,
		"$ai_request_url": r.URL.String(),
		"$ai_is_error":    isError,
		"$ai_stream":      stream,
		"$ai_http_status": httpStatus,
	}

	if errorMessage != "" {
		props["$ai_error_message"] = errorMessage
	}

	if temp != nil {
		props["$ai_temperature"] = *temp
	}
	if maxTokens > 0 {
		props["$ai_max_tokens"] = maxTokens
	}

	// Usage
	if resJson != nil {
		usage := styles.TryGetFromPartialJSON[map[string]any](resJson, "usage")
		if usage != nil {
			if pt, ok := usage["prompt_tokens"].(float64); ok {
				props["$ai_input_tokens"] = int(pt)
			}
			if ct, ok := usage["completion_tokens"].(float64); ok {
				props["$ai_output_tokens"] = int(ct)
			}
		}
	}

	return props
}

func (p *Posthog) extractChatCompletionsProps(props map[string]any, reqJson styles.PartialJSON, resJson styles.PartialJSON, isStreaming bool, ctx context.Context) {
	if !services.PosthogIncludeContent {
		return
	}

	// Input
	messages := styles.TryGetFromPartialJSON[[]any](reqJson, "messages")
	if len(messages) > 0 {
		props["$ai_input"] = messages
	}
	tools := styles.TryGetFromPartialJSON[[]any](reqJson, "tools")
	if len(tools) > 0 {
		props["$ai_tools"] = tools
	}

	// Output
	if isStreaming {
		if accumVal := ctx.Value(posthogStreamAccumKey); accumVal != nil {
			if accum, ok := accumVal.(*streamAccumulator); ok {
				props["$ai_output_choices"] = accum.buildChoices()
			}
		}
	} else if resJson != nil {
		choices := styles.TryGetFromPartialJSON[[]any](resJson, "choices")
		if len(choices) > 0 {
			props["$ai_output_choices"] = choices
		}
	}
}

// Context keys
type contextKey string

const (
	posthogTimeStartKey   contextKey = "posthog_time_start"
	posthogStreamAccumKey contextKey = "posthog_stream_accum"
)

var (
	_ plugin.BeforePlugin = (*Posthog)(nil)
	_ plugin.AfterPlugin  = (*Posthog)(nil)
	_ plugin.ErrorPlugin  = (*Posthog)(nil)
)
