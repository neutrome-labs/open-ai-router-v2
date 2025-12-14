// Package styles defines provider-specific API styles and format conversions.
// V3 upgrade: Styles handle the conversion between input format and provider format,
// supporting passthrough when styles match.
package styles

import (
	"github.com/neutrome-labs/open-ai-router/src/formats"
	"go.uber.org/zap"
)

// Logger for debug output (set by module during Provision)
var Logger *zap.Logger

// Style represents a provider's API style
type Style string

const (
	StyleOpenAIChat      Style = "openai-chat-completions"
	StyleOpenAIResponses Style = "openai-responses"
	StyleAnthropic       Style = "anthropic-messages"
	StyleGoogleGenAI     Style = "google-genai"
	StyleCloudflare      Style = "cloudflare"
)

// ParseStyle parses a style string, defaulting to OpenAI chat completions
func ParseStyle(s string) Style {
	switch s {
	case "openai-chat-completions", "openai", "":
		return StyleOpenAIChat
	case "openai-responses", "responses":
		return StyleOpenAIResponses
	case "anthropic-messages", "anthropic":
		return StyleAnthropic
	case "google-genai", "google":
		return StyleGoogleGenAI
	case "cloudflare", "cf":
		return StyleCloudflare
	default:
		return StyleOpenAIChat
	}
}

// RequestConverter converts requests between styles
type RequestConverter interface {
	// Convert transforms a request from one style to another
	Convert(req formats.ManagedRequest, from, to Style) (formats.ManagedRequest, error)
}

// ResponseConverter converts responses between styles
type ResponseConverter interface {
	// Convert transforms a response from one style to another
	Convert(resp formats.ManagedResponse, from, to Style) (formats.ManagedResponse, error)
}

// StyleEndpoint returns the API endpoint path for a given style and action
func StyleEndpoint(style Style, action string) string {
	switch style {
	case StyleOpenAIChat:
		if action == "chat" {
			return "/chat/completions"
		}
		return "/models"
	case StyleOpenAIResponses:
		if action == "responses" {
			return "/responses"
		}
		return "/models"
	case StyleAnthropic:
		if action == "messages" {
			return "/messages"
		}
		return "/models"
	case StyleGoogleGenAI:
		return "/models"
	case StyleCloudflare:
		return "/ai/run"
	default:
		return "/chat/completions"
	}
}

// StyleContentType returns the Content-Type header for a given style
func StyleContentType(style Style) string {
	return "application/json"
}

// StyleAuthHeader returns the auth header name for a given style
func StyleAuthHeader(style Style) string {
	switch style {
	case StyleAnthropic:
		return "x-api-key"
	default:
		return "Authorization"
	}
}

// StyleAuthFormat returns how to format the auth value for a given style
func StyleAuthFormat(style Style, key string) string {
	switch style {
	case StyleAnthropic:
		return key // Anthropic uses raw key
	default:
		return "Bearer " + key
	}
}

// StyleRequiresVersion returns whether a style requires a version header
func StyleRequiresVersion(style Style) (string, string, bool) {
	switch style {
	case StyleAnthropic:
		return "anthropic-version", "2023-06-01", true
	default:
		return "", "", false
	}
}
