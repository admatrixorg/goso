# QA — SPEC 044 router9 + Functions page

Date: 2026-08-27. Clean-room. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. No product secrets in git.

## Wired

| Surface | Status |
|---------|--------|
| Named provider `router9` | Constructed when `GOSO_ROUTER9_BASE_URL` is non-empty. Default BaseURL `http://127.0.0.1:20127/v1`, model `cx/gpt-5.6-sol`. `GOSO_ROUTER9_API_KEY` may be empty (Authorization omitted). Catalog entry sits beside SPEC 039 names. |
| `GOSO_LLM_PROVIDER` | Overrides `Preferred()` when that name exists. Else prefer `router9` if constructed, else anthropic/openai/named/echo. |
| Chat URL join | BaseURL ending in `/v1` → `/chat/completions` only (no `/v1/v1`). groq/openrouter (no `/v1` suffix) still append `/v1/chat/completions`. |
| Timeout / parse | router9 client timeout ≥ 120s. `parseOpenAIChat` strips trailing `data: [DONE]`. |
| `GET /api/providers` | Lists configured names only (includes `router9` when constructed). No secrets. |
| Builtin tools | `web_search`, `sandbox`, `browser`, `media`. Default **OFF**. UI flags in SQLite `tool_flags` (and in-memory store). |
| `web_search` network | Only if UI flag on **and** `GOSO_WEB_SEARCH=ddg` or `1` (DuckDuckGo Instant Answer). Unconfigured → `not_configured`, no network. Tests use `httptest`, not live DDG. |
| sandbox / browser / media | Always `not_configured`. **Do not** exec, docker, Chrome, or ffmpeg. |
| `GET /api/agents/{id}/tools` | `{tools:[{name, connector, description, requires_approval, enabled}]}`. `runtime.ListTools` + builtins. **404** if agent missing. |
| `PATCH /api/agents/{id}/tools/{name}` | `{enabled:bool}`. Builtin → `tool_flags`. Connector-bound tool → `SetConnectorEnabled`. |
| `PATCH /api/connectors/{name}` | `{enabled?, endpoint?, token?}`. Token written to secret store; **never** returned. GET/list show `token_set: bool` and mask `credential_ref` when it looks like a secret. Empty token = unchanged. |
| Control-plane Functions | Live sidebar tab (not DEMO-only). Agent picker, tool table, connector URL + password token. i18n vi+en. |

Unit tests do **not** call live `127.0.0.1:20127`. Advisor probe: `GET /v1/models` was 200; `POST /v1/chat/completions` with `cx/gpt-5.6-sol` may return **401 Codex token expired**. This document does **not** claim a live sol success.

Router native `search:true` is a **model capability**, not a GOSO HTTP search API.

## Env (already in SETUP.md / `.env.example`)

```
GOSO_ROUTER9_BASE_URL=http://127.0.0.1:20127/v1
GOSO_ROUTER9_MODEL=cx/gpt-5.6-sol
GOSO_ROUTER9_API_KEY=
GOSO_LLM_PROVIDER=router9
GOSO_WEB_SEARCH=          # empty = fail-closed; ddg or 1 = Instant Answer
```

## Missing / parked

| Item | Status |
|------|--------|
| Live `cx/gpt-5.6-sol` chat | May 401 if upstream Codex session expired. Report real output; do not fake PONG. |
| Postgres / pgvector (DI-09) | Parked. SQLite + FTS5 remains default. |
| OAuth / Apple / Stripe / K8s / Grafana / Tailscale / Redis | Non-goals. |
| Channel live tokens | User supplies later; adapters already list `configured: false`. |
| Sandbox image / Chrome overlay / ffmpeg | Stubs only. |
| GET webhook list | Still does not exist (SPEC 043). |

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o /tmp/goso-gateway-044-qc ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD <sibling>/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Coordinator after merge: refresh `:18080` + `:3000`.
