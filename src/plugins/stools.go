package plugins

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/styles"
	"go.uber.org/zap"
)

// Stools (Smart Tools) strips tool calls and their responses from conversation history,
// preserving only the last tool interaction. Earlier tool calls are truncated to 100 chars.
// This reduces context size while keeping the current tool state intact.
//
// Supports: Chat Completions style (messages array)
// TODO: Add Responses API support
//
// Usage: model+stools
type Stools struct{}

func (s *Stools) Name() string {
	return "stools"
}

// isToolMessage checks if a ChatCompletionsMessage is related to tool calling
func isToolMessage(msg styles.ChatCompletionsMessage) bool {
	if len(msg.ToolCalls) > 0 {
		return true
	}
	if msg.Role == "tool" {
		return true
	}
	if msg.ToolCallID != "" {
		return true
	}
	return false
}

// truncateContent truncates content to maxLen characters with ellipsis
func truncateContent(content any, maxLen int) any {
	if content == nil {
		return content
	}

	// Handle string content
	if strContent, ok := content.(string); ok {
		if len(strContent) <= maxLen {
			return content
		}
		if maxLen <= 3 {
			return strContent[:maxLen]
		}
		return strContent[:maxLen-3] + "..."
	}

	// Handle multimodal content ([]ChatCompletionsContentPart)
	if parts, ok := content.([]any); ok {
		for i, part := range parts {
			if partMap, ok := part.(map[string]any); ok {
				if text, ok := partMap["text"].(string); ok {
					if len(text) > maxLen {
						if maxLen > 3 {
							partMap["text"] = text[:maxLen-3] + "..."
						} else {
							partMap["text"] = text[:maxLen]
						}
						parts[i] = partMap
					}
				}
			}
		}
		return parts
	}

	return content
}

// truncateToolCalls truncates tool call arguments to maxLen
func truncateToolCalls(toolCalls []styles.ChatCompletionsToolCall, maxLen int) []styles.ChatCompletionsToolCall {
	for i := range toolCalls {
		if toolCalls[i].Function != nil {
			args := toolCalls[i].Function.Arguments
			if len(args) > maxLen {
				if maxLen > 3 {
					toolCalls[i].Function.Arguments = args[:maxLen-3] + "..."
				} else {
					toolCalls[i].Function.Arguments = args[:maxLen]
				}
			}
		}
	}
	return toolCalls
}

// findLastToolInteractionBoundary finds the start index of the last tool interaction that should be preserved.
func findLastToolInteractionBoundary(messages []styles.ChatCompletionsMessage) int {
	if len(messages) == 0 {
		return -1
	}

	lastMsg := messages[len(messages)-1]
	if !isToolMessage(lastMsg) {
		return -1
	}

	for i := len(messages) - 1; i >= 0; i-- {
		if !isToolMessage(messages[i]) {
			return i + 1
		}
	}

	return 0
}

func (s *Stools) Before(params string, p *services.ProviderService, r *http.Request, reqBody []byte) ([]byte, error) {
	if Logger != nil {
		Logger.Debug("stools plugin before hook")
	}

	switch p.Style {
	case styles.StyleOpenAIChat:
		return s.processChatCompletions(reqBody)

	case styles.StyleOpenAIResponses:
		if Logger != nil {
			Logger.Debug("stools: Responses API style detected, not yet supported")
		}
		return nil, fmt.Errorf("stools plugin does not yet support Responses API style")

	default:
		return reqBody, nil
	}
}

func (s *Stools) processChatCompletions(reqBody []byte) ([]byte, error) {
	const truncateLen = 100

	// Parse full request
	req, err := styles.ParseChatCompletionsRequest(reqBody)
	if err != nil {
		return reqBody, nil
	}

	if len(req.Messages) == 0 {
		return reqBody, nil
	}

	preserveFromIdx := findLastToolInteractionBoundary(req.Messages)

	if Logger != nil {
		Logger.Debug("stools analysis",
			zap.Int("totalMessages", len(req.Messages)),
			zap.Int("preserveFromIdx", preserveFromIdx))
	}

	// Check if there are any tool messages
	hasToolMessages := false
	for _, msg := range req.Messages {
		if isToolMessage(msg) {
			hasToolMessages = true
			break
		}
	}
	if !hasToolMessages {
		return reqBody, nil
	}

	// Process messages: truncate tool content in earlier messages
	for i := range req.Messages {
		msg := &req.Messages[i]
		shouldTruncate := preserveFromIdx == -1 || i < preserveFromIdx

		if shouldTruncate && isToolMessage(*msg) {
			if msg.Role == "tool" || msg.ToolCallID != "" {
				msg.Content = truncateContent(msg.Content, truncateLen)
			}
			if len(msg.ToolCalls) > 0 {
				msg.ToolCalls = truncateToolCalls(msg.ToolCalls, truncateLen)
			}
		}
	}

	// Rebuild request with modified messages
	var fullReq map[string]json.RawMessage
	if err := json.Unmarshal(reqBody, &fullReq); err != nil {
		return reqBody, nil
	}

	messagesJSON, err := json.Marshal(req.Messages)
	if err != nil {
		return reqBody, nil
	}
	fullReq["messages"] = messagesJSON

	result, err := json.Marshal(fullReq)
	if err != nil {
		return reqBody, nil
	}

	if Logger != nil {
		Logger.Debug("stools processed messages", zap.Int("preserveFromIdx", preserveFromIdx))
	}

	return result, nil
}
