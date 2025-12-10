package chatcompletionsplugins

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/neutrome-labs/open-ai-router-v2/src/services"
)

// context key type to avoid collisions
type posthogCtxKey string

const (
	ctxKeyTimeStart   posthogCtxKey = "posthog_time_start"
	ctxKeyStreamAccum posthogCtxKey = "posthog_stream_accum"
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
func (sa *streamAccumulator) accumulate(chunk map[string]any) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	if model, ok := chunk["model"].(string); ok && model != "" {
		sa.model = model
	}
	if sysFP, ok := chunk["system_fingerprint"].(string); ok && sysFP != "" {
		sa.systemFP = sysFP
	}

	choices, ok := chunk["choices"].([]any)
	if !ok {
		return
	}

	for _, c := range choices {
		choice, ok := c.(map[string]any)
		if !ok {
			continue
		}

		idx := 0
		if idxVal, ok := choice["index"].(float64); ok {
			idx = int(idxVal)
		}

		accum, exists := sa.choices[idx]
		if !exists {
			accum = &choiceAccum{}
			sa.choices[idx] = accum
		}

		if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
			accum.finishReason = fr
		}

		delta, ok := choice["delta"].(map[string]any)
		if !ok {
			continue
		}

		if role, ok := delta["role"].(string); ok && role != "" {
			accum.role = role
		}
		if content, ok := delta["content"].(string); ok {
			accum.content.WriteString(content)
		}

		// accumulate tool calls
		if toolCalls, ok := delta["tool_calls"].([]any); ok {
			for _, tc := range toolCalls {
				tcMap, ok := tc.(map[string]any)
				if !ok {
					continue
				}
				tcIdx := 0
				if tcIdxVal, ok := tcMap["index"].(float64); ok {
					tcIdx = int(tcIdxVal)
				}

				// extend slice if needed
				for len(accum.toolCalls) <= tcIdx {
					accum.toolCalls = append(accum.toolCalls, map[string]any{})
				}

				existing := accum.toolCalls[tcIdx]
				if id, ok := tcMap["id"].(string); ok && id != "" {
					existing["id"] = id
				}
				if typ, ok := tcMap["type"].(string); ok && typ != "" {
					existing["type"] = typ
				}
				if fn, ok := tcMap["function"].(map[string]any); ok {
					existingFn, _ := existing["function"].(map[string]any)
					if existingFn == nil {
						existingFn = map[string]any{}
						existing["function"] = existingFn
					}
					if name, ok := fn["name"].(string); ok && name != "" {
						existingFn["name"] = name
					}
					if args, ok := fn["arguments"].(string); ok {
						prevArgs, _ := existingFn["arguments"].(string)
						existingFn["arguments"] = prevArgs + args
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

type Posthog struct{}

func (*Posthog) Before(params string, p *services.ProviderImpl, r *http.Request, body []byte) ([]byte, error) {
	ctx := r.Context()
	ctx = context.WithValue(ctx, ctxKeyTimeStart, time.Now())
	ctx = context.WithValue(ctx, ctxKeyStreamAccum, newStreamAccumulator())
	*r = *r.WithContext(ctx)
	return body, nil
}

func (*Posthog) After(params string, p *services.ProviderImpl, r *http.Request, body []byte, hres *http.Response, res map[string]any) (map[string]any, error) {
	ctx := r.Context()

	// Get or create stream accumulator
	accumVal := ctx.Value(ctxKeyStreamAccum)
	accum, _ := accumVal.(*streamAccumulator)

	isStreamingChunk := res != nil && res["object"] == "chat.completion.chunk"
	hasUsage := res != nil && res["usage"] != nil

	// For streaming chunks without usage, accumulate and return
	if isStreamingChunk {
		if accum != nil {
			accum.accumulate(res)
		}
		// Only fire event on final chunk (with usage) or if no usage tracking
		if !hasUsage {
			return res, nil
		}
	}

	// Extract context values safely
	traceId := ""
	if v := ctx.Value("trace_id"); v != nil {
		traceId, _ = v.(string)
	}

	var startTime time.Time
	if v := ctx.Value(ctxKeyTimeStart); v != nil {
		startTime, _ = v.(time.Time)
	} else {
		startTime = time.Now()
	}

	userId := ""
	if v := ctx.Value("user_id"); v != nil {
		userId, _ = v.(string)
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return res, nil
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
		httpStatus = 0
	}

	if res == nil {
		isError = true
		res = map[string]any{}
	} else if errVal, ok := res["error"]; ok {
		isError = true
		// Extract error message if available
		if errMap, ok := errVal.(map[string]any); ok {
			if msg, ok := errMap["message"].(string); ok {
				errorMessage = msg
			}
		} else if errStr, ok := errVal.(string); ok {
			errorMessage = errStr
		}
	}

	usageData, _ := res["usage"].(map[string]any)
	if usageData == nil {
		usageData = map[string]any{}
	}

	// Build output choices - use accumulated data for streaming
	var outputChoices any
	if isStreamingChunk && accum != nil {
		outputChoices = accum.buildChoices()
	} else {
		outputChoices = res["choices"]
	}

	// Build event properties
	event := map[string]any{
		"$ai_trace_id":      traceId,
		"$ai_model":         req["model"],
		"$ai_provider":      p.Name,
		"$ai_input_tokens":  usageData["prompt_tokens"],
		"$ai_output_tokens": usageData["completion_tokens"],
		"$ai_latency":       time.Since(startTime).Seconds(),
		"$ai_http_status":   httpStatus,
		"$ai_base_url":      p.ParsedURL.String(),
		"$ai_request_url":   r.URL.String(),
		"$ai_is_error":      isError,
		"$ai_temperature":   req["temperature"],
		"$ai_stream":        req["stream"],
		"$ai_max_tokens":    req["max_tokens"],
	}

	// Add error message if present
	if errorMessage != "" {
		event["$ai_error_message"] = errorMessage
	}

	// Include content-related fields only if enabled via env
	if services.PosthogIncludeContent {
		event["$ai_input"] = req["messages"]
		event["$ai_output_choices"] = outputChoices
		event["$ai_tools"] = req["tools"]
	}

	_ = services.FireObservabilityEvent(userId, "", "$ai_generation", event)

	return res, nil
}
