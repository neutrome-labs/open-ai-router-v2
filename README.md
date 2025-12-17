# Open AI Router (Gateway)

# Why another gateway

- Caddy-based -> fast, confugurable, reliable;
- Go-based -> exteremely fast, error-resilient;
- Built-in fallback over providers, eg. `if OpenAI returns 4xx/5xx, transparently retry with Openrouter`;
- [Plugins](#plugins) -> programable, including:
    - Fallback over models, eg. `if GPT-5 is not available, transparently retry with GPT-4.1`;
    - Autocompact (infinite context) -> summarize messages to fit into model limits;
    - Strip completed tools data to save tokens;

# Philosophy

# Features Map

Style                   | Server | Client
------------------------|--------|--------
OpenAI Chat Completions | Full   | Full 
OpenAI Responses        | None   | None
Anthropic Messages      | None   | None
Google GenAI            | None   | None
Google Responses        | None   | None
Cloudflare Workers AI   | None   | None
Cloudflare AI Gateway   | None   | None

# Plugins

## posthog

## models

## parallel

### select

## fuzz

## zip

## stools