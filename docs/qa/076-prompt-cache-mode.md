# QA — SPEC 076 Prompt cache_control + persisted prompt_mode

Date: 2026-08-29. Clean-room Go/React. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not merge. Do not start SPEC 077.

Closes matrix **Q2, Q3**. R1 leftover stream stays SPEC 068; this spec does not re-split SSE.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Four prompt modes (full / task / minimal / none) with section gating | `/Users/mqglobal/Documents/goclaw/goclaw-source/README.md` (Core Features — 4-Mode Prompt System); `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/01-agent-loop.md` (system prompt mode) |
| Anthropic native HTTP+SSE with prompt caching | `/Users/mqglobal/Documents/goclaw/goclaw-source/README.md` (20+ LLM providers — Anthropic native HTTP+SSE with prompt caching); `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/02-providers.md` (Anthropic native / CacheMiddleware / CacheReadTokens) |

goso mapping (self-written): `sessions.prompt_mode` TEXT, default empty = full. `POST /api/chat` uses request `prompt_mode` if set; else the session row; else full. Unknown → 400. `PATCH /api/sessions/{id}` `{prompt_mode}` persists the canonical mode. Anthropic `CacheMode=full` or `GOSO_PROMPT_CACHE=full` attaches `cache_control: {type: ephemeral}` to stable prefix blocks listed below; other values omit the field and must not crash. OpenAI-compat payloads never gain a fake cache field.

## Stable prefix (`cache_control` list)

When Anthropic cache is on, the Messages JSON attaches `cache_control` to:

1. **System prompt block** — identity / instructions / tool notes / safety (mode-gated), plus team note, `TEAM.md`, and agent `instructions`.
2. **Bootstrap files block** — labeled `SOUL.md` / `IDENTITY.md` / `AGENTS.md` / `USER.md` from `GOSO_CONTEXT_DIR` (own system content block; omitted when missing or `prompt_mode=none`).
3. **Last non-user message** in the `messages` array (assistant). The current user turn is not cached.

Rolling `Previous summary:` system text is **not** a cache breakpoint. Other CacheMode values omit `cache_control` entirely. Empty / unset cache env is off. `GOSO_ANTHROPIC_CACHE_MODE=full` applies to both env-registry and sqlite-built Anthropic (same as `GOSO_PROMPT_CACHE=full`).

## What changed

- Persist `prompt_mode` on memory + SQLite sessions (`ALTER` + CREATE default empty).
- `POST /api/chat` request-if-set else session else full. `PATCH /api/sessions/{id}` `{prompt_mode}`; unknown 400; missing session 404.
- Anthropic stable-prefix `cache_control` as above. `GOSO_ANTHROPIC_CACHE_MODE=full` or `GOSO_PROMPT_CACHE=full`.
- Control-plane: Chat chrome + Sessions table select full/task/minimal/none; PATCH persist; i18n vi+en.
- 068 SSE path unchanged (`handleChatRuntime` stream still `ChatOptsStream` + `data: [DONE]`).

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 077.

## Proof

- Session round-trip (memory + SQLite reopen): `TestStore_SessionAndMessage`, `TestSQLiteStore_PromptModeRoundTrip`.
- PATCH persist + unknown 400: `TestPatchSession_PromptMode`.
- Chat without field uses stored mode; request overrides: `TestChat_UsesStoredPromptMode` (httpapi + agent).
- Anthropic httptest: cache_control in full (system + bootstrap + last non-user), omitted in none/bogus; `GOSO_PROMPT_CACHE=full` enables it (`TestAnthropic_CacheControlFull`, `TestAnthropic_PromptCacheEnvFull`).
- OpenAI-compat does not fake cache (`TestOpenAI_NoFakeCacheControl`).
- Unknown `POST /api/chat` `prompt_mode` still 400 (`TestChat_PromptModeUnknown400`).

## Non-goals

pgvector. Changing 068 SSE. Copying goclaw. Merge. SPEC 077.
