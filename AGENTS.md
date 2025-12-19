# AI Agent Instructions

You are an expert Go developer and Caddy module specialist working on the **Open AI Router**. This project is a high-performance, plugin-oriented AI gateway built on Caddy v2.

## Project Core Principles

1.  **Performance First**: Minimize serialization/deserialization. Use `[]byte` for data passthrough whenever possible.
2.  **Plugin-Oriented**: Almost all logic (fallback, parallel execution, logging, cost tracking) should be implemented as plugins.
3.  **Style Agnostic**: The core should handle requests in a unified way, with `drivers` and `styles` handling the specifics of different provider APIs.

## Architecture Reference

Before making changes, refer to the detailed data flow documentation:
- `docs/ai_chat_completions_flow.md`: Detailed Mermaid diagrams of how headers and bodies flow through the system.

## Critical Instructions for Agents

### 1. Documentation Maintenance
**Mandatory**: After every code modification, feature addition, or architectural change, you MUST:
- Update `README.md` if the change affects user-facing features, configuration, or plugin availability.
- Update relevant files in `docs/` (especially `docs/ai_chat_completions_flow.md`) to reflect changes in data flow, new plugin hooks, or modified interfaces.
- Ensure Mermaid diagrams in documentation remain syntactically valid (quote labels with special characters).

### 2. Plugin Development (V3)
When adding or modifying plugins:
- Implement the appropriate interfaces in `src/plugin/interfaces.go` (`BeforePlugin`, `AfterPlugin`, `StreamChunkPlugin`, `StreamEndPlugin`, `RecursiveHandlerPlugin`).
- Use `RecursiveHandlerPlugin` for logic that needs to control the request flow (like `models` for fallback or `parallel` for fan-out).
- Always consider both streaming and non-streaming paths.

### 3. Driver & Style Migration
- Follow the pattern in `src/drivers/openai/chat_completions.go` for new drivers.
- Use the `InferenceCommand` interface from `src/drivers/interfaces.go`.
- Ensure `src/services/converter.go` is updated when adding support for new API styles.

### 4. Testing
- Run `make test` before submitting changes.
- Add unit tests for new plugins in `src/plugins/` (e.g., `*_test.go`).

## Tech Stack
- **Language**: Go 1.21+
- **Framework**: Caddy v2 (HTTP Handler modules)
- **Logging**: `go.uber.org/zap`
- **Serialization**: `encoding/json` (prefer `[]byte` for passthrough)
- **SSE**: Custom implementation in `src/sse/`
