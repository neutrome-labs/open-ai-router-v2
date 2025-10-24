package modules

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/neutrome-labs/open-ai-router-v2/src/drivers/openai"
	"github.com/neutrome-labs/open-ai-router-v2/src/services"
	"go.uber.org/zap"
)

var routerRegistry sync.Map // map[string]*RouterModule

func RegisterRouter(name string, m *RouterModule) {
	routerRegistry.Store(strings.ToLower(name), m)
}

func GetRouter(name string) (*RouterModule, bool) {
	if strings.TrimSpace(name) == "" {
		name = "default"
	}
	if v, ok := routerRegistry.Load(strings.ToLower(name)); ok {
		if m, ok2 := v.(*RouterModule); ok2 {
			return m, true
		}
	}
	return nil, false
}

type RouterModule struct {
	Name                    string                  `json:"name,omitempty"`
	AuthManagerName         string                  `json:"auth_manager,omitempty"`
	Providers               map[string]*ProviderDef `json:"providers,omitempty"`
	DefaultProviderForModel map[string][]string     `json:"default_provider_for_model,omitempty"`
	ProviderOrder           []string                `json:"provider_order,omitempty"`

	impl services.RouterImpl
}

type ProviderDef struct {
	Name       string `json:"name,omitempty"`
	APIBaseURL string `json:"api_base_url,omitempty"`
	Style      string `json:"style,omitempty"`

	impl services.ProviderImpl
}

func ParseRouterModule(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m RouterModule
	err := m.UnmarshalCaddyfile(h.Dispenser)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (*RouterModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.ai_router",
		New: func() caddy.Module { return new(RouterModule) },
	}
}

func (m *RouterModule) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	m.impl.Mu.Lock()
	defer m.impl.Mu.Unlock()

	if m.Providers == nil {
		m.Providers = make(map[string]*ProviderDef)
	}
	if m.DefaultProviderForModel == nil {
		m.DefaultProviderForModel = make(map[string][]string)
	}
	if m.ProviderOrder == nil {
		m.ProviderOrder = []string{}
	}

	for d.Next() {
		if d.Val() == "ai_router" && !d.NextArg() {
		}
		for d.NextBlock(0) {
			switch d.Val() {
			case "name":
				if !d.NextArg() {
					return d.ArgErr()
				}
				m.Name = strings.ToLower(strings.TrimSpace(d.Val()))
			case "auth":
				if !d.NextArg() {
					return d.ArgErr()
				}
				m.AuthManagerName = strings.ToLower(strings.TrimSpace(d.Val()))
			case "provider":
				if !d.NextArg() {
					return d.ArgErr()
				}
				providerName := strings.ToLower(d.Val())
				if _, ok := m.Providers[providerName]; ok {
					return d.Errf("provider %s already defined", providerName)
				}
				p := ProviderDef{Name: providerName}
				for d.NextBlock(1) {
					switch d.Val() {
					case "api_base_url":
						if !d.NextArg() {
							return d.ArgErr()
						}
						p.APIBaseURL = d.Val()
					case "style":
						if !d.NextArg() {
							return d.ArgErr()
						}
						p.Style = strings.ToLower(d.Val())
					default:
						return d.Errf("unrecognized provider option '%s' for provider '%s'", d.Val(), providerName)
					}
				}
				if p.APIBaseURL == "" {
					return d.Errf("provider %s: api_base_url is required", providerName)
				}
				m.Providers[providerName] = &p
				m.ProviderOrder = append(m.ProviderOrder, providerName)
			case "default_provider_for_model":
				args := d.RemainingArgs()
				if len(args) < 2 {
					return d.Errf("default_provider_for_model expects <model_name> <provider_name_1> [<provider_name_2>...], got %d args", len(args))
				}
				modelName := args[0]
				var providerNames []string
				for _, pName := range args[1:] {
					providerNames = append(providerNames, strings.ToLower(pName))
				}
				m.DefaultProviderForModel[modelName] = providerNames
			default:
				return d.Errf("unrecognized ai_core_router option '%s'", d.Val())
			}
		}
	}

	return nil
}

