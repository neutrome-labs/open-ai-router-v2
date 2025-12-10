package services

import (
	"os"

	"github.com/posthog/posthog-go"
)

var posthogEndpoint = os.Getenv("POSTHOG_BASE_URL")
var posthogAPIKey = os.Getenv("POSTHOG_API_KEY")
var PosthogIncludeContent = os.Getenv("POSTHOG_INCLUDE_CONTENT") == "true"

var posthogClient posthog.Client

func TryInstrumentAppObservability() bool {
	if posthogAPIKey == "" {
		return false
	}

	client, err := posthog.NewWithConfig(posthogAPIKey, posthog.Config{Endpoint: posthogEndpoint})
	if err != nil {
		return false
	}

	posthogClient = client
	return true
}

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
