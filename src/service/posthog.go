package service

import (
	"os"

	"github.com/posthog/posthog-go"
)

var posthogClient posthog.Client

func TryInstrumentAppObservability() bool {
	key := os.Getenv("POSTHOG_API_KEY")
	if key == "" {
		return false
	}

	client, err := posthog.NewWithConfig(key, posthog.Config{Endpoint: os.Getenv("POSTHOG_BASE_URL")})
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
