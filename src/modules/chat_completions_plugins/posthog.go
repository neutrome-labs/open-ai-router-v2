package chatcompletionsplugins

import (
	"context"
	"net/http"
	"time"

	"github.com/neutrome-labs/open-ai-router-v2/src/formats"
	"github.com/neutrome-labs/open-ai-router-v2/src/service"
)

type Posthog struct{}

func (f *Posthog) Before(params string, p *service.ProviderImpl, r *http.Request, req *formats.ChatCompletionsRequest) error {
	ctx := context.WithValue(r.Context(), "posthog_time_start", time.Now())
	*r = *r.WithContext(ctx)
	return nil
}

func (f *Posthog) After(params string, p *service.ProviderImpl, r *http.Request, req *formats.ChatCompletionsRequest, hres *http.Response, res *formats.ChatCompletionsResponse) error {
	traceId := r.Context().Value("trace_id").(string)
	userId := r.Context().Value("user_id").(string)
	startTime := r.Context().Value("posthog_time_start").(time.Time)

	_ = service.FireObservabilityEvent(userId, "", "$ai_generation", map[string]any{
		"$ai_trace_id":       traceId,
		"$ai_model":          req.Model,
		"$ai_provider":       p.Name,
		"$ai_input":          req.Messages,
		"$ai_input_tokens":   res.Usage.PromptTokens,
		"$ai_output_choices": res.Choices,
		"$ai_output_tokens":  res.Usage.CompletionTokens,
		"$ai_latency":        time.Since(startTime).Seconds(),
		"$ai_http_status":    hres.StatusCode,
		"$ai_base_url":       p.ParsedURL.String(),
		"$ai_request_url":    r.URL.String(),
		"$ai_is_error":       false,
		// "$ai_temperature":   0.7,
		"$ai_stream": req.Stream,
		// "$ai_max_tokens":    500,
		// "$ai_tools": [{"type": "function", "function": {"name": "analyze_data", "description": "Analyzes data and provides insights", "parameters": {"type": "object", "properties": {"data_type": {"type": "string"}}}}}],
		// "$ai_cache_read_input_tokens": 50,
		// "$ai_span_name": "data_analysis_chat",
	})

	return nil
}
