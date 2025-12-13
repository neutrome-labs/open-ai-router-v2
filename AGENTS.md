# AI Agents Guide

This document provides architectural overview and guidelines for AI agents working on the open-ai-router codebase.

## Architecture Overview

### Core Concepts

**Open AI Router v3** is a Caddy plugin that routes LLM API requests to multiple providers with format conversion and plugin support.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Caddy Server                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│  Input Handlers (Modules)                                                    │
│  ┌─────────────────┐ ┌─────────────────┐ ┌──────────────────────┐           │
│  │ ai_openai_chat  │ │ ai_openai_      │ │ ai_anthropic_        │           │
│  │ _completions    │ │ responses       │ │ messages             │           │
│  └────────┬────────┘ └────────┬────────┘ └──────────┬───────────┘           │
│           │                   │                      │                       │
│           └───────────────────┼──────────────────────┘                       │
│                               ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │                        Managed Formats                                   ││
│  │  Parse JSON once → ManagedRequest → Process → ManagedResponse → JSON    ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                               │                                              │
│                               ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │                         Plugin Chain                                     ││
│  │  Before → [Provider Call] → After (non-stream) / AfterChunk + StreamEnd ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                               │                                              │
│                               ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │                      Style Converter                                     ││
│  │  Passthrough if input style == provider style, else convert             ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                               │                                              │
│                               ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │                          Drivers                                         ││
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐                 ││
│  │  │  OpenAI  │  │ Anthropic│  │  Google  │  │Cloudflare│                 ││
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────┘                 ││
│  └─────────────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────────────┘
```

### Package Structure

```
src/
├── main.go              # Caddy entry point
├── formats/             # Managed request/response structures
│   ├── format.go        # Core interfaces (ManagedRequest, ManagedResponse, etc.)
│   ├── openai_chat.go   # OpenAI Chat Completions format
│   ├── openai_responses.go  # OpenAI Responses API format
│   └── anthropic.go     # Anthropic Messages format
├── styles/              # Style definitions and converters
│   ├── style.go         # Style constants and utilities
│   └── converter.go     # Format conversion between styles
├── services/            # Shared services and implementations
│   ├── auth_manager.go  # Auth manager interface and registry
│   ├── router_impl.go   # Router runtime implementation
│   ├── provider_impl.go # Provider runtime implementation
│   └── posthog.go       # PostHog observability
├── plugins/             # Request/response plugins
│   ├── plugin.go        # Plugin interfaces (Before, After, StreamChunk, StreamEnd)
│   ├── registry.go      # Plugin registry
│   ├── posthog.go       # PostHog plugin (with stream accumulation)
│   ├── models.go        # Model mapping plugin
│   ├── fuzz.go          # Fuzzy model matching plugin
│   └── zip.go           # Auto-compaction plugin (zip, zipc, zips, zipsc variants)
├── drivers/             # Provider-specific API drivers
│   ├── interfaces.go    # Driver interfaces
│   ├── openai/          # OpenAI driver
│   └── anthropic/       # Anthropic driver
├── modules/             # Caddy HTTP handler modules
│   ├── init.go          # Module registration
│   ├── env_auth_manager.go  # ai_auth_env directive
│   ├── router.go        # ai_router directive
│   ├── list_models.go   # ai_list_models directive
│   ├── openai_chat_completions.go   # ai_openai_chat_completions
│   ├── openai_responses.go          # ai_openai_responses
│   └── anthropic_messages.go        # ai_anthropic_messages
└── sse/                 # Server-Sent Events utilities
    ├── reader.go        # SSE event parser
    └── writer.go        # SSE event writer
