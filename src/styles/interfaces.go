package styles

import (
	"encoding/json"
	"fmt"
	"maps"

	"go.uber.org/zap"
)

// Logger for debug output (set by module during Provision)
var Logger *zap.Logger = zap.NewNop()

// Style represents a provider's API style
type Style string

const (
	StyleUnknown         Style = ""
	StyleVirtual         Style = "virtual"
	StyleChatCompletions Style = "openai-chat-completions"
	StyleResponses       Style = "openai-responses"
	StyleAnthropic       Style = "anthropic-messages"
	StyleGoogleGenAI     Style = "google-genai"
	StyleCfAiGateway     Style = "cloudflare-ai-gateway"
	StyleCfWorkersAi     Style = "cloudflare-workers-ai"
)

type PartialJSON map[string]json.RawMessage

// ParseStyle parses a style string, defaulting to OpenAI chat completions
func ParseStyle(s string) (Style, error) {
	switch s {
	case "virtual":
		return StyleVirtual, nil
	case "openai-chat-completions", "openai", "":
		return StyleChatCompletions, nil
	case "openai-responses", "responses":
		return StyleResponses, nil
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

func ParsePartialJSON(data []byte) (PartialJSON, error) {
	var pj PartialJSON
	err := json.Unmarshal(data, &pj)
	return pj, err
}

func PartiallyMarshalJSON(obj any) (PartialJSON, error) {
	// todo find a way to avoid double marshal
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return ParsePartialJSON(data)
}

func GetFromPartialJSON[T any](pj PartialJSON, key string) (T, error) {
	var zero T
	raw, ok := pj[key]
	if !ok {
		return zero, nil
	}
	var result T
	err := json.Unmarshal(raw, &result)
	if err != nil {
		return zero, err
	}
	return result, nil
}

func TryGetFromPartialJSON[T any](pj PartialJSON, key string) T {
	var zero T
	raw, ok := pj[key]
	if !ok {
		return zero
	}
	var result T
	err := json.Unmarshal(raw, &result)
	if err != nil {
		return zero
	}
	return result
}

func (pj PartialJSON) Set(key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	pj[key] = b
	return nil
}

func (pj PartialJSON) Clone() PartialJSON {
	clone := make(PartialJSON)
	maps.Copy(clone, pj)
	return clone
}

func (pj PartialJSON) CloneWith(key string, value any) (PartialJSON, error) {
	clone := pj.Clone()
	err := clone.Set(key, value)
	if err != nil {
		return nil, err
	}
	return clone, nil
}

func (pj PartialJSON) Marshal() ([]byte, error) {
	return json.Marshal(pj)
}
