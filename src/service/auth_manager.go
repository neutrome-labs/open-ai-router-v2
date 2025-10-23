package service

import (
	"net/http"
)

type AuthManager interface {
	CollectTargetAuth(scope string, p *ProviderImpl, r *http.Request) (string, error)
}
