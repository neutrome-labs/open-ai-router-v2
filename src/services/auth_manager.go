package services

import (
	"net/http"
	"strings"
	"sync"
)

var authManagerRegistry sync.Map

type AuthManager interface {
	CollectTargetAuth(scope string, p *ProviderImpl, rIn, rOut *http.Request) (string, error)
}

type NopAuthManager struct{}

func (NopAuthManager) CollectTargetAuth(scope string, p *ProviderImpl, rIn, rOut *http.Request) (string, error) {
	return "", nil
}

func RegisterAuthManager(name string, m AuthManager) {
	authManagerRegistry.Store(strings.ToLower(name), m)
}

func GetAuthManager(name string) AuthManager {
	if v, ok := authManagerRegistry.Load(strings.ToLower(name)); ok {
		if m, ok2 := v.(AuthManager); ok2 {
			return m
		}
	}

	return &NopAuthManager{}
}
