package styles

import (
	"encoding/json"
	"fmt"
)

// ================================================================================
// Conversion Functions between Chat Completions and Responses APIs
// ================================================================================

// ConvertChatCompletionsRequestToResponses converts a Chat Completions request to Responses format
func ConvertChatCompletionsRequestToResponses(reqJson PartialJSON) (PartialJSON, error) {
	res := reqJson.Clone()

	// 1. Rename messages -> input
	if messages, ok := res["messages"]; ok {
		res["input"] = messages
		delete(res, "messages")
	}

	// 2. Rename max_tokens -> max_output_tokens
	if maxTokens, ok := res["max_tokens"]; ok {
		res["max_output_tokens"] = maxTokens
		delete(res, "max_tokens")
	}

	// 3. Convert tools if present
	if toolsRaw, ok := res["tools"]; ok {
		var chatTools []ChatCompletionsTool
		if err := json.Unmarshal(toolsRaw, &chatTools); err != nil {
			return nil, fmt.Errorf("ConvertChatCompletionsRequestToResponses: failed to unmarshal tools: %w", err)
		}

		var respTools []ResponsesTool
		for _, tool := range chatTools {
			respTool := ResponsesTool{
				Type: tool.Type,
			}
			if tool.Function != nil {
				respTool.Name = tool.Function.Name
				respTool.Description = tool.Function.Description
				respTool.Parameters = tool.Function.Parameters
				respTool.Strict = tool.Function.Strict
			}
			respTools = append(respTools, respTool)
		}

		if err := res.Set("tools", respTools); err != nil {
			return nil, fmt.Errorf("ConvertChatCompletionsRequestToResponses: failed to set tools: %w", err)
		}
	}

	return res, nil
}

// ConvertResponsesResponseToChatCompletions converts a Responses API response to Chat Completions format
func ConvertResponsesResponseToChatCompletions(respJson PartialJSON) (PartialJSON, error) {
	res := respJson.Clone()

	// 1. Rename created_at -> created
	if createdAt, ok := res["created_at"]; ok {
		res["created"] = createdAt
		delete(res, "created_at")
	}

	// 2. Convert output -> choices
	if outputRaw, ok := res["output"]; ok {
		var outputItems []ResponsesOutputItem
		if err := json.Unmarshal(outputRaw, &outputItems); err != nil {
			return nil, fmt.Errorf("ConvertResponsesResponseToChatCompletions: failed to unmarshal output: %w", err)
		}

		var choices []ChatCompletionsChoice
		for i, item := range outputItems {
			if item.Type == "message" {
				choice := ChatCompletionsChoice{
					Index: i,
					Message: &ChatCompletionsMessage{
						Role:    item.Role,
						Content: item.Content,
					},
					FinishReason: "stop", // Default
				}
				choices = append(choices, choice)
			}
			// TODO: handle function calls
		}

		if err := res.Set("choices", choices); err != nil {
			return nil, fmt.Errorf("ConvertResponsesResponseToChatCompletions: failed to set choices: %w", err)
		}
		delete(res, "output")
	}

	// 3. Convert usage
	if usageRaw, ok := res["usage"]; ok {
		var respUsage ResponsesUsage
		if err := json.Unmarshal(usageRaw, &respUsage); err != nil {
			return nil, fmt.Errorf("ConvertResponsesResponseToChatCompletions: failed to unmarshal usage: %w", err)
		}

		chatUsage := ChatCompletionsUsage{
			PromptTokens:     respUsage.InputTokens,
			CompletionTokens: respUsage.OutputTokens,
			TotalTokens:      respUsage.TotalTokens,
		}

		if err := res.Set("usage", chatUsage); err != nil {
			return nil, fmt.Errorf("ConvertResponsesResponseToChatCompletions: failed to set usage: %w", err)
		}
	}

	return res, nil
}

