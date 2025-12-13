package services

import (
	"net/url"

	"github.com/neutrome-labs/open-ai-router/src/styles"
)

// ProviderImpl provides the runtime implementation for a provider
type ProviderImpl struct {
	Name      string
	ParsedURL url.URL
	Style     styles.Style
	Router    *RouterImpl
	Commands  map[string]any
}
