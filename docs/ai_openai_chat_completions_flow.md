# OpenAI Chat Completions Data Flow

This document describes how HTTP requests and responses (headers and bodies) flow through the `ai_openai_chat_completions` Caddy module.

## High-Level Architecture

```mermaid
flowchart TB
    subgraph Client
        REQ[HTTP Request]
        RES[HTTP Response]
    end
    
    subgraph Caddy["Caddy Server"]
        subgraph Module["OpenAIChatCompletionsModule"]
            SERVE[ServeHTTP]
            HANDLE[handleRequest]
            CHAT[serveChatCompletions]
            STREAM[serveChatCompletionsStream]
        end
        
        subgraph Services
            ROUTER[RouterModule]
            AUTH[AuthService]
            PROVIDER[ProviderService]
        end
        
        subgraph Plugins["Plugin Chain"]
            BEFORE[RunBefore]
            AFTER[RunAfter]
            CHUNK[RunAfterChunk]
            STREAMEND[RunStreamEnd]
            ERROR[RunError]
            RECURSIVE[RunRecursiveHandlers]
        end
        
        subgraph Drivers["Driver Layer"]
            CMD[InferenceCommand]
            CONV[StyleConverter]
        end
    end
    
    subgraph Provider["AI Provider"]
        API[Provider API]
    end
    
    REQ --> SERVE
    SERVE --> AUTH
    SERVE --> ROUTER
    SERVE --> RECURSIVE
    RECURSIVE --> HANDLE
    
    HANDLE --> BEFORE
    BEFORE --> CHAT
    BEFORE --> STREAM
    
    CHAT --> CONV
    CHAT --> CMD
    CHAT --> AFTER
    
    STREAM --> CONV
    STREAM --> CMD
    STREAM --> CHUNK
    STREAM --> STREAMEND
    
    CMD --> API
    API --> CMD
    
    AFTER --> RES
    CHUNK --> RES
    STREAMEND --> RES
    ERROR --> RES
```

## Detailed Request Flow

### 1. Entry Point - ServeHTTP

```mermaid
sequenceDiagram
    participant Client
    participant ServeHTTP as ServeHTTP
    participant Router as RouterModule
    participant Auth as AuthService
    participant Plugins as PluginChain
    participant HandleReq as handleRequest
    
    Client->>ServeHTTP: HTTP POST /chat/completions
    Note over ServeHTTP: Read request body (io.ReadAll)
    Note over ServeHTTP: Parse known fields (stream, model)
    
    ServeHTTP->>Router: GetRouter(routerName)
    Router-->>ServeHTTP: RouterModule
    
    ServeHTTP->>Auth: CollectIncomingAuth(request)
    Note over Auth: Extract auth from headers<br/>Set context values (user_id, key_id)
    Auth-->>ServeHTTP: Modified request with context
    
    ServeHTTP->>Plugins: TryResolvePlugins(url, model)
    Note over Plugins: Parse URL path plugins<br/>Parse model suffix plugins<br/>Add head/tail plugins
    Plugins-->>ServeHTTP: PluginChain
    
    Note over ServeHTTP: Generate trace_id (UUID)<br/>Add to request context
    
    ServeHTTP->>Plugins: RunRecursiveHandlers()
    alt Plugin handles request
        Plugins-->>Client: Plugin-generated response
    else No recursive handler
        ServeHTTP->>HandleReq: handleRequest()
        HandleReq-->>Client: Provider response
    end
```

### 2. Request Body Processing

```mermaid
flowchart LR
    subgraph "Request Body Flow"
        direction TB
        RAW["Raw HTTP Body<br/>[]byte"]
        PARSE["Parse Known Fields<br/>stream, model"]
        CLONE["Clone for Provider<br/>[]byte copy"]
        BEFORE["RunBefore Plugins<br/>Modify request"]
        CONVERT["Style Converter<br/>Format transformation"]
        PROVIDER["Provider Request<br/>Native format"]
    end
    
    RAW --> PARSE
    PARSE --> CLONE
    CLONE --> BEFORE
    BEFORE --> CONVERT
    CONVERT --> PROVIDER
```

