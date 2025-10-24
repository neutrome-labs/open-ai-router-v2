package services

import "net/url"

type ProviderImpl struct {
	Name      string
	ParsedURL url.URL
	Router    *RouterImpl
	Commands  map[string]any
}
