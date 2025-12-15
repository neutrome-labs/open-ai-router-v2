// Package services provides core service interfaces and implementations.
package services

import (
	"net/http"
	"strings"
	"sync"
)

var authManagerRegistry sync.Map

// AuthManager handles authentication for provider requests
type AuthManager interface {
	// CollectIncomingAuth is called early in request handling (after router resolution)
	// to allow auth managers to set context values that plugins can depend on.
	// This is called once per incoming request before any provider attempts.
	CollectIncomingAuth(r *http.Request) (*http.Request, error)

	// CollectTargetAuth is called when preparing the outgoing request to a provider.
	// It returns the auth value (e.g., API key) to use for the provider request.
	CollectTargetAuth(scope string, p *ProviderImpl, rIn, rOut *http.Request) (string, error)
}

// NopAuthManager is a no-op auth manager
type NopAuthManager struct{}

func (NopAuthManager) CollectIncomingAuth(r *http.Request) (*http.Request, error) {
	return r, nil
}

func (NopAuthManager) CollectTargetAuth(scope string, p *ProviderImpl, rIn, rOut *http.Request) (string, error) {
	return "", nil
}

// RegisterAuthManager registers an auth manager by name
func RegisterAuthManager(name string, m AuthManager) {
	authManagerRegistry.Store(strings.ToLower(name), m)
}

// GetAuthManager retrieves an auth manager by name
func GetAuthManager(name string) AuthManager {
	if v, ok := authManagerRegistry.Load(strings.ToLower(name)); ok {
		if m, ok2 := v.(AuthManager); ok2 {
			return m
		}
	}
	return &NopAuthManager{}
}
