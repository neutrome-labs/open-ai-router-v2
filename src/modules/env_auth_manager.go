package modules

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/neutrome-labs/open-ai-router-v2/src/services"
	"go.uber.org/zap"
)

// EnvAuthManagerModule serves authentication from environment variables.
type EnvAuthManagerModule struct {
	Name   string `json:"name,omitempty"`
	logger *zap.Logger
}

func ParseEnvAuthManagerModuleModule(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m EnvAuthManagerModule
	for h.Next() {
		for h.NextBlock(0) {
			switch h.Val() {
			case "name":
				if !h.NextArg() {
					return nil, h.ArgErr()
				}
				m.Name = h.Val()
				if m.Name == "" {
					m.Name = "default"
				}
			default:
				return nil, h.Errf("unrecognized ai_auth_env option '%s'", h.Val())
			}
		}
	}
	return &m, nil
}

func (*EnvAuthManagerModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.ai_auth_env",
		New: func() caddy.Module { return new(EnvAuthManagerModule) },
	}
}

func (m *EnvAuthManagerModule) Provision(ctx caddy.Context) error {
	m.logger = ctx.Logger(m)
	services.RegisterAuthManager(m.Name, m)
	return nil
}

func (m *EnvAuthManagerModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	return next.ServeHTTP(w, r)
}

func (m *EnvAuthManagerModule) CollectTargetAuth(scope string, p *services.ProviderImpl, rIn, rOut *http.Request) (string, error) {
	key := os.Getenv(strings.ToUpper(p.Name) + "_API_KEY")
	if key == "" {
		m.logger.Warn("no key found in environment variables for provider", zap.String("provider", p.Name))
		return "", nil
	}

	ctx := context.WithValue(rIn.Context(), "key_id", "env:"+p.Name)
	ctx = context.WithValue(ctx, "user_id", "env:"+p.Name)
	*rIn = *rIn.WithContext(ctx)

	return key, nil
}

var (
	_ caddy.Provisioner           = (*EnvAuthManagerModule)(nil)
	_ caddyhttp.MiddlewareHandler = (*EnvAuthManagerModule)(nil)
)
