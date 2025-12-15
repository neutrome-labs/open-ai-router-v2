package plugins

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router/src/formats"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"go.uber.org/zap"
)

// Stools (Smart Tools) strips tool calls and their responses from conversation history,
// preserving only the last tool interaction. Earlier tool calls are truncated to 100 chars.
// This reduces context size while keeping the current tool state intact.
//
// Usage: model+stools
type Stools struct{}

func (s *Stools) Name() string {
	return "stools"
}

// isToolMessage checks if a message is related to tool calling
func isToolMessage(msg formats.Message) bool {
	// Message with tool_calls (assistant calling tools)
	if len(msg.ToolCalls) > 0 {
		return true
	}
	// Message with tool role (tool response)
	if msg.Role == "tool" {
		return true
	}
	// Message with tool_call_id (tool result)
	if msg.ToolCallID != "" {
		return true
	}
	return false
}

// truncateContent truncates content to maxLen characters with ellipsis
func truncateContent(content any, maxLen int) any {
	switch c := content.(type) {
	case string:
		if len(c) <= maxLen {
			return c
		}
		if maxLen <= 3 {
			return c[:maxLen]
		}
		return c[:maxLen-3] + "..."
	case []any:
		// For content arrays (multimodal), truncate text parts
		result := make([]any, len(c))
		for i, part := range c {
			if partMap, ok := part.(map[string]any); ok {
				newPart := make(map[string]any)
				for k, v := range partMap {
					if k == "text" {
						if text, ok := v.(string); ok {
							if len(text) > maxLen {
								if maxLen > 3 {
									newPart[k] = text[:maxLen-3] + "..."
								} else {
									newPart[k] = text[:maxLen]
								}
							} else {
								newPart[k] = text
							}
						} else {
							newPart[k] = v
						}
					} else {
						newPart[k] = v
					}
				}
				result[i] = newPart
			} else {
				result[i] = part
			}
		}
		return result
	default:
		return content
	}
}

// truncateToolCalls truncates tool call arguments to maxLen
func truncateToolCalls(toolCalls []formats.ToolCall, maxLen int) []formats.ToolCall {
	if len(toolCalls) == 0 {
		return toolCalls
	}

	result := make([]formats.ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		result[i] = formats.ToolCall{
			Index: tc.Index,
			ID:    tc.ID,
			Type:  tc.Type,
		}
		if tc.Function != nil {
			args := tc.Function.Arguments
			if len(args) > maxLen {
				if maxLen > 3 {
					args = args[:maxLen-3] + "..."
				} else {
					args = args[:maxLen]
				}
			}
			result[i].Function = &struct {
				Name      string `json:"name,omitempty"`
				Arguments string `json:"arguments,omitempty"`
			}{
				Name:      tc.Function.Name,
				Arguments: args,
			}
		}
	}
	return result
}

// findLastToolInteractionBoundary finds the start index of the last tool interaction that should be preserved.
// Returns -1 if no tool messages found, or if all tool interactions are completed (followed by non-tool messages).
// A tool interaction is "active" if it's at the very end of the conversation without subsequent non-tool messages.
func findLastToolInteractionBoundary(messages []formats.Message) int {
	if len(messages) == 0 {
		return -1
	}

	// Check if the last message is a tool message
	// If not, all tool interactions are "completed" and should be truncated
	lastMsg := messages[len(messages)-1]
	if !isToolMessage(lastMsg) {
		return -1 // All tool interactions are completed, truncate everything
	}

	// Find the start of the active tool interaction at the end
	// Walk backwards from the end to find where this tool interaction starts
	for i := len(messages) - 1; i >= 0; i-- {
		if !isToolMessage(messages[i]) {
			return i + 1 // The tool interaction starts right after this non-tool message
		}
	}

	// All messages are tool messages
	return 0
}

func (s *Stools) Before(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest) (formats.ManagedRequest, error) {
	if Logger != nil {
		Logger.Debug("stools plugin before hook")
	}

	const truncateLen = 100

	messages := req.GetMessages()
	if len(messages) == 0 {
		return req, nil
	}

	// Find the boundary: messages at or after this index are preserved (not truncated)
	// If -1, all tool interactions should be truncated (they're all "completed")
	preserveFromIdx := findLastToolInteractionBoundary(messages)

	if Logger != nil {
		Logger.Debug("stools analysis",
			zap.Int("totalMessages", len(messages)),
			zap.Int("preserveFromIdx", preserveFromIdx))
	}

	// Check if there are any tool messages to process
	hasToolMessages := false
	for _, msg := range messages {
		if isToolMessage(msg) {
			hasToolMessages = true
			break
		}
	}
	if !hasToolMessages {
		return req, nil
	}

	// Process messages: truncate tool content in earlier messages
	newMessages := make([]formats.Message, len(messages))
	for i, msg := range messages {
		newMsg := formats.Message{
			Role:       msg.Role,
			Name:       msg.Name,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
			Refusal:    msg.Refusal,
			ToolCalls:  msg.ToolCalls,
		}

		// Truncate if:
		// - preserveFromIdx is -1 (all tool interactions completed), OR
		// - this message is before the preserved boundary
		shouldTruncate := preserveFromIdx == -1 || i < preserveFromIdx

		if shouldTruncate && isToolMessage(msg) {
			// Truncate content for tool response messages
			if msg.Role == "tool" || msg.ToolCallID != "" {
				newMsg.Content = truncateContent(msg.Content, truncateLen)
			}
			// Truncate tool call arguments
			if len(msg.ToolCalls) > 0 {
				newMsg.ToolCalls = truncateToolCalls(msg.ToolCalls, truncateLen)
			}
		}

		newMessages[i] = newMsg
	}

	req.SetMessages(newMessages)

	if Logger != nil {
		Logger.Debug("stools processed messages",
			zap.Int("preserveFromIdx", preserveFromIdx))
	}

	return req, nil
}
