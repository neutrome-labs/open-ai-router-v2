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

	// OpenAI Responses -> OpenAI Chat (simplified)
	if from == StyleOpenAIResponses && to == StyleOpenAIChat {
		responsesResp, ok := resp.(*formats.OpenAIResponsesResponse)
		if !ok {
			return nil, fmt.Errorf("expected OpenAIResponsesResponse, got %T", resp)
		}

		openaiResp := &formats.OpenAIChatResponse{
			ID:     responsesResp.ID,
			Object: "chat.completion",
			Model:  responsesResp.Model,
		}

		// Convert output items to choices
		// First, gather all message content and tool calls
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

		// Determine finish reason based on response status and tool calls
		finishReason := "stop"
		if len(toolCalls) > 0 {
			finishReason = "tool_calls"
		} else {
			// Map Responses API status to Chat Completions finish_reason
			switch responsesResp.Status {
			case "completed":
				finishReason = "stop"
			case "incomplete":
				finishReason = "length"
			case "cancelled":
				finishReason = "stop"
			case "failed":
				finishReason = "stop"
			default:
				finishReason = "stop"
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

		return openaiResp, nil
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
