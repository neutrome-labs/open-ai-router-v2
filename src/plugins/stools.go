package plugins

import (
	"encoding/json"
	"net/http"

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

// message is a minimal struct for message inspection and modification
type message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	Name       string          `json:"name,omitempty"`
	Refusal    string          `json:"refusal,omitempty"`
}

// isToolMessage checks if a message is related to tool calling
func isToolMessageRaw(msg message) bool {
	// Message with tool_calls (assistant calling tools)
	if len(msg.ToolCalls) > 0 && string(msg.ToolCalls) != "null" {
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

// truncateContentRaw truncates content in JSON format to maxLen characters with ellipsis
func truncateContentRaw(content json.RawMessage, maxLen int) json.RawMessage {
	if content == nil || len(content) == 0 {
		return content
	}

	// Try to parse as string
	var strContent string
	if err := json.Unmarshal(content, &strContent); err == nil {
		if len(strContent) <= maxLen {
			return content
		}
		if maxLen <= 3 {
			truncated, _ := json.Marshal(strContent[:maxLen])
			return truncated
		}
		truncated, _ := json.Marshal(strContent[:maxLen-3] + "...")
		return truncated
	}

	// Try to parse as array (multimodal content)
	var arrContent []map[string]any
	if err := json.Unmarshal(content, &arrContent); err == nil {
		for i, part := range arrContent {
			if text, ok := part["text"].(string); ok {
				if len(text) > maxLen {
					if maxLen > 3 {
						part["text"] = text[:maxLen-3] + "..."
					} else {
						part["text"] = text[:maxLen]
					}
					arrContent[i] = part
				}
			}
		}
		truncated, _ := json.Marshal(arrContent)
		return truncated
	}

	return content
}

// truncateToolCallsRaw truncates tool call arguments in JSON format to maxLen
func truncateToolCallsRaw(toolCalls json.RawMessage, maxLen int) json.RawMessage {
	if toolCalls == nil || len(toolCalls) == 0 || string(toolCalls) == "null" {
		return toolCalls
	}

	var tcs []map[string]any
	if err := json.Unmarshal(toolCalls, &tcs); err != nil {
		return toolCalls
	}

	for i, tc := range tcs {
		if fn, ok := tc["function"].(map[string]any); ok {
			if args, ok := fn["arguments"].(string); ok && len(args) > maxLen {
				if maxLen > 3 {
					fn["arguments"] = args[:maxLen-3] + "..."
				} else {
					fn["arguments"] = args[:maxLen]
				}
				tc["function"] = fn
				tcs[i] = tc
			}
		}
	}

	result, _ := json.Marshal(tcs)
	return result
}

// findLastToolInteractionBoundaryRaw finds the start index of the last tool interaction that should be preserved.
// Returns -1 if no tool messages found, or if all tool interactions are completed (followed by non-tool messages).
// A tool interaction is "active" if it's at the very end of the conversation without subsequent non-tool messages.
func findLastToolInteractionBoundaryRaw(messages []message) int {
	if len(messages) == 0 {
		return -1
	}

	// Check if the last message is a tool message
	// If not, all tool interactions are "completed" and should be truncated
	lastMsg := messages[len(messages)-1]
	if !isToolMessageRaw(lastMsg) {
		return -1 // All tool interactions are completed, truncate everything
	}

	// Find the start of the active tool interaction at the end
	// Walk backwards from the end to find where this tool interaction starts
	for i := len(messages) - 1; i >= 0; i-- {
		if !isToolMessageRaw(messages[i]) {
			return i + 1 // The tool interaction starts right after this non-tool message
		}
	}

	// All messages are tool messages
	return 0
}

func (s *Stools) Before(params string, p *services.ProviderService, r *http.Request, req json.RawMessage) (json.RawMessage, error) {
	if Logger != nil {
		Logger.Debug("stools plugin before hook")
	}

	const truncateLen = 100

	// Parse request to extract messages
	var reqData struct {
		Messages []message `json:"messages"`
	}
	if err := json.Unmarshal(req, &reqData); err != nil {
		return req, nil
	}

	if len(reqData.Messages) == 0 {
		return req, nil
	}

	// Find the boundary: messages at or after this index are preserved (not truncated)
	// If -1, all tool interactions should be truncated (they're all "completed")
	preserveFromIdx := findLastToolInteractionBoundaryRaw(reqData.Messages)

	if Logger != nil {
		Logger.Debug("stools analysis",
			zap.Int("totalMessages", len(reqData.Messages)),
			zap.Int("preserveFromIdx", preserveFromIdx))
	}

	// Check if there are any tool messages to process
	hasToolMessages := false
	for _, msg := range reqData.Messages {
		if isToolMessageRaw(msg) {
			hasToolMessages = true
			break
		}
	}
	if !hasToolMessages {
		return req, nil
	}

	// Process messages: truncate tool content in earlier messages
	for i := range reqData.Messages {
		msg := &reqData.Messages[i]

		// Truncate if:
		// - preserveFromIdx is -1 (all tool interactions completed), OR
		// - this message is before the preserved boundary
		shouldTruncate := preserveFromIdx == -1 || i < preserveFromIdx

		if shouldTruncate && isToolMessageRaw(*msg) {
			// Truncate content for tool response messages
			if msg.Role == "tool" || msg.ToolCallID != "" {
				msg.Content = truncateContentRaw(msg.Content, truncateLen)
			}
			// Truncate tool call arguments
			if len(msg.ToolCalls) > 0 && string(msg.ToolCalls) != "null" {
				msg.ToolCalls = truncateToolCallsRaw(msg.ToolCalls, truncateLen)
			}
		}
	}

	// Rebuild request with modified messages
	var fullReq map[string]json.RawMessage
	if err := json.Unmarshal(req, &fullReq); err != nil {
		return req, nil
	}

	messagesJSON, err := json.Marshal(reqData.Messages)
	if err != nil {
		return req, nil
	}
	fullReq["messages"] = messagesJSON

	result, err := json.Marshal(fullReq)
	if err != nil {
		return req, nil
	}

	if Logger != nil {
		Logger.Debug("stools processed messages",
			zap.Int("preserveFromIdx", preserveFromIdx))
	}

	return result, nil
}
