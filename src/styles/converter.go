package styles

import (
	"fmt"
)

// DefaultConverter provides request/response conversion between styles.
// Currently only supports passthrough (same style in/out).
// Conversion logic will be implemented later using json.RawMessage.
type DefaultConverter struct{}

// ConvertRequest converts a request from one style to another.
func (c *DefaultConverter) ConvertRequest(reqBody []byte, from, to Style) ([]byte, error) {
	if from == to {
		return reqBody, nil // Passthrough
	}

	if from == StyleOpenAIChat && to == StyleOpenAIResponses {
		return ConvertChatCompletionsRequestToResponses(reqBody)
	}

	return nil, fmt.Errorf("conversion from %s to %s not yet implemented", from, to)
}

// ConvertResponse converts a response from one style to another.
func (c *DefaultConverter) ConvertResponse(resBody []byte, from, to Style) ([]byte, error) {
	if from == to {
		return resBody, nil // Passthrough
	}

	if from == StyleOpenAIResponses && to == StyleOpenAIChat {
		converted, err := ConvertResponsesResponseToChatCompletions(resBody)
		return converted, err
	}

	return nil, fmt.Errorf("conversion from %s to %s not yet implemented", to, from)
}

// ConvertResponseChunk converts a response chunk from one style to another.
func (c *DefaultConverter) ConvertResponseChunk(chunkBody []byte, from, to Style) ([]byte, error) {
	if from == to {
		return chunkBody, nil // Passthrough
	}

	if from == StyleOpenAIResponses && to == StyleOpenAIChat {
		converted, err := ConvertResponsesResponseChunkToChatCompletions(chunkBody)
		return converted, err
	}

	return nil, fmt.Errorf("conversion from %s to %s not yet implemented", to, from)
}
