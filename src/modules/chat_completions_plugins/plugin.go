package chatcompletionsplugins

import (
	"net/http"

	"github.com/neutrome-labs/open-ai-router-v2/src/services"
	"go.uber.org/zap"
)

// Logger is the shared logger for all plugins, set by the parent module
var Logger *zap.Logger = zap.NewNop()

// SetLogger sets the shared logger for plugins (called from ChatCompletionsModule.Provision)
func SetLogger(l *zap.Logger) {
	if l != nil {
		Logger = l
	}
}

type ChatCompletionsPlugin interface {
	Before(params string, p *services.ProviderImpl, r *http.Request, body []byte) ([]byte, error)
	After(params string, p *services.ProviderImpl, r *http.Request, body []byte, hres *http.Response, res map[string]any) (map[string]any, error)
}
