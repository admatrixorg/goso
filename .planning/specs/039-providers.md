# SPEC 039 — LLM providers (named compat, no product keys)

> LOCKED: 2026-08-27. Closes **R0–R14, R16** as **adapters + tests**. Live keys = **DI-20** — do **not** put API keys in git, `.env.example` placeholders only (`changeme` / empty).
> SSE + `cache_control` types may be added; tests use `httptest` fake servers, not paid APIs.

## Goal

Registry lists named providers: `echo`, `anthropic`, `openai`, plus **OpenAI-compatible names** `openrouter`, `groq`, `deepseek`, `gemini`, `mistral`, `xai`, `minimax`, `dashscope` each with default BaseURL + env key name. Construct only when env key is **non-empty**. `GET /api/providers` returns configured names (never secrets).

Claude CLI / Codex / ACP: **interfaces + stub** that fail closed (`not_configured`) with tests; no bundled CLIs.

## HTTP

Existing Anthropic/OpenAI stay. OpenAI-compat: reuse `openai.go` `BaseURL`. Add `ChatTools` if the loop needs it — fake server in tests returns one tool_use JSON then text.

Optional SSE: parse `text/event-stream` in a test against httptest; production code path used only when `stream=true` (chat API may stay non-stream in 039 if timeboxed — then document PARTIAL SSE). Prefer implementing stream reader + test double.

Prompt cache: accept/ignore `cache_control` field on Anthropic request struct; test that JSON includes it when mode=full **or** skip and QA “deferred types, still DI/039 follow-up” — **must not crash**.

## Forbidden

Hardcoded `sk-`, `gsk_`, real tokens. Empty env → provider absent, echo fallback.

## QC

`go test ./...`, build, agpl 0, `docs/qa/039-providers.md`. Commit, do not merge.
