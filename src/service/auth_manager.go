package service

import (
	"net/http"
)

type AuthManager interface {
	CollectTargetAuth(p *ProviderImpl, r *http.Request) (string, error)
}
