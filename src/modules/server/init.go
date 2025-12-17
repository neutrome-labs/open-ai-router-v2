// Package modules provides Caddy v2 HTTP handler modules for AI routing.
package server

import (
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
)

func init() {
	caddy.RegisterModule(&ListModelsModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_list_models", ParseListModelsModule)
	httpcaddyfile.RegisterDirectiveOrder("ai_list_models", httpcaddyfile.Before, "header")

	caddy.RegisterModule(&OpenAIChatCompletionsModule{})
	httpcaddyfile.RegisterHandlerDirective("ai_openai_chat_completions", ParseOpenAIChatCompletionsModule)
	httpcaddyfile.RegisterDirectiveOrder("ai_openai_chat_completions", httpcaddyfile.Before, "header")
}
