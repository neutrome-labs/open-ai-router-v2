package styles

import (
	"fmt"

	"github.com/neutrome-labs/open-ai-router/src/formats"
)

// DefaultConverter provides request/response conversion between styles
type DefaultConverter struct{}

// ConvertRequest converts a request from one style to another
func (c *DefaultConverter) ConvertRequest(req formats.ManagedRequest, from, to Style) (formats.ManagedRequest, error) {
	if from == to {
		return req, nil // Passthrough
	}

	// OpenAI Chat -> Anthropic
	if from == StyleOpenAIChat && to == StyleAnthropic {
		openaiReq, ok := req.(*formats.OpenAIChatRequest)
		if !ok {
			return nil, fmt.Errorf("expected OpenAIChatRequest, got %T", req)
		}

		anthropicReq := &formats.AnthropicRequest{}
		anthropicReq.FromOpenAIChat(openaiReq)
		return anthropicReq, nil
	}

	// OpenAI Chat -> OpenAI Responses (simplified)
	if from == StyleOpenAIChat && to == StyleOpenAIResponses {
		openaiReq, ok := req.(*formats.OpenAIChatRequest)
		if !ok {
			return nil, fmt.Errorf("expected OpenAIChatRequest, got %T", req)
		}

		responsesReq := &formats.OpenAIResponsesRequest{
			Model:  openaiReq.Model,
			Input:  openaiReq.Messages,
			Stream: openaiReq.Stream,
		}

		if openaiReq.MaxTokens > 0 {
			responsesReq.MaxOutputTokens = openaiReq.MaxTokens
		}
		responsesReq.Temperature = openaiReq.Temperature
		responsesReq.TopP = openaiReq.TopP
		responsesReq.User = openaiReq.User

		// Convert tools from Chat to Responses format
		// Responses API has flat structure for function tools (name/description/parameters at top level)
		for _, tool := range openaiReq.Tools {
			respTool := formats.ResponsesTool{
				Type: tool.Type,
			}
			if tool.Function != nil {
				respTool.Name = tool.Function.Name
				respTool.Description = tool.Function.Description
				respTool.Parameters = tool.Function.Parameters
				respTool.Strict = tool.Function.Strict
			}
			responsesReq.Tools = append(responsesReq.Tools, respTool)
		}

		// Pass through tool_choice
		responsesReq.ToolChoice = openaiReq.ToolChoice

		// Copy extras
		responsesReq.SetRawExtras(openaiReq.GetRawExtras())

		return responsesReq, nil
	}

	// Anthropic -> OpenAI Chat
	if from == StyleAnthropic && to == StyleOpenAIChat {
		anthropicReq, ok := req.(*formats.AnthropicRequest)
		if !ok {
			return nil, fmt.Errorf("expected AnthropicRequest, got %T", req)
		}

		openaiReq := &formats.OpenAIChatRequest{
			Model:    anthropicReq.Model,
			Messages: anthropicReq.Messages,
			Stream:   anthropicReq.Stream,
		}

		// Add system message if present
		if anthropicReq.System != nil {
			systemMsg := formats.Message{Role: "system"}
			switch v := anthropicReq.System.(type) {
			case string:
				systemMsg.Content = v
			default:
				systemMsg.Content = v
			}
			openaiReq.Messages = append([]formats.Message{systemMsg}, openaiReq.Messages...)
		}

		openaiReq.MaxTokens = anthropicReq.MaxTokens
		openaiReq.Temperature = anthropicReq.Temperature
		openaiReq.TopP = anthropicReq.TopP

		// Convert tools
		for _, tool := range anthropicReq.Tools {
			openaiReq.Tools = append(openaiReq.Tools, formats.Tool{
				Type: "function",
				Function: &formats.ToolFunction{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.InputSchema,
				},
			})
		}

		// Copy extras
		openaiReq.SetRawExtras(anthropicReq.GetRawExtras())

		return openaiReq, nil
	}

	// OpenAI Responses -> OpenAI Chat
	if from == StyleOpenAIResponses && to == StyleOpenAIChat {
		responsesReq, ok := req.(*formats.OpenAIResponsesRequest)
		if !ok {
			return nil, fmt.Errorf("expected OpenAIResponsesRequest, got %T", req)
		}

		openaiReq := &formats.OpenAIChatRequest{
			Model:  responsesReq.Model,
			Stream: responsesReq.Stream,
		}

		// Convert input to messages
		switch v := responsesReq.Input.(type) {
		case string:
			openaiReq.Messages = []formats.Message{{Role: "user", Content: v}}
		case []formats.Message:
			openaiReq.Messages = v
		case []interface{}:
			for _, item := range v {
				if msg, ok := item.(map[string]interface{}); ok {
					m := formats.Message{
						Role: msg["role"].(string),
					}
					if content, ok := msg["content"]; ok {
						m.Content = content
					}
					openaiReq.Messages = append(openaiReq.Messages, m)
				}
			}
		}

		// Add instructions as system message
		if responsesReq.Instructions != "" {
			openaiReq.Messages = append([]formats.Message{{
				Role:    "system",
				Content: responsesReq.Instructions,
			}}, openaiReq.Messages...)
		}

		if responsesReq.MaxOutputTokens > 0 {
			openaiReq.MaxTokens = responsesReq.MaxOutputTokens
		}
		openaiReq.Temperature = responsesReq.Temperature
		openaiReq.TopP = responsesReq.TopP
		openaiReq.User = responsesReq.User

		// Copy extras
		openaiReq.SetRawExtras(responsesReq.GetRawExtras())

		return openaiReq, nil
	}

	return nil, fmt.Errorf("unsupported conversion from %s to %s", from, to)
}

