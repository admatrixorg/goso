# QA — SPEC 039 LLM providers (named compat, no product keys)

Date: 2026-08-27. Clean-room. Closes matrix rows **R0–R14, R16** as **adapters + tests**. Live keys = **DI-20** — not in git; `.env.example` placeholders empty. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. No goclaw copy. Tests use `httptest` fake servers, not paid APIs.

## What changed

Registry constructs a provider only when its env key is **non-empty**. `echo` is always present. Native `anthropic` / `openai` stay. Named OpenAI-compat reuse `openai.go` `BaseURL` + `Label`:

| Name | Env key | Default BaseURL |
|------|---------|-----------------|
| openrouter | `GOSO_OPENROUTER_API_KEY` | `https://openrouter.ai/api` |
| groq | `GOSO_GROQ_API_KEY` | `https://api.groq.com/openai` |
| deepseek | `GOSO_DEEPSEEK_API_KEY` | `https://api.deepseek.com` |
| gemini | `GOSO_GEMINI_API_KEY` | `https://generativelanguage.googleapis.com/v1beta/openai` |
| mistral | `GOSO_MISTRAL_API_KEY` | `https://api.mistral.ai` |
| xai | `GOSO_XAI_API_KEY` | `https://api.x.ai` |
| minimax | `GOSO_MINIMAX_API_KEY` | `https://api.minimax.io` |
| dashscope | `GOSO_DASHSCOPE_API_KEY` | `https://dashscope.aliyuncs.com/compatible-mode` |

Default chat provider: anthropic, else openai, else first configured named compat, else echo.

`GET /api/providers` returns `{providers:[…]}` configured names only (never secrets). Empty env → `["echo"]`.

OpenAI-compat `ChatTools` parses `tool_calls`; fake server tests return one tool_use JSON then text.

Claude CLI / Codex / ACP: interfaces + stubs that fail closed (`not_configured`). No bundled CLIs. Not registered from env.

SSE: `ParseSSE` / `ReadOpenAIStream` against httptest `text/event-stream`. Production path used only when `OpenAI.Stream=true`. Default chat stays non-stream.

Prompt cache: Anthropic `CacheMode=full` includes `cache_control` on system/last message JSON; other values ignored (must not crash).

## Commands

```
go test ./...
gofmt -l gateway
go vet ./gateway/...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
rg -n 'sk-[A-Za-z0-9]' gateway || true
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- Empty env → only echo; groq key → groq present + preferred (`TestRegistry`, `TestRegistry_NamedCompat`).
- Catalog names R3–R10 (`TestOpenAICompatCatalog`).
- Named Label + ChatTools tool_use then text (`TestOpenAI_NamedLabelAndChatTools`).
- `GET /api/providers` names only (`TestProvidersAPI_ConfiguredNamesOnly`, `TestProvidersListsConfigured`).
- SSE httptest (`TestParseSSE_OpenAICompat`, `TestOpenAI_StreamHttptest`).
- Stubs fail closed, not in empty registry (`TestStubs_FailClosed`, `TestStubs_NotInEmptyRegistry`).
- Anthropic `cache_control` when mode=full (`TestAnthropic_CacheControlFull`).

## Non-goals

Live paid API calls, product keys in git, bundled Claude CLI / Codex / ACP binaries, MCP `/v1/providers` CRUD (gateway list is `/api/providers`), extra vendors beyond the named catalog.
