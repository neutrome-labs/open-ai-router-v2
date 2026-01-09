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
//
// Message structure in OpenAI API:
// - Assistant makes tool calls: role="assistant" with tool_calls array
// - Tool results: role="tool" with tool_call_id referencing the call
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
	// and ends with the last consecutive tool response message

	type toolInteraction struct {
		startIdx int // Index of assistant message with tool_calls
		endIdx   int // Index of last tool response in this sequence (inclusive)
	}

	var interactions []toolInteraction
	for i := 0; i < len(messages); i++ {
		msg := messages[i]

		// Check if this is an assistant message with tool calls
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			interaction := toolInteraction{startIdx: i, endIdx: i}

			// Find all subsequent tool response messages (role="tool")
			for j := i + 1; j < len(messages) && messages[j].Role == "tool"; j++ {
				interaction.endIdx = j
			}

			interactions = append(interactions, interaction)
			// Continue from after the interaction to find more
			i = interaction.endIdx
		}
	}

	// If we have 0 or 1 tool interactions, nothing to strip
	if len(interactions) <= 1 {
		return reqJson, nil
	}

	// Build set of indices to remove (all interactions except the last one)
	indicesToRemove := make(map[int]bool)
	for _, interaction := range interactions[:len(interactions)-1] {
		for idx := interaction.startIdx; idx <= interaction.endIdx; idx++ {
			indicesToRemove[idx] = true
		}
	}

	// In-place removal: use write index to compact the slice
	writeIdx := 0
	for readIdx := 0; readIdx < len(messages); readIdx++ {
		if indicesToRemove[readIdx] {
			// For assistant messages being removed, preserve text content if present
			if messages[readIdx].Role == "assistant" && messages[readIdx].Content != nil {
				messages[writeIdx] = styles.ChatCompletionsMessage{
					Role:    messages[readIdx].Role,
					Name:    messages[readIdx].Name,
					Content: messages[readIdx].Content,
				}
				writeIdx++
			}
			// Tool response messages (role="tool") are fully removed
			continue
		}

		if writeIdx != readIdx {
			messages[writeIdx] = messages[readIdx]
		}
		writeIdx++
	}

	// Update reqJson with truncated messages
	return reqJson.CloneWith("messages", messages[:writeIdx])
}

var (
	_ plugin.BeforePlugin = (*StripTools)(nil)
)
