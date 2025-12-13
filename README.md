# Open AI Router

A Caddy-based HTTP router for LLM providers with multi-format support and plugins.

## Overview

Open AI Router is a Caddy plugins pack that provides handler directives for routing requests to multiple LLM providers. It supports:

- **Multiple input styles**: OpenAI Chat Completions, OpenAI Responses API, Anthropic Messages (and more soon)
- **Multiple output styles**: Route to any provider regardless of it's input format
- **Automatic format conversion**: Converts between styles when needed
- **Passthrough optimization**: No conversion when input/output styles match
- **Plugin system**: Extensible request/response processing

## Quick Start

### Build

```bash
make build
```

### Configure

Create a `Caddyfile` (see `Caddyfile.example`):

### Run

```bash
./caddy run --config Caddyfile
```

## Directives

### `ai_auth_env`

Registers an environment-based auth manager.

```caddyfile
ai_auth_env {
    name default
}
```

For each provider `X`, reads API key from environment variable `${X}_API_KEY` (uppercased).
Eg. `OPENAI_API_KEY` for `provider openai {...}` provider or `MY-CUSTOM-PROVIDER_API_KEY` for `provider my-custom-provider {...}` in a router config.

### `ai_router`

Defines providers and routing rules.

```caddyfile
ai_router {
    name default
    auth default

    provider <name> {
        api_base_url "<url>"
        style "<style>"  # optional, default: openai-chat-completions
    }
}
```

**Supported styles:**
- `openai-chat-completions` (default)
- `openai-responses`
- `anthropic-messages`
- `google-genai` (planned)
- `cloudflare` (planned)

### `ai_openai_chat_completions`

Handles `/chat/completions` requests in OpenAI format.

```caddyfile
ai_openai_chat_completions {
    router default
}
```

### `ai_openai_responses`

Handles `/responses` requests in OpenAI Responses API format.

```caddyfile
ai_openai_responses {
    router default
}
```

### `ai_anthropic_messages`

Handles `/messages` requests in Anthropic format.

```caddyfile
ai_anthropic_messages {
    router default
}
```

### `ai_list_models` / `ai_list_models`

Aggregates models from all configured providers.

```caddyfile
ai_list_models {
    router default
}
```

## Model Routing

1. **Explicit provider**: `model="provider/model"` routes to that provider
2. **Default mapping**: Configure per-model defaults (if implemented)
3. **Fallback**: Uses providers in definition order

## Plugins

Plugins process requests before/after provider calls.

**Built-in plugins:**
- `posthog` - Observability events (with stream chunk accumulation)
- `models` - AI Council (multiple models in parallel)
- `fuzz` - Fuzzy model name matching
- `zip` - Auto-compact long conversations (with caching)
- `zipc` - Like `zip`, but preserve the first user message
- `zips` - Like `zip`, but disable cache (recompact each time)
- `zipsc` - Preserve first user message and disable cache

The zip family takes an optional numeric parameter (approx max tokens):
- `zip:65535`, `zipc:100000`, `zips:1999999`, `zipsc:32000`

**Plugin activation:**
- URL path: `/v1/chat/completions/fuzz/...`
- Model suffix: `model="gpt-4+fuzz"`

## Environment Variables

| Variable | Description |
|----------|-------------|
| `<PROVIDER>_API_KEY` | API key for provider (e.g., `OPENROUTER_API_KEY`) |
| `POSTHOG_API_KEY` | Enable PostHog observability |
| `POSTHOG_BASE_URL` | PostHog endpoint (optional) |
| `POSTHOG_INCLUDE_CONTENT` | If `true`, include message content in observability events |

## Response Headers

| Header | Description |
|--------|-------------|
| `X-Real-Provider-Id` | Provider used for the request |
| `X-Real-Model-Id` | Final model name after mapping |
| `X-Plugins-Executed` | Plugins executed for this request |

## Development

```bash
make build   # Build binary
make run     # Build and run with Caddyfile
make tidy    # Run go mod tidy
make clean   # Remove binary
```

For architecture details and contribution guidelines, see [agents.md](agents.md).