```

### Key Design Principles (v3)

1. **Single Serialization**: JSON is parsed once at input, processed as Go structs, serialized once at output. The `extras` map preserves unknown fields for passthrough.

2. **Style Passthrough**: When input style matches provider style, requests pass through with minimal modification. Conversion only happens when styles differ.

3. **Per-Provider Plugin Execution**: Plugins are called separately for each provider attempt with an isolated (cloned) copy of the request. This ensures:
   - Each provider gets a fresh request that plugins can modify independently
   - Plugin modifications for one provider don't affect subsequent provider attempts
   - Provider context is available to plugins during the `Before` hook

4. **Separated Plugin Hooks**:
   - `Before`: Runs before provider call (modify request) - called per-provider with cloned request
   - `After`: Runs after non-streaming response
   - `AfterChunk`: Runs for each streaming chunk
   - `StreamEnd`: Runs when stream completes (for finalization when no usage data in stream)

4. **Provider Styles**: Each provider has a style (openai-chat-completions, openai-responses, anthropic-messages, google-genai, cloudflare). The converter handles transformations between styles.

## Supported Styles

| Style | Description | Endpoint |
|-------|-------------|----------|
| `openai-chat-completions` | OpenAI Chat Completions API (default) | `/v1/chat/completions` |
| `openai-responses` | OpenAI Responses API | `/v1/responses` |
| `anthropic-messages` | Anthropic Messages API | `/v1/messages` |
| `google-genai` | Google Generative AI |  (soon) |
| `cloudflare-ai-gateway` | Cloudflare AI Gateway |  (soon) |
| `cloudflare-workers-ai` | Cloudflare Workers AI |  (soon) |

## Adding New Features

### Adding a New Provider Style

1. Create format structs in `src/formats/` (implement `ManagedRequest` and `ManagedResponse` interfaces)
2. Add style constant in `src/styles/style.go`
3. Implement conversion methods in `src/styles/converter.go`
4. Create driver in `src/drivers/<provider>/`
5. Create handler module in `src/modules/`
6. Register in `src/modules/init.go`

### Adding a New Plugin

1. Create plugin file in `src/plugins/`
2. Implement required interfaces (`BeforePlugin`, `AfterPlugin`, `StreamChunkPlugin`, `StreamEndPlugin`)
3. Register in `src/plugins/registry.go`

### Adding a New Driver

1. Create package under `src/drivers/<provider>/`
2. Implement relevant command interfaces from `src/drivers/interfaces.go`
3. Wire into the appropriate module handler

## Important Interfaces

### ManagedRequest (formats/format.go)
```go
type ManagedRequest interface {
    GetModel() string
    SetModel(model string)
    GetMessages() []Message
    SetMessages(messages []Message)
    IsStreaming() bool
    GetRawExtras() map[string]json.RawMessage
    SetRawExtras(extras map[string]json.RawMessage)
    MergeFrom(raw []byte) error
    Clone() ManagedRequest // Deep copy for isolated plugin processing
    ToJSON() ([]byte, error)
}
```

### ManagedResponse (formats/format.go)
```go
type ManagedResponse interface {
    GetModel() string
    GetUsage() *Usage
    SetUsage(usage *Usage)
    GetChoices() []Choice
    IsChunk() bool
    GetRawExtras() map[string]json.RawMessage
    ToJSON() ([]byte, error)
}
```

### Plugin Interfaces (plugins/plugin.go)
```go
type BeforePlugin interface {
    // Before is called before the request is sent to each provider (with cloned request)
    // p contains the provider context, allowing plugins to make provider-specific modifications
    Before(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest) (formats.ManagedRequest, error)
}

type AfterPlugin interface {
    After(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, resp formats.ManagedResponse) (formats.ManagedResponse, error)
}

type StreamChunkPlugin interface {
    AfterChunk(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, chunk formats.ManagedResponse) (formats.ManagedResponse, error)
}

type StreamEndPlugin interface {
    StreamEnd(params string, p *services.ProviderImpl, r *http.Request, req formats.ManagedRequest, hres *http.Response, lastChunk formats.ManagedResponse) error
}
```

## Documentation Maintenance

When making changes to this codebase:

1. **Update README.md** if you change:
   - Caddyfile directives or their options
   - Environment variables
   - Build/run instructions
   - Public-facing features

2. **Update agents.md** if you change:
   - Package structure
   - Core interfaces
   - Architecture patterns
   - Adding new styles/drivers/plugins procedures

Always keep both documents in sync with the actual implementation.
