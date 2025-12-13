package plugins

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/neutrome-labs/open-ai-router/src/formats"
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

// accumulate merges a streaming chunk into the accumulator
func (sa *streamAccumulator) accumulate(chunk formats.ManagedResponse) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if model := chunk.GetModel(); model != "" {
		sa.model = model
	}

	choices := chunk.GetChoices()
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
			if len(choice.Delta.ToolCalls) > 0 {
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

func (p *Posthog) Before(params string, provider *services.ProviderImpl, r *http.Request, req formats.ManagedRequest) (formats.ManagedRequest, error) {
	ctx := r.Context()
	ctx = context.WithValue(ctx, posthogTimeStartKey, time.Now())
	ctx = context.WithValue(ctx, posthogStreamAccumKey, newStreamAccumulator())
	*r = *r.WithContext(ctx)
	return req, nil
}

func (p *Posthog) After(params string, provider *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, res formats.ManagedResponse) (formats.ManagedResponse, error) {
	p.fireEvent(provider, r, req, hres, res, false)
	return res, nil
}

func (p *Posthog) AfterChunk(params string, provider *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, chunk formats.ManagedResponse) (formats.ManagedResponse, error) {
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

func (p *Posthog) StreamEnd(params string, provider *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, lastChunk formats.ManagedResponse) error {
	p.fireEvent(provider, r, req, hres, lastChunk, true)
	return nil
}

func (p *Posthog) fireEvent(provider *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, res formats.ManagedResponse, isStreaming bool) {
	ctx := r.Context()
	traceId, _ := ctx.Value(traceIDKey).(string)
	startTime, _ := ctx.Value(posthogTimeStartKey).(time.Time)
	userId, _ := ctx.Value(userIDKey).(string)

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

	if hres != nil {
		httpStatus = hres.StatusCode
		if httpStatus >= 400 {
			isError = true
		}
	} else {
		isError = true
	}

	if res == nil {
		isError = true
	}

	// Extract provider info safely
	providerName := ""
	providerBaseURL := ""
	if provider != nil {
		providerName = provider.Name
		providerBaseURL = provider.ParsedURL.String()
	}

	props := map[string]any{
		"$ai_trace_id":    traceId,
		"$ai_model":       req.GetModel(),
		"$ai_provider":    providerName,
		"$ai_latency":     latency,
		"$ai_base_url":    providerBaseURL,
		"$ai_request_url": r.URL.String(),
		"$ai_is_error":    isError,
		"$ai_stream":      req.IsStreaming(),
		"$ai_http_status": httpStatus,
	}

	if errorMessage != "" {
		props["$ai_error_message"] = errorMessage
	}

	if res != nil {
		if usage := res.GetUsage(); usage != nil {
			props["$ai_input_tokens"] = usage.PromptTokens
			props["$ai_output_tokens"] = usage.CompletionTokens
		}
	}

	// Handle OpenAI-specific fields
	if openaiReq, ok := req.(*formats.OpenAIChatRequest); ok {
		if openaiReq.Temperature != nil {
			props["$ai_temperature"] = *openaiReq.Temperature
		}
		if openaiReq.MaxTokens > 0 {
			props["$ai_max_tokens"] = openaiReq.MaxTokens
		}

		// Include content if enabled
		if services.PosthogIncludeContent {
			props["$ai_input"] = openaiReq.Messages
			if len(openaiReq.Tools) > 0 {
				props["$ai_tools"] = openaiReq.Tools
			}
		}
	}

	// Build output choices - use accumulated data for streaming
	if services.PosthogIncludeContent {
		if isStreaming && accum != nil {
			props["$ai_output_choices"] = accum.buildChoices()
		} else if res != nil {
			props["$ai_output_choices"] = res.GetChoices()
		}
	}

	_ = services.FireObservabilityEvent(userId, "", "$ai_generation", props)
}

// Context keys
type contextKey string

const (
	posthogTimeStartKey   contextKey = "posthog_time_start"
	posthogStreamAccumKey contextKey = "posthog_stream_accum"
	traceIDKey            contextKey = "trace_id"
	userIDKey             contextKey = "user_id"
	keyIDKey              contextKey = "key_id"
)

// ContextTraceID returns the trace ID context key
func ContextTraceID() contextKey { return traceIDKey }

// ContextUserID returns the user ID context key
func ContextUserID() contextKey { return userIDKey }

// ContextKeyID returns the key ID context key
func ContextKeyID() contextKey { return keyIDKey }
