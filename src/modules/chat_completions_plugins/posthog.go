package chatcompletionsplugins

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/neutrome-labs/open-ai-router-v2/src/services"
)

type Posthog struct{}

func (*Posthog) Before(params string, p *services.ProviderImpl, r *http.Request, body []byte) ([]byte, error) {
	ctx := context.WithValue(r.Context(), "posthog_time_start", time.Now())
	*r = *r.WithContext(ctx)
	return body, nil
}

func (*Posthog) After(params string, p *services.ProviderImpl, r *http.Request, body []byte, hres *http.Response, res map[string]any) (map[string]any, error) {
	if res != nil && res["object"] == "chat.completion.chunk" && res["usage"] == nil {
		return res, nil
	}

	traceId := r.Context().Value("trace_id").(string)
	startTime := r.Context().Value("posthog_time_start").(time.Time)
	userIdPtr := r.Context().Value("user_id")
	userId := ""
	if userIdPtr != nil {
		userId = userIdPtr.(string)
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return res, nil
	}

	isError := false
	if res == nil {
		res = map[string]any{}
		isError = true
	}

	usageData, ok := res["usage"]
	if !ok {
		usageData = map[string]any{}
	}

	_ = services.FireObservabilityEvent(userId, "", "$ai_generation", map[string]any{
		"$ai_trace_id":       traceId,
		"$ai_model":          req["model"],
		"$ai_provider":       p.Name,
		"$ai_input":          req["messages"],
		"$ai_input_tokens":   usageData.(map[string]any)["prompt_tokens"],
		"$ai_output_choices": res["choices"],
		"$ai_output_tokens":  usageData.(map[string]any)["completion_tokens"],
		"$ai_latency":        time.Since(startTime).Seconds(),
		"$ai_http_status":    hres.StatusCode,
		"$ai_base_url":       p.ParsedURL.String(),
		"$ai_request_url":    r.URL.String(),
		"$ai_is_error":       isError,
		"$ai_temperature":    req["temperature"],
		"$ai_stream":         req["stream"],
		"$ai_max_tokens":     req["max_tokens"],
		"$ai_tools":          req["tools"],
		// "$ai_cache_read_input_tokens": 50,
		// "$ai_span_name": "data_analysis_chat",
	})

	return res, nil
}