### 3. Provider Resolution

```mermaid
flowchart TB
    subgraph "Provider Resolution"
        MODEL["Input Model String<br/>e.g. 'provider/gpt-4+plugin:arg'"]
        
        subgraph Parse["Parse Model"]
            STRIP["Strip Plugin Suffix<br/>'gpt-4'"]
            PREFIX["Check Provider Prefix<br/>'provider/gpt-4' -> 'gpt-4'"]
        end
        
        subgraph Lookup["Provider Lookup"]
            EXPLICIT[Explicit Provider<br/>from prefix]
            DEFAULT[Default Provider<br/>for model]
            ORDER[Providers Order<br/>fallback list]
        end
        
        PROVIDERS[Ordered Provider List]
    end
    
    MODEL --> STRIP
    STRIP --> PREFIX
    PREFIX --> EXPLICIT
    EXPLICIT --> DEFAULT
    DEFAULT --> ORDER
    EXPLICIT --> PROVIDERS
    DEFAULT --> PROVIDERS
    ORDER --> PROVIDERS
```

## Non-Streaming Flow

```mermaid
sequenceDiagram
    participant Handle as handleRequest
    participant Plugins as PluginChain
    participant Serve as serveChatCompletions
    participant Converter as StyleConverter
    participant Driver as ChatCompletions
    participant Provider as AI Provider
    participant Writer as ResponseWriter
    
    Handle->>Handle: ResolveProvidersOrderAndModel()
    
    loop For each provider
        Handle->>Plugins: RunBefore(provider, request)
        Note over Plugins: Logger, Models, Custom plugins
        Plugins-->>Handle: Modified request body
        
        Handle->>Serve: serveChatCompletions()
        
        Serve->>Converter: ConvertRequest(body, OpenAIChat, providerStyle)
        Note over Converter: Passthrough if same style<br/>Transform if different
        Converter-->>Serve: Provider-format request
        
        Serve->>Driver: DoInference(provider, body, request)
        
        Driver->>Driver: createRequest()
        Note over Driver: Clone headers<br/>Set Content-Type: application/json<br/>Set Authorization header
        
        Driver->>Provider: HTTP POST /chat/completions
        Provider-->>Driver: HTTP Response + Body
        
        alt Success (200 OK)
            Driver-->>Serve: (response, body, nil)
            
            Serve->>Converter: ConvertResponse(body, providerStyle, OpenAIChat)
            Converter-->>Serve: OpenAI-format response
            
            Serve->>Plugins: RunAfter(provider, request, response)
            Plugins-->>Serve: Modified response body
            
            Serve->>Writer: Write JSON response
            Note over Writer: Content-Type: application/json
            
        else Error
            Driver-->>Serve: (response, nil, error)
            Serve->>Plugins: RunError(provider, request, error)
            Note over Handle: Try next provider
        end
    end
    
    Handle->>Writer: Set X-Real-Provider-Id header
    Handle->>Writer: Set X-Real-Model-Id header
    Handle->>Writer: Set X-Plugins-Executed header
```

## Streaming Flow

