package services

import (
	"os"

	"github.com/posthog/posthog-go"
)

var posthogClient posthog.Client

// PosthogIncludeContent controls whether to include message content in observability events
var PosthogIncludeContent = os.Getenv("POSTHOG_INCLUDE_CONTENT") == "true"

// TryInstrumentAppObservability initializes PostHog if configured
func TryInstrumentAppObservability() bool {
	key := os.Getenv("POSTHOG_API_KEY")
	if key == "" {
		return false
	}

	baseURL := os.Getenv("POSTHOG_BASE_URL")
	if baseURL == "" {
		baseURL = "https://app.posthog.com"
	}

	client, err := posthog.NewWithConfig(key, posthog.Config{Endpoint: baseURL})
	if err != nil {
		return false
	}

	posthogClient = client
	return true
}

// FireObservabilityEvent sends an event to PostHog
func FireObservabilityEvent(userId, url, eventName string, properties map[string]any) error {
	if posthogClient == nil {
		return nil
	}

	if userId == "" {
		userId = "anonymous"
	}

	if url != "" {
		properties["$current_url"] = url
	}

	return posthogClient.Enqueue(posthog.Capture{
		DistinctId: userId,
		Event:      eventName,
		Properties: properties,
	})
}
