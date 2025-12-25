package flow

import (
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/neutrome-labs/open-ai-router/src/plugin"
	"github.com/neutrome-labs/open-ai-router/src/plugins"
	"github.com/neutrome-labs/open-ai-router/src/styles"
	"go.uber.org/zap"
)

// Parallel handles pipe-separated models in the model field for parallel execution.
// Example: model="gpt-5+fuzz|opus-4.5:fuzz" will call both models in parallel
// and merge responses (combining choices from all responses).
//
// This plugin implements RecursiveHandlerPlugin to fan-out requests to multiple models
// and aggregate the results.
type Parallel struct{}

func (p *Parallel) Name() string { return "parallel" }

// RecursiveHandler implements parallel execution by calling multiple models concurrently.
func (p *Parallel) RecursiveHandler(
	params string,
	invoker plugin.HandlerInvoker,
	reqJson styles.PartialJSON,
	w http.ResponseWriter,
	r *http.Request,
) (handled bool, err error) {
	model := styles.TryGetFromPartialJSON[string](reqJson, "model")

	// Parse pipe-separated models (strip plugin suffix first)
	models, pluginSuffix := parseModelListForParallel(model)
	if len(models) <= 1 {
		// Single model or no models - let normal flow handle it
		return false, nil
	}

	// Check if streaming - parallel doesn't support streaming
	stream := styles.TryGetFromPartialJSON[bool](reqJson, "stream")
	if stream {
		plugins.Logger.Warn("parallel plugin: streaming not supported for parallel requests, using first model only",
			zap.Strings("models", models))
		return false, nil
	}

	plugins.Logger.Debug("parallel plugin starting fan-out",
		zap.Strings("models", models),
		zap.String("plugin_suffix", pluginSuffix))

	// Execute all models in parallel
	type result struct {
		model    string
		response styles.PartialJSON
		err      error
	}

	results := make(chan result, len(models))
	var wg sync.WaitGroup

	for _, currentModel := range models {
		wg.Add(1)
		go func(model string) {
			defer wg.Done()

			clonedJson, err := reqJson.CloneWith("model", model+pluginSuffix)
			if err != nil {
				plugins.Logger.Error("parallel plugin: failed to clone request JSON",
					zap.String("model", model),
					zap.Error(err))
				results <- result{model: model, err: err}
				return
			}

			reqData, err := clonedJson.Marshal()
			if err != nil {
				plugins.Logger.Error("parallel plugin: failed to marshal request JSON",
					zap.String("model", model),
					zap.Error(err))
				results <- result{model: model, err: err}
				return
			}

			// Clone request and set current model
			clonedReq := r.Clone(r.Context())
			clonedReq.Body = io.NopCloser(strings.NewReader(string(reqData)))

			// Invoke and capture the response
			respJson, err := invoker.InvokeHandlerCapture(clonedReq)
			if err != nil {
				plugins.Logger.Debug("parallel plugin: model call failed",
					zap.String("model", model),
					zap.Error(err))
				results <- result{model: model, err: err}
				return
			}

			plugins.Logger.Debug("parallel plugin: model call succeeded",
				zap.String("model", model))
			results <- result{model: model, response: respJson}
		}(currentModel)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var responses []styles.PartialJSON
	var errors []error
	for res := range results {
		if res.err != nil {
			errors = append(errors, res.err)
		} else {
			responses = append(responses, res.response)
		}
	}

	// If all failed, return the last error
	if len(responses) == 0 {
		plugins.Logger.Error("parallel plugin: all models failed",
			zap.Strings("models", models),
			zap.Int("error_count", len(errors)))
		if len(errors) > 0 {
			return true, errors[len(errors)-1]
		}
		return true, nil
	}

	// Merge responses - combine choices from all successful responses
	mergedResponse, err := mergeParallelResponses(responses)
	if err != nil {
		plugins.Logger.Error("parallel plugin: failed to merge responses",
			zap.Error(err))
		return true, err
	}

	// Write merged response
	w.Header().Set("Content-Type", "application/json")
	respData, err := mergedResponse.Marshal()
	if err != nil {
		return true, err
	}
	w.Write(respData)

	plugins.Logger.Debug("parallel plugin completed",
		zap.Int("successful_models", len(responses)),
		zap.Int("failed_models", len(errors)))

	return true, nil
}

// mergeParallelResponses merges multiple ChatCompletions responses into one.
// It combines choices from all responses, re-indexing them sequentially.
// Uses the first response as the base for ID, object, created, model, etc.
func mergeParallelResponses(responses []styles.PartialJSON) (styles.PartialJSON, error) {
	if len(responses) == 0 {
		return nil, nil
	}

	if len(responses) == 1 {
		return responses[0], nil
	}

	// Parse all responses to get choices
	var allChoices []styles.ChatCompletionsChoice
	var totalUsage styles.ChatCompletionsUsage
	hasUsage := false

	for _, respJson := range responses {
		resp, err := styles.ParseChatCompletionsResponse(respJson)
		if err != nil {
			plugins.Logger.Warn("parallel plugin: failed to parse response for merging",
				zap.Error(err))
			continue
		}

		// Re-index choices and add to collection
		for _, choice := range resp.Choices {
			choice.Index = len(allChoices)
			allChoices = append(allChoices, choice)
		}

		// Sum up usage if present
		if resp.Usage != nil {
			hasUsage = true
			totalUsage.PromptTokens += resp.Usage.PromptTokens
			totalUsage.CompletionTokens += resp.Usage.CompletionTokens
			totalUsage.TotalTokens += resp.Usage.TotalTokens
		}
	}

	// Use first response as base and update choices
	baseResp, err := styles.ParseChatCompletionsResponse(responses[0])
	if err != nil {
		return nil, err
	}

	baseResp.Choices = allChoices
	if hasUsage {
		baseResp.Usage = &totalUsage
	}

	// Convert back to PartialJSON
	return styles.PartiallyMarshalJSON(baseResp)
}

// parseModelListForParallel parses a pipe-separated model string into a list.
// Returns the list of models (without plugin suffix) and the plugin suffix separately.
func parseModelListForParallel(model string) ([]string, string) {
	// First, extract plugin suffix if present (after + or :)
	plusIdx := strings.IndexByte(model, '+')
	colonIdx := strings.IndexByte(model, ':')

	modelPart := model
	pluginSuffix := ""

	// Find the first delimiter (+ or :) that indicates plugin suffix
	suffixIdx := -1
	if plusIdx >= 0 && (colonIdx < 0 || plusIdx < colonIdx) {
		suffixIdx = plusIdx
	} else if colonIdx >= 0 {
		suffixIdx = colonIdx
	}

	if suffixIdx >= 0 {
		modelPart = model[:suffixIdx]
		pluginSuffix = model[suffixIdx:]
	}

	if !strings.Contains(modelPart, "|") {
		return []string{model}, "" // Return original if no pipe
	}

	parts := strings.Split(modelPart, "|")
	var models []string
	for _, m := range parts {
		m = strings.TrimSpace(m)
		if m != "" {
			models = append(models, m)
		}
	}

	return models, pluginSuffix
}

var (
	_ plugin.RecursiveHandlerPlugin = (*Parallel)(nil)
)