```mermaid
sequenceDiagram
    participant Handle as handleRequest
    participant Plugins as PluginChain
    participant Stream as serveChatCompletionsStream
    participant SSEWriter as SSE Writer
    participant Converter as StyleConverter
    participant Driver as ChatCompletions
    participant Provider as AI Provider
    participant SSEReader as SSE Reader
    
    Handle->>Stream: serveChatCompletionsStream()
    
    Stream->>SSEWriter: NewWriter(responseWriter)
    Note over SSEWriter: Set headers:<br/>Content-Type: text/event-stream<br/>Cache-Control: no-cache<br/>Connection: keep-alive<br/>X-Accel-Buffering: no
    
    Stream->>SSEWriter: WriteHeartbeat("ok")
    Note over SSEWriter: ":ok\n\n"
    
    Stream->>Converter: ConvertRequest(body, OpenAIChat, providerStyle)
    Converter-->>Stream: Provider-format request
    
    Stream->>Driver: DoInferenceStream(provider, body, request)
    
    Driver->>Driver: createRequest()
    Driver->>Provider: HTTP POST /chat/completions (stream=true)
    
    Provider-->>Driver: HTTP Response (streaming)
    
    Driver->>Driver: Create chunks channel
    
    Note over Driver: Goroutine starts
    
    Driver->>SSEReader: NewDefaultReader(response.Body)
    
    loop For each SSE event
        SSEReader->>Driver: Event{RawData, Done, Error}
        
        alt Done signal
            Driver->>Driver: Close channel
        else Error
            Driver->>Stream: InferenceStreamChunk{RuntimeError}
        else Data
            Driver->>Stream: InferenceStreamChunk{Data: rawJSON}
        end
    end
    
    loop For each chunk from channel
        alt RuntimeError
            Stream->>SSEWriter: WriteError(message)
            Stream->>Plugins: RunError()
        else Data chunk
            Stream->>Converter: ConvertResponseChunk(chunk, providerStyle, OpenAIChat)
            Converter-->>Stream: OpenAI-format chunk
            
            Stream->>Plugins: RunAfterChunk(chunk)
            Plugins-->>Stream: Modified chunk
            
            Stream->>SSEWriter: WriteRaw(chunk)
            Note over SSEWriter: "data: {...}\n\n"
        end
    end
    
    Stream->>Plugins: RunStreamEnd(lastChunk)
    Stream->>SSEWriter: WriteDone()
    Note over SSEWriter: "data: [DONE]\n\n"
```

## Header Flow Details

### Incoming Request Headers

```mermaid
flowchart LR
    subgraph "Incoming Headers"
        H1[Authorization<br/>Bearer token]
        H2[Content-Type<br/>application/json]
        H3["Accept<br/>*/*"]
        H4[Accept-Encoding<br/>gzip, deflate]
    end
    
    subgraph "Processing"
        AUTH[AuthService.CollectIncomingAuth]
        CLONE[Header.Clone]
    end
    
    subgraph "Outgoing to Provider"
        O1[Authorization<br/>Provider API key]
        O2[Content-Type<br/>application/json]
        O3["Accept-Encoding<br/>REMOVED"]
    end
    
    H1 --> AUTH
    AUTH --> |Extract user context| CLONE
    H2 --> CLONE
    H3 --> CLONE
    H4 --> CLONE
    CLONE --> |Set new auth| O1
    CLONE --> O2
    CLONE --> |Delete| O3
```

### Response Headers

```mermaid
flowchart TB
    subgraph "Non-Streaming Response"
        NS1["Content-Type: application/json"]
        NS2["X-Real-Provider-Id: provider-name"]
        NS3["X-Real-Model-Id: model-name"]
        NS4["X-Plugins-Executed: plugin1,plugin2"]
    end
    
    subgraph "Streaming Response"
        S1["Content-Type: text/event-stream"]
        S2["Cache-Control: no-cache, no-transform"]
        S3["Connection: keep-alive"]
        S4["X-Accel-Buffering: no"]
        S5["X-Real-Provider-Id: provider-name"]
        S6["X-Real-Model-Id: model-name"]
        S7["X-Plugins-Executed: plugin1,plugin2"]
    end
```

## Plugin Chain Execution Order

```mermaid
flowchart TB
    subgraph "Plugin Resolution"
        HEAD["Head Plugins<br/>models, parallel"]
        PATH["Path Plugins<br/>/plugin1:arg/plugin2"]
        MODEL["Model Plugins<br/>model+plugin:arg"]
        TAIL["Tail Plugins<br/>posthog"]
    end
    
    subgraph "Execution Phases"
        RECURSIVE[RecursiveHandler<br/>Can intercept entire flow]
        BEFORE[Before<br/>Modify request]
        INFERENCE[Inference<br/>Provider call]
        AFTER_NS[After<br/>Non-streaming response]
        AFTER_CHUNK[AfterChunk<br/>Each stream chunk]
        STREAM_END[StreamEnd<br/>Stream completion]
        ERROR[OnError<br/>Error handling]
    end
    
    HEAD --> PATH --> MODEL --> TAIL
    
    TAIL --> RECURSIVE
    RECURSIVE --> BEFORE
    BEFORE --> INFERENCE
    INFERENCE --> |Non-streaming| AFTER_NS
    INFERENCE --> |Streaming| AFTER_CHUNK
    AFTER_CHUNK --> STREAM_END
    INFERENCE --> |Error| ERROR
```