// ConvertResponse converts a response from one style to another
func (c *DefaultConverter) ConvertResponse(resp formats.ManagedResponse, from, to Style) (formats.ManagedResponse, error) {
	if from == to {
		return resp, nil // Passthrough
	}

	// Anthropic -> OpenAI Chat
	if from == StyleAnthropic && to == StyleOpenAIChat {
		anthropicResp, ok := resp.(*formats.AnthropicResponse)
		if !ok {
			return nil, fmt.Errorf("expected AnthropicResponse, got %T", resp)
		}
		return anthropicResp.ToOpenAIChat(), nil
	}

	// OpenAI Responses -> OpenAI Chat
	if from == StyleOpenAIResponses && to == StyleOpenAIChat {
		// Handle streaming events from Responses API
		if streamEvent, ok := resp.(*formats.OpenAIResponsesStreamEvent); ok {
			return convertResponsesStreamEventToChat(streamEvent), nil
		}

		// Handle non-streaming response
		responsesResp, ok := resp.(*formats.OpenAIResponsesResponse)
		if !ok {
			return nil, fmt.Errorf("expected OpenAIResponsesResponse or OpenAIResponsesStreamEvent, got %T", resp)
		}

		return convertResponsesResponseToChat(responsesResp), nil
	}

	// OpenAI Chat -> Anthropic
	if from == StyleOpenAIChat && to == StyleAnthropic {
		openaiResp, ok := resp.(*formats.OpenAIChatResponse)
		if !ok {
			return nil, fmt.Errorf("expected OpenAIChatResponse, got %T", resp)
		}

		anthropicResp := &formats.AnthropicResponse{
			ID:    openaiResp.ID,
			Type:  "message",
			Role:  "assistant",
			Model: openaiResp.Model,
		}

		// Convert choices to content blocks
		for _, choice := range openaiResp.Choices {
			if choice.Message != nil {
				if content, ok := choice.Message.Content.(string); ok {
					anthropicResp.Content = append(anthropicResp.Content, formats.AnthropicContentBlock{
						Type: "text",
						Text: content,
					})
				}

				// Convert tool calls
				for _, tc := range choice.Message.ToolCalls {
					if tc.Function != nil {
						anthropicResp.Content = append(anthropicResp.Content, formats.AnthropicContentBlock{
							Type:  "tool_use",
							ID:    tc.ID,
							Name:  tc.Function.Name,
							Input: tc.Function.Arguments,
						})
					}
				}

				// Map finish reason
				switch choice.FinishReason {
				case "stop":
					anthropicResp.StopReason = "end_turn"
				case "length":
					anthropicResp.StopReason = "max_tokens"
				case "tool_calls":
					anthropicResp.StopReason = "tool_use"
				}
			}
		}

		if openaiResp.Usage != nil {
			anthropicResp.Usage = &formats.AnthropicUsage{
				InputTokens:  openaiResp.Usage.PromptTokens,
				OutputTokens: openaiResp.Usage.CompletionTokens,
			}
		}

		return anthropicResp, nil
	}

	return nil, fmt.Errorf("unsupported conversion from %s to %s", from, to)
}

