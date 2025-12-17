package plugins

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/neutrome-labs/open-ai-router/src/plugin"
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
	invoker plugin.HandlerInvoker,
	reqBody []byte,
	w http.ResponseWriter,
	r *http.Request,
) (handled bool, err error) {
	// Extract model from request
	var known struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(reqBody, &known); err != nil {
		return false, nil
	}
	model := known.Model

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
		resp  json.RawMessage
		err   error
	}

	results := make(chan result, len(models))
	var wg sync.WaitGroup

	for i, m := range models {
		i, m := i, m // capture for goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Clone request, set model and disable streaming
			cloned := cloneSetModelAndDisableStream(reqBody, m, r)
			resp, err := invoker.InvokeHandlerCapture(cloned)
			results <- result{index: i, model: m, resp: resp, err: err}
		}()
	}

	// Wait for all results
	wg.Wait()
	close(results)

	// Collect results in order
	responses := make([]json.RawMessage, len(models))
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
	var firstResponse json.RawMessage
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
			zap.Int("total_responses", successCount))
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Parallel-Models", strings.Join(models, "|"))
	_, err = w.Write(merged)
	return true, err
}

// cloneSetModelAndDisableStream creates a copy of the request with a new model and stream=false
func cloneSetModelAndDisableStream(reqBody []byte, model string, r *http.Request) *http.Request {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(reqBody, &data); err != nil {
		return r
	}

	modelJSON, _ := json.Marshal(model)
	data["model"] = modelJSON
	data["stream"] = json.RawMessage("false")
	delete(data, "stream_options")

	result, err := json.Marshal(data)
	if err != nil {
		return r
	}

	clone := r.Clone(r.Context())
	clone.Body = io.NopCloser(strings.NewReader(string(result)))
	return clone
}

// mergeParallelResponses combines multiple responses into one by merging choices.
// The first response is used as the base, and choices from other responses are appended
// with re-indexed choice indices.
func mergeParallelResponses(primary json.RawMessage, all []json.RawMessage, models []string) json.RawMessage {
	if primary == nil {
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

	// Parse the primary response
	var baseResp map[string]json.RawMessage
	if err := json.Unmarshal(primary, &baseResp); err != nil {
		return primary
	}

	// Collect all choices with re-indexed indices
	var allChoices []json.RawMessage
	idx := 0

	for _, resp := range all {
		if resp == nil {
			continue
		}

		var respData struct {
			Choices []json.RawMessage `json:"choices"`
		}
		if err := json.Unmarshal(resp, &respData); err != nil {
			continue
		}

		for _, choice := range respData.Choices {
			// Update the index in the choice
			var choiceMap map[string]json.RawMessage
			if err := json.Unmarshal(choice, &choiceMap); err != nil {
				continue
			}
			indexJSON, _ := json.Marshal(idx)
			choiceMap["index"] = indexJSON

			updatedChoice, err := json.Marshal(choiceMap)
			if err != nil {
				continue
			}
			allChoices = append(allChoices, updatedChoice)
			idx++
		}
	}

	// Update the model to show all merged models
	modelJSON, _ := json.Marshal(strings.Join(models, "|"))
	baseResp["model"] = modelJSON

	// Update choices
	choicesJSON, _ := json.Marshal(allChoices)
	baseResp["choices"] = choicesJSON

	// TODO: Merge usage from all responses

	result, err := json.Marshal(baseResp)
	if err != nil {
		return primary
	}
	return result
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