## Style Conversion

```mermaid
flowchart LR
    subgraph "Input Style"
        OPENAI_CHAT["OpenAI Chat<br/>/chat/completions"]
    end
    
    subgraph "Provider Styles"
        OPENAI["openai_chat<br/>Passthrough"]
        RESPONSES["openai_responses<br/>Convert format"]
    end
    
    subgraph "Conversion"
        REQ_CONV[ConvertRequest]
        RES_CONV[ConvertResponse]
        CHUNK_CONV[ConvertResponseChunk]
    end
    
    OPENAI_CHAT --> |Request| REQ_CONV
    REQ_CONV --> OPENAI
    REQ_CONV --> RESPONSES
    
    OPENAI --> |Response| RES_CONV
    RESPONSES --> |Response| RES_CONV
    RES_CONV --> OPENAI_CHAT
    
    OPENAI --> |Chunk| CHUNK_CONV
    RESPONSES --> |Chunk| CHUNK_CONV
    CHUNK_CONV --> OPENAI_CHAT
```

## Context Values

```mermaid
flowchart TB
    subgraph "Request Context"
        TRACE[trace_id<br/>UUID per request]
        USER[user_id<br/>From auth]
        KEY[key_id<br/>From auth]
    end
    
    subgraph "Set By"
        MODULE[OpenAIChatCompletionsModule<br/>Sets trace_id]
        AUTH[AuthService<br/>Sets user_id, key_id]
    end
    
    subgraph "Used By"
        PLUGINS[Plugins<br/>Logging, analytics]
        DRIVERS[Drivers<br/>Request context]
    end
    
    MODULE --> TRACE
    AUTH --> USER
    AUTH --> KEY
    
    TRACE --> PLUGINS
    USER --> PLUGINS
    KEY --> PLUGINS
    TRACE --> DRIVERS
```

## Error Handling Flow

```mermaid
flowchart TB
    subgraph "Error Sources"
        PARSE[Request Parse Error]
        AUTH_ERR[Auth Error]
        PLUGIN_ERR[Plugin Error]
        CONV_ERR[Conversion Error]
        PROVIDER_ERR[Provider Error]
        STREAM_ERR[Stream Runtime Error]
    end
    
    subgraph "Handling"
        HTTP_ERR[http.Error<br/>Return to client]
        NEXT_PROVIDER[Try Next Provider]
        PLUGIN_NOTIFY[RunError Plugins]
        SSE_ERR[SSE WriteError]
    end
    
    PARSE --> HTTP_ERR
    AUTH_ERR --> HTTP_ERR
    PLUGIN_ERR --> NEXT_PROVIDER
    CONV_ERR --> HTTP_ERR
    PROVIDER_ERR --> PLUGIN_NOTIFY --> NEXT_PROVIDER
    STREAM_ERR --> PLUGIN_NOTIFY --> SSE_ERR
```

## Data Types Summary

| Stage | Type | Description |
|-------|------|-------------|
| HTTP Request Body | `[]byte` | Raw JSON bytes from client |
| Parsed Request | `KnownOpenAIChatRequest` | Extracted stream/model fields |
| Plugin Processing | `[]byte` | JSON bytes, modifiable |
| Style Conversion | `[]byte` | Transformed JSON |
| Provider Request | `[]byte` | Native provider format |
| Provider Response | `[]byte` | Raw response bytes |
| Stream Chunk | `drivers.InferenceStreamChunk` | `{Data: []byte, RuntimeError: error}` |
| SSE Event | `sse.Event` | `{RawData: []byte, Done: bool, Error: error}` |
| HTTP Response | `[]byte` / SSE stream | Final client response |
