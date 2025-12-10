# AI Agent Guidelines for `open-ai-router-v2`

This file is for AI coding agents (e.g. Copilot Chat, GPT-based tools) that work on this repository.

---

## 1. Project Overview (for agents)

- This is a **Go** project that builds a **custom Caddy binary** with additional HTTP handler modules for routing AI/LLM traffic.
- Main entrypoint is `src/main.go`, which imports:
  - `src/modules` – Caddy modules (`ai_auth_env`, `ai_router`, `ai_list_models`, `ai_chat_completions`).
  - `src/drivers/openai` – provider-specific HTTP clients for OpenAI-compatible APIs.
  - `src/modules/chat_completions_plugins` – plugins applied around chat completions (e.g. `posthog`, `models`, `fuzz`, `zip` variants).
- Example configuration is in `Caddyfile.example`.
- Build command (see `Makefile`): `go build -o caddy ./src`.

When writing code or docs, **preserve compatibility** with existing Caddy module IDs and directive names.

---

## 2. Coding Rules for Agents

When you modify code in this repo, follow these rules:

1. **Use the VS Code tools API**
   - Use `apply_patch` to edit existing files.
   - Use `create_file` for new files.
   - Do not run raw shell commands if a dedicated tool exists for that task.

2. **Respect existing architecture**
   - Do not rename public module IDs (e.g. `http.handlers.ai_router`, `http.handlers.ai_chat_completions`) or Caddy directives (`ai_auth_env`, `ai_router`, `ai_list_models`, `ai_chat_completions`) without explicit user approval.
   - Keep provider wiring via `services.ProviderImpl` and `RouterModule` intact unless the user asks for a refactor.
   - `chat_completions` plugins must continue to implement `ChatCompletionsPlugin` from `chat_completions_plugins/plugin.go`.

3. **Keep changes minimal and scoped**
   - Fix the **root cause** of the issue described by the user, but avoid opportunistic large refactors.
   - Match existing code style (naming, logging via `zap`, error handling strategy).
   - Avoid introducing new external dependencies unless requested or clearly justified.

4. **Safety and behavior**
   - Do not remove or weaken observability and auth controls (`EnvAuthManagerModule`, PostHog instrumentation) unless explicitly asked.
   - When changing request/response shapes, be careful not to break OpenAI-compatible semantics.

5. **Testing and validation**
   - When you touch runnable code (Go files), prefer to:
     - At least build the project: `go build ./src/...` or `make build`.
     - If tests exist in the future, run targeted tests related to changed packages.
   - If you cannot run tests in this environment, mention that to the user and explain how they can run them locally.

---

## 3. Documentation Rules for Agents

When you add or change behavior that affects how humans use this project (configuration, build, APIs, plugins, environment variables):

- Update or propose updates to **`README.md`** to reflect:
  - New/changed features.
  - New/changed environment variables.
  - New/changed Caddy directives or example configuration.
- If you change workflows relevant to agents (how they should behave), update or propose updates to **`agents.md`**.

Keep documentation:

- Concise and task-focused.
- Accurate with respect to current code and examples (especially `Caddyfile.example`).

---

## 4. Required Post-Change Prompt to the User

After you complete **any non-trivial coding change** (Go source, Caddyfile semantics, or plugin logic), you **must explicitly ask the user** something like:

> "I’ve updated the code. Do you want me to also refresh `agents.md` and `README.md` to match these changes?"

Then:

- If the user says **yes**:
  - Summarize the behavior changes in 2–4 bullets.
  - Propose concrete edits to `README.md` and/or `agents.md` and apply them after confirmation (if the user wants to see a preview first, show a short diff or snippet instead of full files).
- If the user says **no**:
  - Continue with their requested task, but avoid silently contradicting existing docs.

This rule is **mandatory** so that human-facing docs and AI-facing guidelines stay aligned with the actual behavior of the router.

---

## 5. When Unsure

If repo intent or configuration is ambiguous:

- Prefer **asking a brief clarifying question** over guessing critical behavior (auth, routing, provider selection).
- When guessing is acceptable (e.g. log message wording, variable naming), keep guesses conservative and reversible.

End of file.
