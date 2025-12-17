package services

import (
	"net/url"

	"github.com/neutrome-labs/open-ai-router/src/styles"
)

// ProviderService provides the runtime implementation for a provider
type ProviderService struct {
	Name      string
	ParsedURL url.URL
	Style     styles.Style
	Router    *RouterService
	Commands  map[string]any
}
