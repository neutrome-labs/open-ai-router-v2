package plugins

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router/src/plugin"
	"github.com/neutrome-labs/open-ai-router/src/services"
	"github.com/neutrome-labs/open-ai-router/src/styles"
)

// StripTools provides plugin to strip completed tool calls from messages.
// It removes all tool calls (assistant messages with tool_calls) and tool responses
// (messages with role "tool") except for the last tool interaction sequence.
// The idea is that the AI has already summarized previous tool responses and has
// all required information in the text bodies, so we can reduce token usage.
type StripTools struct {
}

func (f *StripTools) Name() string { return "stools" }

func (f *StripTools) Before(params string, p *services.ProviderService, r *http.Request, reqJson styles.PartialJSON) (styles.PartialJSON, error) {
	// Get messages from request
	messages, err := styles.GetFromPartialJSON[[]styles.ChatCompletionsMessage](reqJson, "messages")
	if err != nil {
		return reqJson, err
	}

	if len(messages) == 0 {
		return reqJson, nil
	}

	// Find all tool interaction boundaries (sequences of tool_call + tool responses)
	// A tool interaction starts with an assistant message containing tool_calls
	// and ends before the next non-tool message or at the end of messages

	type toolInteraction struct {
		startIdx int // Index of assistant message with tool_calls
		endIdx   int // Index of last tool response in this sequence (inclusive)
	}

	var interactions []toolInteraction
	i := 0
	for i < len(messages) {
		msg := messages[i]

		// Check if this is an assistant message with tool calls
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			interaction := toolInteraction{startIdx: i, endIdx: i}

			// Find all subsequent tool response messages
			j := i + 1
			for j < len(messages) && messages[j].Role == "tool" {
				interaction.endIdx = j
				j++
			}

			interactions = append(interactions, interaction)
			i = j
		} else {
			i++
		}
	}

	// If we have 0 or 1 tool interactions, nothing to strip
	if len(interactions) <= 1 {
		return reqJson, nil
	}

	// Keep only the last tool interaction, strip all previous ones
	// Build new messages list excluding all but the last interaction
	lastInteraction := interactions[len(interactions)-1]
	indicesToRemove := make(map[int]bool)

	for _, interaction := range interactions[:len(interactions)-1] {
		for idx := interaction.startIdx; idx <= interaction.endIdx; idx++ {
			indicesToRemove[idx] = true
		}
	}

	// Also strip tool_calls from assistant messages that are NOT part of the last interaction
	// but keep their text content
	newMessages := make([]styles.ChatCompletionsMessage, 0, len(messages)-len(indicesToRemove))

	for idx, msg := range messages {
		if indicesToRemove[idx] {
			continue
		}

		// For assistant messages with tool_calls that are not part of the last interaction,
		// strip the tool_calls but keep the message if it has content
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			isLastInteraction := idx >= lastInteraction.startIdx && idx <= lastInteraction.endIdx
			if !isLastInteraction {
				// Strip tool_calls, keep only content
				if msg.Content != nil {
					newMessages = append(newMessages, styles.ChatCompletionsMessage{
						Role:    msg.Role,
						Name:    msg.Name,
						Content: msg.Content,
					})
				}
				continue
			}
		}

		newMessages = append(newMessages, msg)
	}

	// Update reqJson with filtered messages
	return reqJson.CloneWith("messages", newMessages)
}

var (
	_ plugin.BeforePlugin = (*StripTools)(nil)
)
