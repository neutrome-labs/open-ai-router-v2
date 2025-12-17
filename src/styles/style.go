// Package styles defines provider-specific API styles and format conversions.
// V3 upgrade: Styles handle the conversion between input format and provider format,
// supporting passthrough when styles match.
package styles

import (
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// Logger for debug output (set by module during Provision)
var Logger *zap.Logger

// Style represents a provider's API style
type Style string

const (
	StyleUnknown         Style = ""
	StyleOpenAIChat      Style = "openai-chat-completions"
	StyleOpenAIResponses Style = "openai-responses"
	StyleAnthropic       Style = "anthropic-messages"
	StyleGoogleGenAI     Style = "google-genai"
	StyleCfAiGateway     Style = "cloudflare-ai-gateway"
	StyleCfWorkersAi     Style = "cloudflare-workers-ai"
)

// ParseStyle parses a style string, defaulting to OpenAI chat completions
func ParseStyle(s string) (Style, error) {
	switch s {
	case "openai-chat-completions", "openai", "":
		return StyleOpenAIChat, nil
	case "openai-responses", "responses":
		return StyleOpenAIResponses, nil
	/*case "anthropic-messages", "anthropic":
		return StyleAnthropic, nil
	case "google-genai", "google":
		return StyleGoogleGenAI, nil
	case "cloudflare-ai-gateway":
		return StyleCfAiGateway, nil
	case "cloudflare-workers-ai", "cloudflare", "cf":
		return StyleCfWorkersAi, nil*/
	default:
		return StyleUnknown, fmt.Errorf("unknown style: %s", s)
	}
}

// RequestConverter converts requests between styles
type RequestConverter interface {
	// Convert transforms a request from one style to another
	Convert(req json.RawMessage, from, to Style) (json.RawMessage, error)
}

// ResponseConverter converts responses between styles
type ResponseConverter interface {
	// Convert transforms a response from one style to another
	Convert(resp json.RawMessage, from, to Style) (json.RawMessage, error)
}
