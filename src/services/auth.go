// Package services provides core service interfaces and implementations.
package services

import (
	"net/http"
	"strings"
	"sync"
)

var authServiceRegistry sync.Map

// AuthService handles authentication for provider requests
type AuthService interface {
	// CollectIncomingAuth is called early in request handling (after router resolution)
	// to allow auth managers to set context values that plugins can depend on.
	// This is called once per incoming request before any provider attempts.
	CollectIncomingAuth(r *http.Request) (*http.Request, error)

	// CollectTargetAuth is called when preparing the outgoing request to a provider.
	// It returns the auth value (e.g., API key) to use for the provider request.
	CollectTargetAuth(scope string, p *ProviderService, rIn, rOut *http.Request) (string, error)
}

// NopAuthService is a no-op auth manager
type NopAuthService struct{}

func (NopAuthService) CollectIncomingAuth(r *http.Request) (*http.Request, error) {
	return r, nil
}

func (NopAuthService) CollectTargetAuth(scope string, p *ProviderService, rIn, rOut *http.Request) (string, error) {
	return "", nil
}

// RegisterAuthService registers an auth manager by name
func RegisterAuthService(name string, m AuthService) {
	authServiceRegistry.Store(strings.ToLower(name), m)
}

// GetAuthService retrieves an auth manager by name
func GetAuthService(name string) AuthService {
	if v, ok := authServiceRegistry.Load(strings.ToLower(name)); ok {
		if m, ok2 := v.(AuthService); ok2 {
			return m
		}
	}
	return &NopAuthService{}
}
