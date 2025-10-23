package modules

import (
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/neutrome-labs/open-ai-router-v2/src/service"
)

var APP_VERSION = "2.0.0"

func init() {
	service.TryInstrumentAppObservability()
	defer func() {
		_ = service.FireObservabilityEvent("app", "", "init", map[string]any{
			"version": APP_VERSION,
		})
	}()

	caddy.RegisterModule(&EnvAuthManagerModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_auth_env", ParseEnvAuthManagerModuleModule)

	caddy.RegisterModule(&RouterModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_router", ParseRouterModule)

	caddy.RegisterModule(&ListModelsModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_list_models", ParseListModelsModule)

	caddy.RegisterModule(&ChatCompletionsModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_chat_completions", ParseChatCompletionsModule)
}
