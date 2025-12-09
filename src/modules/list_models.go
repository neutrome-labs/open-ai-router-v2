package modules

import (
	"encoding/json"
	"net/http"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/neutrome-labs/open-ai-router-v2/src/commands"
	"go.uber.org/zap"
)

// ListModelsModule serves aggregated models under any path.
type ListModelsModule struct {
	RouterName string `json:"router,omitempty"`
	logger     *zap.Logger
}

func ParseListModelsModule(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m ListModelsModule
	for h.Next() {
		for h.NextBlock(0) {
			switch h.Val() {
			case "router":
				if !h.NextArg() {
					return nil, h.ArgErr()
				}
				m.RouterName = h.Val()
			default:
				return nil, h.Errf("unrecognized ai_list_models option '%s'", h.Val())
			}
		}
	}
	return &m, nil
}

func (*ListModelsModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.ai_list_models",
		New: func() caddy.Module { return new(ListModelsModule) },
	}
}

func (m *ListModelsModule) Provision(ctx caddy.Context) error {
	m.logger = ctx.Logger(m)
	return nil
}

func (m *ListModelsModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	router, ok := GetRouter(m.RouterName)
	if !ok {
		m.logger.Error("Router not found", zap.String("name", m.RouterName))
		http.Error(w, "Router not found", http.StatusInternalServerError)
		return nil
	}

	models := make([]commands.ListModelsModel, 0)
	for _, name := range router.ProviderOrder {
		p := router.Providers[name]
		if p == nil {
			m.logger.Warn("Provider config is nil", zap.String("name", name))
			continue
		}

		if p.impl.Commands == nil || len(p.impl.Commands) == 0 {
			m.logger.Warn("Provider commands is nil or empty", zap.String("name", name))
			continue
		}

		if _, ok := p.impl.Commands["list_models"]; !ok {
			continue
		}

		cmd, ok := p.impl.Commands["list_models"].(commands.ListModelsCommand)
		if !ok {
			continue
		}

		xmodels, err := cmd.DoListModels(&p.impl, r)
		if err != nil {
			m.logger.Error("Error listing models", zap.String("provider", name), zap.Error(err))
			continue
		}

		models = append(models, xmodels...)
	}

	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   models,
	})
}

var (
	_ caddy.Provisioner           = (*ListModelsModule)(nil)
	_ caddyhttp.MiddlewareHandler = (*ListModelsModule)(nil)
)
