package services

import (
	"fmt"

	"github.com/neutrome-labs/open-ai-router/src/styles"
)

// DefaultConverter provides request/response conversion between styles.
// Currently only supports passthrough (same style in/out).
// Conversion logic will be implemented later using json.RawMessage.
type DefaultConverter struct{}

// ConvertRequest converts a request from one style to another.
func (c *DefaultConverter) ConvertRequest(reqJson styles.PartialJSON, from, to styles.Style) (styles.PartialJSON, error) {
	if from == to {
		return reqJson, nil // Passthrough
	}

	if from == styles.StyleChatCompletions && to == styles.StyleResponses {
		return styles.ConvertChatCompletionsRequestToResponses(reqJson)
	}

	return nil, fmt.Errorf("conversion from %s to %s not yet implemented", from, to)
}

// ConvertResponse converts a response from one style to another.
func (c *DefaultConverter) ConvertResponse(resJson styles.PartialJSON, from, to styles.Style) (styles.PartialJSON, error) {
	if from == to {
		return resJson, nil // Passthrough
	}

	if from == styles.StyleResponses && to == styles.StyleChatCompletions {
		return styles.ConvertResponsesResponseToChatCompletions(resJson)
	}

	return nil, fmt.Errorf("conversion from %s to %s not yet implemented", from, to)
}

// ConvertResponseChunk converts a response chunk from one style to another.
func (c *DefaultConverter) ConvertResponseChunk(chunkJson styles.PartialJSON, from, to styles.Style) (styles.PartialJSON, error) {
	if from == to {
		return chunkJson, nil // Passthrough
	}

	if from == styles.StyleResponses && to == styles.StyleChatCompletions {
		return styles.ConvertResponsesResponseChunkToChatCompletions(chunkJson)
	}

	return nil, fmt.Errorf("conversion from %s to %s not yet implemented", from, to)
}