// ConvertResponsesResponseChunkToChatCompletions converts a Responses API streaming chunk to Chat Completions format
// Responses API events: response.created, response.output_item.added, response.content_part.delta,
// response.output_text.delta, response.function_call_arguments.delta, response.completed, etc.
func ConvertResponsesResponseChunkToChatCompletions(chunkJson PartialJSON) (PartialJSON, error) {
	eventType := TryGetFromPartialJSON[string](chunkJson, "type")

	switch eventType {
	case "response.created", "response.in_progress":
		// Initial response - convert to first chunk with role
		return buildChatCompletionsChunk(chunkJson, &ChatCompletionsMessage{
			Role: "assistant",
		}, "")

	case "response.output_text.delta":
		// Text content delta
		delta := TryGetFromPartialJSON[string](chunkJson, "delta")
		return buildChatCompletionsChunk(chunkJson, &ChatCompletionsMessage{
			Content: delta,
		}, "")

	case "response.function_call_arguments.delta":
		// Tool call arguments delta
		delta := TryGetFromPartialJSON[string](chunkJson, "delta")
		itemID := TryGetFromPartialJSON[string](chunkJson, "item_id")
		outputIndex := TryGetFromPartialJSON[int](chunkJson, "output_index")

		return buildChatCompletionsChunk(chunkJson, &ChatCompletionsMessage{
			ToolCalls: []ChatCompletionsToolCall{{
				Index: outputIndex,
				ID:    itemID,
				Type:  "function",
				Function: &struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				}{
					Arguments: delta,
				},
			}},
		}, "")

	case "response.output_item.added":
		// New output item - could be message or function call
		var item ResponsesOutputItem
		if itemRaw, ok := chunkJson["item"]; ok {
			if err := json.Unmarshal(itemRaw, &item); err == nil {
				if item.Type == "function_call" {
					return buildChatCompletionsChunk(chunkJson, &ChatCompletionsMessage{
						ToolCalls: []ChatCompletionsToolCall{{
							Index: TryGetFromPartialJSON[int](chunkJson, "output_index"),
							ID:    item.CallID,
							Type:  "function",
							Function: &struct {
								Name      string `json:"name,omitempty"`
								Arguments string `json:"arguments,omitempty"`
							}{
								Name: item.Name,
							},
						}},
					}, "")
				}
			}
		}
		return buildChatCompletionsChunk(chunkJson, &ChatCompletionsMessage{
			Role: "assistant",
		}, "")

	case "response.output_item.done":
		// Output item completed - check finish reason
		var item ResponsesOutputItem
		if itemRaw, ok := chunkJson["item"]; ok {
			if err := json.Unmarshal(itemRaw, &item); err == nil {
				if item.Type == "message" && item.Status == "completed" {
					return buildChatCompletionsChunk(chunkJson, nil, "stop")
				}
				if item.Type == "function_call" && item.Status == "completed" {
					return buildChatCompletionsChunk(chunkJson, nil, "tool_calls")
				}
			}
		}
		return chunkJson, nil

	case "response.completed", "response.done":
		// Final response with usage
		res := make(PartialJSON)

		// Copy ID and model
		if id, ok := chunkJson["response"]; ok {
			var resp struct {
				ID    string         `json:"id"`
				Model string         `json:"model"`
				Usage ResponsesUsage `json:"usage"`
			}
			if err := json.Unmarshal(id, &resp); err == nil {
				res.Set("id", resp.ID)
				res.Set("model", resp.Model)
				res.Set("usage", ChatCompletionsUsage{
					PromptTokens:     resp.Usage.InputTokens,
					CompletionTokens: resp.Usage.OutputTokens,
					TotalTokens:      resp.Usage.TotalTokens,
				})
			}
		}

		res.Set("object", "chat.completion.chunk")
		res.Set("choices", []ChatCompletionsChoice{{
			Index:        0,
			Delta:        &ChatCompletionsMessage{},
			FinishReason: "stop",
		}})

		return res, nil

	default:
		// For unhandled events, passthrough or skip
		// Events like response.content_part.added, response.content_part.done can be skipped
		return nil, nil
	}
}

// buildChatCompletionsChunk creates a Chat Completions streaming chunk
func buildChatCompletionsChunk(source PartialJSON, delta *ChatCompletionsMessage, finishReason string) (PartialJSON, error) {
	res := make(PartialJSON)

	// Try to get response ID from nested response object or top level
	if respRaw, ok := source["response"]; ok {
		var resp struct {
			ID    string `json:"id"`
			Model string `json:"model"`
		}
		if err := json.Unmarshal(respRaw, &resp); err == nil {
			res.Set("id", resp.ID)
			res.Set("model", resp.Model)
		}
	} else {
		if id := TryGetFromPartialJSON[string](source, "response_id"); id != "" {
			res.Set("id", id)
		}
	}

	res.Set("object", "chat.completion.chunk")

	choice := ChatCompletionsChoice{
		Index: 0,
	}
	if delta != nil {
		choice.Delta = delta
	}
	if finishReason != "" {
		choice.FinishReason = finishReason
	}

	res.Set("choices", []ChatCompletionsChoice{choice})

	return res, nil
}
