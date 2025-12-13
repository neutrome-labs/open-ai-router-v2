// Package modules provides Caddy v2 HTTP handler modules for AI routing.
package modules

import (
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/neutrome-labs/open-ai-router/src/services"
)

var APP_VERSION = "3.0.0"

func init() {
	services.TryInstrumentAppObservability()
	defer func() {
		_ = services.FireObservabilityEvent("app", "", "init", map[string]any{
			"version": APP_VERSION,
		})
	}()

	// Auth manager
	caddy.RegisterModule(&EnvAuthManagerModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_auth_env", ParseEnvAuthManagerModule)

	// Router
	caddy.RegisterModule(&RouterModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_router", ParseRouterModule)

	// List models
	caddy.RegisterModule(&ListModelsModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_list_models", ParseListModelsModule)

	// OpenAI Chat Completions (input style)
	caddy.RegisterModule(&OpenAIChatCompletionsModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_chat_completions", ParseOpenAIChatCompletionsModule)
	httpcaddyfile.RegisterHandlerDirective("ai_openai_chat_completions", ParseOpenAIChatCompletionsModule)

	// OpenAI Responses API (input style)
	caddy.RegisterModule(&OpenAIResponsesModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_openai_responses", ParseOpenAIResponsesModule)

	// Anthropic Messages API (input style)
	caddy.RegisterModule(&AnthropicMessagesModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_anthropic_messages", ParseAnthropicMessagesModule)

	// Register directive ordering - these run early, before standard handlers
	httpcaddyfile.RegisterDirectiveOrder("ai_auth_env", httpcaddyfile.Before, "header")
	httpcaddyfile.RegisterDirectiveOrder("ai_router", httpcaddyfile.Before, "header")
	httpcaddyfile.RegisterDirectiveOrder("ai_list_models", httpcaddyfile.Before, "header")
	httpcaddyfile.RegisterDirectiveOrder("ai_chat_completions", httpcaddyfile.Before, "header")
	httpcaddyfile.RegisterDirectiveOrder("ai_openai_chat_completions", httpcaddyfile.Before, "header")
	httpcaddyfile.RegisterDirectiveOrder("ai_openai_responses", httpcaddyfile.Before, "header")
	httpcaddyfile.RegisterDirectiveOrder("ai_anthropic_messages", httpcaddyfile.Before, "header")
}
