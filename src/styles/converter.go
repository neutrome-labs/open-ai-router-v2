package styles

import (
	"encoding/json"
	"fmt"
)

// DefaultConverter provides request/response conversion between styles.
// Currently only supports passthrough (same style in/out).
// Conversion logic will be implemented later using json.RawMessage.
type DefaultConverter struct{}

// ConvertRequest converts a request from one style to another.
// Currently only supports passthrough when styles match.
func (c *DefaultConverter) ConvertRequest(reqBody []byte, from, to Style) ([]byte, error) {
	if from == to {
		return reqBody, nil // Passthrough
	}

	return nil, fmt.Errorf("conversion from %s to %s not yet implemented", from, to)
}

// ConvertResponse converts a response from one style to another.
// Currently only supports passthrough when styles match.
func (c *DefaultConverter) ConvertResponse(resBody []byte, from, to Style) (json.RawMessage, error) {
	if from == to {
		return resBody, nil // Passthrough
	}

	return nil, fmt.Errorf("conversion from %s to %s not yet implemented", from, to)
}