func (m *RouterModule) Provision(ctx caddy.Context) error {
	m.impl.Logger = ctx.Logger(m)
	m.impl.Mu.Lock()
	defer m.impl.Mu.Unlock()

	if strings.TrimSpace(m.Name) == "" {
		m.Name = "default"
	}

	if m.impl.AuthManager == nil {
		m.impl.AuthManager = services.GetAuthManager(m.AuthManagerName)
	}

	for _, name := range m.ProviderOrder {
		p := m.Providers[name]

		if p.APIBaseURL == "" {
			return fmt.Errorf("provider %s: api_base_url is required", name)
		}
		parsedURL, err := url.Parse(p.APIBaseURL)
		if err != nil {
			return fmt.Errorf("provider %s: invalid api_base_url '%s': %v", name, p.APIBaseURL, err)
		}
		p.impl = services.ProviderImpl{
			Name:      name,
			ParsedURL: *parsedURL,
			Router:    &m.impl,
		}

		var providerCommands map[string]any
		switch p.Style {
		/*case "google":
			providerCommands = []interface{}{
				&commands.ListModelsGoogle{},
				&commands.CompletionsGoogle{},
			}
		case "anthropic":
			providerCommands = []interface{}{
				&commands.ListModelsAnthropic{},
				&commands.CompletionsAnthropic{},
			}
		case "cloudflare":
			providerCommands = map[string]any{
				"list_models":      &cfai.ListModels{},
				"chat_completions": &cfai.ChatCompletions{},
			}*/
		default:
			providerCommands = map[string]any{
				"list_models":      &openai.ListModels{},
				"chat_completions": &openai.ChatCompletions{},
			}
		}
		p.impl.Commands = providerCommands

		m.impl.Logger.Info("Provisioned provider", zap.String("name", name), zap.String("base_url", p.APIBaseURL))
	}

	RegisterRouter(m.Name, m)
	return nil
}

func (m *RouterModule) Validate() error {
	m.impl.Mu.RLock()
	defer m.impl.Mu.RUnlock()

	if len(m.Providers) == 0 {
		return fmt.Errorf("at least one provider must be configured for ai_core_router")
	}
	return nil
}

func (m *RouterModule) ServeHTTP(w http.ResponseWriter, req *http.Request, next caddyhttp.Handler) error {
	return next.ServeHTTP(w, req)
}

func (m *RouterModule) ResolveProvidersOrderAndModel(model string) (providerNames []string, actualModelName string) { // Receiver changed to AICoreRouter (r)
	m.impl.Mu.RLock() // Ensure read lock for accessing shared provider maps
	defer m.impl.Mu.RUnlock()

	actualModelName = model // Default to requested model name

	// Check for explicit provider prefix: "providerName/modelName"
	parts := strings.SplitN(model, "/", 2)
	if len(parts) == 2 {
		pName := strings.ToLower(parts[0])
		model := parts[1]
		if _, ok := m.Providers[pName]; ok { // Check if the prefixed provider is configured
			m.impl.Logger.Debug("Found explicit provider by prefix", zap.String("prefix", pName), zap.String("model", model)) // Changed to Debug
			return append([]string{pName}, m.ProviderOrder...), model
		}
		// Log if prefix is found but provider isn't recognized, then proceed to other checks
		m.impl.Logger.Debug("Prefix found but provider not recognized, checking defaults", zap.String("prefix", pName), zap.String("requested_model", model)) // Changed to Debug
	}

	// Check for model-specific default provider
	if pNames, ok := m.DefaultProviderForModel[model]; ok {
		for _, pName := range pNames {
			if _, providerExists := m.Providers[pName]; providerExists {
				m.impl.Logger.Debug("Found default provider for model", zap.String("model", model), zap.String("provider", pName)) // Changed to Debug
				return append([]string{pName}, m.ProviderOrder...), model                                                          // Model name remains as requested
			}
			m.impl.Logger.Warn("Default provider for model configured but provider itself not found", zap.String("model", model), zap.String("configured_provider", pName))
		}
	}

	return m.ProviderOrder, model
}

var (
	_ caddy.Provisioner           = (*RouterModule)(nil)
	_ caddy.Validator             = (*RouterModule)(nil)
	_ caddyhttp.MiddlewareHandler = (*RouterModule)(nil)
	_ caddyfile.Unmarshaler       = (*RouterModule)(nil)
)
