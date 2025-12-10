# open-ai-router-v2

A Caddy-based HTTP router for LLM providers, with plugins.

This project is a caddy plugin, and builds a custom `caddy` binary that exposes a small set of HTTP handler directives:

- `ai_auth_env` - configure global auth proxy using environment variables.
- `ai_router` - define providers and routing rules for models.
- `ai_list_models` - aggregate `/models` from all configured providers.
- `ai_chat_completions` - route `/chat/completions` requests with plugin support.

See example configuration in `Caddyfile.example`.

---

## Features

- **Multiple providers** via a single HTTP endpoint.
  - Each provider has its own `api_base_url`.
  - Default driver is OpenAI-compatible (see `src/drivers/openai`).
- **Routing by model name**:
  - Global provider order.
  - Optional per-model default provider mapping.
  - Explicit provider prefix support: `provider/model`.
- **Chat plugins** (see `src/modules/chat_completions_plugins`):
  - `posthog` - observability events.
  - `models` - model name mapping.
  - `fuzz` - fuzzy resolution of model names.
  - `zip`, `zipc`, `zips`, `zipsc` - auto-compaction of long conversations, with variants for caching/preserving the first user message.
- **Streaming and non-streaming** chat completions.
- **Env-based auth manager** (`ai_auth_env`) for per-provider keys.

---

## Requirements

