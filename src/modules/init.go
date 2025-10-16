package modules

import (
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

func init() {
	caddy.RegisterModule(EnvAuthManagerModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_auth_env", ParseEnvAuthManagerModuleModule)

	caddy.RegisterModule(&RouterModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_router", ParseRouterModule)

	caddy.RegisterModule(ListModelsModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_list_models", ParseListModelsModule)

	caddy.RegisterModule(ChatCompletionsModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_chat_completions", ParseChatCompletionsModule)
}