// convertResponsesStreamEventToChat converts a Responses API streaming event to Chat Completions streaming chunk
func convertResponsesStreamEventToChat(event *formats.OpenAIResponsesStreamEvent) *formats.OpenAIChatResponse {
	resp := &formats.OpenAIChatResponse{
		Object: "chat.completion.chunk",
	}

	// Map different event types to appropriate Chat Completions delta format
	switch event.Type {
	case "response.output_text.delta":
		// Text delta - this is the main content streaming event
		resp.Choices = []formats.Choice{{
			Index: event.OutputIndex,
			Delta: &formats.Message{
				Role:    "assistant",
				Content: event.Delta,
			},
		}}
	case "response.output_text.done":
		// Text done - final content, mark as finished
		resp.Choices = []formats.Choice{{
			Index:        event.OutputIndex,
			Delta:        &formats.Message{},
			FinishReason: "stop",
		}}
	case "response.output_item.added":
		// Output item added - send role
		if event.Item != nil && event.Item.Role != "" {
			resp.Choices = []formats.Choice{{
				Index: event.OutputIndex,
				Delta: &formats.Message{
					Role: event.Item.Role,
				},
			}}
		}
	case "response.content_part.added":
		// Content part added - skip, we'll get the delta events
		resp.Choices = []formats.Choice{{
			Index: event.OutputIndex,
			Delta: &formats.Message{},
		}}
	case "response.function_call_arguments.delta":
		// Function call arguments delta
		resp.Choices = []formats.Choice{{
			Index: event.OutputIndex,
			Delta: &formats.Message{
				ToolCalls: []formats.ToolCall{{
					Index: event.ContentIndex,
					Function: &struct {
						Name      string `json:"name,omitempty"`
						Arguments string `json:"arguments,omitempty"`
					}{
						Arguments: event.Delta,
					},
				}},
			},
		}}
	case "response.function_call_arguments.done":
		// Function call done
		resp.Choices = []formats.Choice{{
			Index: event.OutputIndex,
			Delta: &formats.Message{
				ToolCalls: []formats.ToolCall{{
					ID:    event.ItemID,
					Type:  "function",
					Index: event.ContentIndex,
					Function: &struct {
						Name      string `json:"name,omitempty"`
						Arguments string `json:"arguments,omitempty"`
					}{
						Name:      event.Name,
						Arguments: event.Arguments,
					},
				}},
			},
			FinishReason: "tool_calls",
		}}
	case "response.completed":
		// Response completed - get final info from response object
		if event.Response != nil {
			resp.ID = event.Response.ID
			resp.Model = event.Response.Model
			if event.Response.Usage != nil {
				resp.Usage = &formats.Usage{
					PromptTokens:     event.Response.Usage.InputTokens,
					CompletionTokens: event.Response.Usage.OutputTokens,
					TotalTokens:      event.Response.Usage.TotalTokens,
				}
			}
		}
		resp.Choices = []formats.Choice{{
			Index:        0,
			Delta:        &formats.Message{},
			FinishReason: "stop",
		}}
	case "response.created", "response.in_progress":
		// Response metadata events - extract ID and model if available
		if event.Response != nil {
			resp.ID = event.Response.ID
			resp.Model = event.Response.Model
		}
		resp.Choices = []formats.Choice{{
			Index: 0,
			Delta: &formats.Message{},
		}}
	default:
		// For unhandled event types, return empty delta
		resp.Choices = []formats.Choice{{
			Index: 0,
			Delta: &formats.Message{},
		}}
	}

	return resp
}

// convertResponsesResponseToChat converts a non-streaming Responses API response to Chat Completions format
func convertResponsesResponseToChat(responsesResp *formats.OpenAIResponsesResponse) *formats.OpenAIChatResponse {
	openaiResp := &formats.OpenAIChatResponse{
		ID:     responsesResp.ID,
		Object: "chat.completion",
		Model:  responsesResp.Model,
	}

	// Convert output items to choices
	var content string
	var toolCalls []formats.ToolCall
	var role string = "assistant"

	for _, item := range responsesResp.Output {
		if item.Type == "message" {
			if item.Role != "" {
				role = item.Role
			}
			for _, part := range item.Content {
				if part.Type == "text" || part.Type == "output_text" {
					content += part.Text
				}
			}
		} else if item.Type == "function_call" {
			tc := formats.ToolCall{
				ID:   item.CallID,
				Type: "function",
			}
			tc.Function = &struct {
				Name      string `json:"name,omitempty"`
				Arguments string `json:"arguments,omitempty"`
			}{
				Name:      item.Name,
				Arguments: item.Arguments,
			}
			toolCalls = append(toolCalls, tc)
		}
	}

	// Determine finish reason
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	} else {
		switch responsesResp.Status {
		case "completed":
			finishReason = "stop"
		case "incomplete":
			finishReason = "length"
		}
	}

	choice := formats.Choice{
		Index: 0,
		Message: &formats.Message{
			Role:    role,
			Content: content,
		},
		FinishReason: finishReason,
	}

	if len(toolCalls) > 0 {
		choice.Message.ToolCalls = toolCalls
	}

	openaiResp.Choices = append(openaiResp.Choices, choice)

	if responsesResp.Usage != nil {
		openaiResp.Usage = &formats.Usage{
			PromptTokens:     responsesResp.Usage.InputTokens,
			CompletionTokens: responsesResp.Usage.OutputTokens,
			TotalTokens:      responsesResp.Usage.TotalTokens,
		}
	}

	return openaiResp
}