- Go (version compatible with the `go.mod` in this repository).
- Basic familiarity with [Caddy v2](https://caddyserver.com/) and Caddyfile configuration.

---

## Building

From the repo root:

```bash
make build
```

This runs:

```bash
go build -o caddy ./src
```

The resulting `caddy` binary includes the custom AI modules from this project.

---

## Configuration

The canonical reference is `Caddyfile.example`. Below is a simplified walkthrough.

### Global module order

```caddyfile
{
    order ai_auth_env first
    order ai_router after ai_auth_env
}
```

This ensures auth runs before routing.

### Example site block

```caddyfile
http://localhost:3000 {
    log {
        output stderr
        level DEBUG
    }

    ai_auth_env {
        name default
    }

    ai_router {
        name default
        auth default

        provider openrouter {
            api_base_url "https://openrouter.ai/api/v1"
        }

        provider cf {
            api_base_url "https://api.cloudflare.com/client/v4/accounts/<account_id>/ai"
            style "cloudflare"
        }
    }

    handle_path /api/models {
        route {
            # CORS
            header Access-Control-Allow-Origin "*"
            header Access-Control-Allow-Methods "GET, POST, PUT, DELETE, OPTIONS"
            header Access-Control-Allow-Headers "Authorization, Content-Type, X-Requested-With, X-CSRF-Token, *"
            @options method OPTIONS
            respond @options 204

            ai_list_models {
                router default
            }
        }
    }

    handle_path /api/chat/completions* {
        route {
            header Access-Control-Allow-Origin "*"
            header Access-Control-Allow-Methods "GET, POST, PUT, DELETE, OPTIONS"
            header Access-Control-Allow-Headers "Authorization, Content-Type, X-Requested-With, X-CSRF-Token, *"
            @options method OPTIONS
            respond @options 204

            ai_chat_completions {
                router default
            }
        }
    }
}
```

### `ai_auth_env`

Defined in `src/modules/env_auth_manager.go`.

```caddyfile
ai_auth_env {
    name default
}
```

- Registers an auth manager named `default`.
- For each provider `X`, it reads the API key from the environment variable:
  - `${X}_API_KEY` (uppercased provider name), e.g. `OPENROUTER_API_KEY`, `CF_API_KEY`.
- If no key is found, a warning is logged, and requests are forwarded without `Authorization`.

### `ai_router`

Defined in `src/modules/router.go`.

Basic structure:

```caddyfile
ai_router {
    name default              # router name (optional, default: "default")
    auth default              # auth manager name (from ai_auth_env)

    provider openrouter {
        api_base_url "https://openrouter.ai/api/v1"
        # style "openai"    # default style
    }

    # Additional providers...

    # Optional: default provider per model
    # default_provider_for_model gpt-4.1 openrouter
}
```

Key points:

- `provider <name>`
  - `api_base_url` (required) - base URL for OpenAI-compatible API.
  - `style` (optional) - reserved for future provider-specific behavior (currently the default uses `drivers/openai`).
- `default_provider_for_model <model> <provider1> [provider2 ...]`
  - Prefer the given provider(s) when that model is requested.

Model routing behavior:

- If the client uses `model="provider/model"`, that provider is explicitly chosen (if configured).
- Otherwise, if a `default_provider_for_model` is configured for that model, it is used.
- Otherwise, the router falls back to the global `ProviderOrder` (in the order providers are defined).

### `ai_list_models`

Defined in `src/modules/list_models.go`.

```caddyfile
ai_list_models {
    router default
}
```

- Calls `/models` for each configured provider.
- Aggregates results into a single JSON list:

```json
{
  "object": "list",
  "data": [
    { "id": "providerA:model1", ... },
    { "id": "providerB:model2", ... }
  ]
}
```

(Exact shape depends on `commands.ListModelsModel`.)

### `ai_chat_completions`

Defined in `src/modules/chat_completions.go`.

```caddyfile
ai_chat_completions {
    router default
}
```

Behavior:

- Accepts OpenAI-compatible `/chat/completions` requests (JSON body with `model`, `messages`, etc.).
- Resolves the target provider and normalized model name using the configured router.
- Applies a chain of chat plugins (see below).
- For non-streaming requests, returns a single JSON response.
- For streaming (`stream: true`), returns an SSE-like `text/event-stream` with `data: ...` chunks and a final `data: [DONE]`.

Response headers include debug info:

- `X-Real-Provider-Id` - the provider actually used.
- `X-Real-Model-Id` - the final model name after mapping.
- `X-Plugins-Executed` - comma-separated list of plugins and optional params.

---

## Chat Plugins

Plugins are implemented in `src/modules/chat_completions_plugins` and wired in `ChatCompletionsModule.Provision`.

### Always-on plugins

Currently, these plugins are always executed for chat completions:

- `posthog` - observability and analytics.
- `models` - model name translation/mapping.

### Optional plugins via URL path and model suffix

You can enable plugins in two ways:

1. **URL path**: `/api/chat/completions/<plugin1>[:arg]/<plugin2>[:arg]...`
2. **Model suffix**: `model="gpt-4.1+plugin1:arg+plugin2"`

Plugins available out of the box:

- `fuzz` - fuzzy matching for model names.
- `zip` - auto-compact long conversations.
- `zipc` - like `zip`, but preserve the first user message.
- `zips` - like `zip`, but disable cache (recompact each time).
- `zipsc` - preserve first user message and disable cache.

The `zip` family takes an optional numeric parameter (approx max tokens):

- `zip:65535`, `zipc:65535`, `zips:65535`, `zipsc:65535`.

The `zip` plugin is an OSS implementation of "context autocompact" method - it summarizes older conversation turns using the model itself, then replaces them with a concise summary plus a short assistant acknowledgement, keeping recent turns intact.

---

## Environment Variables

- `POSTHOG_API_KEY` - if set, enables PostHog observability.
- `POSTHOG_BASE_URL` - optional, PostHog endpoint.
- `POSTHOG_INCLUDE_CONTENT` - if `true`, may include more content in observability events.
- `<PROVIDER_NAME>_API_KEY` - API key for each provider, uppercased provider name as configured in `ai_router`.

Example:

```bash
export OPENROUTER_API_KEY="sk-..."
export CF_API_KEY="..."
export POSTHOG_API_KEY="phc_..."
```

---

## Running

1. Build the custom Caddy binary:

   ```bash
   make build
   ```

2. Copy or adapt `Caddyfile.example` to `Caddyfile`, set your provider URLs and keys.

3. Run Caddy from the repo root:

   ```bash
   ./caddy run --config Caddyfile --adapter caddyfile
   ```

4. Test endpoints (example):

   - List models:

     ```bash
     curl http://localhost:3000/api/models
     ```

   - Chat completions:

     ```bash
     curl -X POST http://localhost:3000/api/chat/completions \
       -H "Content-Type: application/json" \
       -d '{"model":"gpt-4.1","messages":[{"role":"user","content":"Hello"}]}'
     ```

---

## Contributing

- Keep changes small, focused, and backward compatible where possible.
- Maintain OpenAI-compatible request/response semantics.
- Coordinate changes to behavior with updates to this `README.md` and `agents.md`.

For AI agents working on this repo, see `agents.md`.
