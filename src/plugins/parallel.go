package plugins

import (
	"net/http"
	"strings"
	"sync"

	"github.com/neutrome-labs/open-ai-router/src/formats"
	"go.uber.org/zap"
)

// Parallel calls multiple models in parallel and merges their responses.
// The model field should contain pipe-separated models: "gpt-4|claude-3|gemini"
// All models are called in parallel using InvokeHandlerCapture, and their
// responses are merged by combining choices from all successful responses.
//
// Note: Streaming is disabled for parallel requests - all responses are collected
// and merged before being returned.
type Parallel struct{}

func (p *Parallel) Name() string { return "parallel" }

// RecursiveHandler implements parallel model calls by invoking the handler for each model
// in parallel and merging the responses.
func (p *Parallel) RecursiveHandler(
	params string,
	invoker HandlerInvoker,
	w http.ResponseWriter,
	r *http.Request,
	req formats.ManagedRequest,
) (handled bool, err error) {
	model := req.GetModel()

	// Parse pipe-separated models for parallel execution
	models, pluginSuffix := parseParallelModelList(model)
	if len(models) <= 1 {
		// Single model - let normal flow handle it
		return false, nil
	}

	if Logger != nil {
		Logger.Debug("parallel plugin starting parallel calls",
			zap.Strings("models", models),
			zap.String("plugin_suffix", pluginSuffix))
	}

	// Call all models in parallel
	type result struct {
		index int
		model string
		resp  formats.ManagedResponse
		err   error
	}

	results := make(chan result, len(models))
	var wg sync.WaitGroup

	for i, m := range models {
		i, m := i, m // capture for goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			cloned := req.Clone()
			// Force non-streaming for parallel
			if chatReq, ok := cloned.(*formats.OpenAIChatRequest); ok {
				chatReq.Stream = false
				chatReq.StreamOptions = nil
			}
			cloned.SetModel(m)
			resp, err := invoker.InvokeHandlerCapture(r, cloned)
			results <- result{index: i, model: m, resp: resp, err: err}
		}()
	}

	// Wait for all results
	wg.Wait()
	close(results)

	// Collect results in order
	responses := make([]formats.ManagedResponse, len(models))
	var errors []error

	for res := range results {
		if res.err != nil {
			if Logger != nil {
				Logger.Warn("parallel plugin model failed",
					zap.String("model", res.model),
					zap.Error(res.err))
			}
			errors = append(errors, res.err)
			continue
		}
		responses[res.index] = res.resp
	}

	// Find first successful response to use as base
	var firstResponse formats.ManagedResponse
	var successCount int
	for _, resp := range responses {
		if resp != nil {
			if firstResponse == nil {
				firstResponse = resp
			}
			successCount++
		}
	}

	if successCount == 0 {
		// All failed
		if Logger != nil {
			Logger.Error("parallel plugin all models failed",
				zap.Strings("models", models))
		}
		if len(errors) > 0 {
			return true, errors[0]
		}
		return true, nil
	}

	// Merge responses - use first response as base and add choices from others
	merged := mergeParallelResponses(firstResponse, responses, models)

	if Logger != nil {
		Logger.Debug("parallel plugin merged responses",
			zap.Int("total_responses", successCount),
			zap.Int("total_choices", len(merged.GetChoices())))
	}

	// Write merged response
	data, err := merged.ToJSON()
	if err != nil {
		return true, err
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Parallel-Models", strings.Join(models, "|"))
	_, err = w.Write(data)
	return true, err
}

// mergeParallelResponses combines multiple responses into one by merging choices.
// The first response is used as the base, and choices from other responses are appended
// with re-indexed choice indices.
func mergeParallelResponses(primary formats.ManagedResponse, all []formats.ManagedResponse, models []string) formats.ManagedResponse {
	if primary == nil && len(all) > 0 {
		for _, r := range all {
			if r != nil {
				primary = r
				break
			}
		}
	}
	if primary == nil {
		return nil
	}

	// Type assert to OpenAIChatResponse for merging
	chatResp, ok := primary.(*formats.OpenAIChatResponse)
	if !ok {
		return primary
	}

	// Collect all choices with re-indexed indices
	var allChoices []formats.Choice
	idx := 0

	for _, resp := range all {
		if resp == nil {
			continue
		}
		for _, choice := range resp.GetChoices() {
			newChoice := choice
			newChoice.Index = idx
			allChoices = append(allChoices, newChoice)
			idx++
		}
	}

	// Create merged response
	merged := &formats.OpenAIChatResponse{
		ID:                chatResp.ID,
		Object:            chatResp.Object,
		Created:           chatResp.Created,
		Model:             strings.Join(models, "|"),
		SystemFingerprint: chatResp.SystemFingerprint,
		Choices:           allChoices,
		ServiceTier:       chatResp.ServiceTier,
	}

	// Merge usage if available
	var totalUsage *formats.Usage
	for _, resp := range all {
		if resp == nil {
			continue
		}
		usage := resp.GetUsage()
		if usage != nil {
			if totalUsage == nil {
				totalUsage = &formats.Usage{}
			}
			totalUsage.PromptTokens += usage.PromptTokens
			totalUsage.CompletionTokens += usage.CompletionTokens
			totalUsage.TotalTokens += usage.TotalTokens
			totalUsage.CacheReadInputTokens += usage.CacheReadInputTokens
			totalUsage.CacheCreationInputTokens += usage.CacheCreationInputTokens
		}
	}
	merged.Usage = totalUsage

	return merged
}

// parseParallelModelList parses a pipe-separated model string into a list.
// Returns the list of models (without plugin suffix) and the plugin suffix separately.
// This ensures recursive calls don't re-parse the models.
func parseParallelModelList(model string) ([]string, string) {
	// First, extract plugin suffix if present
	plusIdx := strings.IndexByte(model, '+')
	modelPart := model
	pluginSuffix := ""
	if plusIdx >= 0 {
		modelPart = model[:plusIdx]
		pluginSuffix = model[plusIdx:]
	}

	if !strings.Contains(modelPart, "|") {
		return []string{model}, pluginSuffix
	}

	parts := strings.Split(modelPart, "|")
	var models []string
	for _, m := range parts {
		m = strings.TrimSpace(m)
		if m != "" {
			// Return individual models WITHOUT plugin suffix
			// (plugin suffix is already parsed and plugins are in the chain)
			models = append(models, m)
		}
	}

	return models, pluginSuffix
}

// ParseParallelModelList is exported for other packages that need to detect pipe-separated models.
func ParseParallelModelList(model string) []string {
	models, _ := parseParallelModelList(model)
	return models
}
