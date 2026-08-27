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

Unit tests do **not** call live `127.0.0.1:20127`. Router native `search:true` is a **model capability**, not a GOSO HTTP search API.

## Live smoke (2026-08-27) — BLOCKED by 401, not a success

Coordinator + user probes against 9Router `http://127.0.0.1:20127`. **Do not treat this SPEC as a live-chat green.**

| Probe | Result |
|-------|--------|
| `GET /v1/models` (no auth) | **HTTP 200**. 35 models including `cx/gpt-5.6-sol` and `cx/gpt-5.6-sol-review` (400K ctx, search+vision+tools). |
| `POST /v1/chat/completions` model `cx/gpt-5.6-sol` | **HTTP 401**. Body: `[codex/gpt-5.6-sol] token_expired` / `"Provided authentication token is expired. Please try signing in again."` |
| User long probe | Same 401 with Bearer = Codex `id_token` from `~/.codex/auth.json` (file mtime **2026-08-24**, token stale). `/v1/models` does not need that token; **chat does**. |
| Gateway pipeline `POST /api/chat` with `GOSO_ROUTER9_MODEL=cx/gpt-5.6-sol` | **HTTP 502**, error `router9 401: … token_expired`. Log: `provider=router9 model=cx/gpt-5.6-sol`. Evidence: `/tmp/goso-044-demo/chat-sol-rerun.json`, `sol-direct.json`, `models.json`. |

**Fix (user):** `codex login` (or equivalent) to refresh `~/.codex/auth.json`, then re-run `POST /v1/chat/completions` and `POST :18080/api/chat`. Coordinator will not wait mid-SPEC.

This document **does not** claim a successful `cx/gpt-5.6-sol` completion. Any other 9Router model that happened to answer is **not** the locked live-smoke target.

## Functions / tools evidence (independent of 401)

Live demo (no Codex):

```
GET http://127.0.0.1:3000/api/providers          → 200 {"providers":["echo","router9"]}
GET http://127.0.0.1:3000/api/agents/20260827-1/tools → 200
  web_search/sandbox/browser/media  connector=builtin  enabled=false
GET http://127.0.0.1:3000/                       → 200
```

Unit tests (httptest, no 20127): `gateway/internal/httpapi/handlers_tools_test.go` covers GET/PATCH `/api/agents/{id}/tools` and connector PATCH; `gateway/internal/builtin/builtin_test.go` covers fail-closed + fake DDG. Connector tools (zalocrm-style) stay on existing `handlers_connector_test.go` fake HTTP server.

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
| Live `cx/gpt-5.6-sol` chat | **Blocked.** 401 `token_expired` (Codex `~/.codex/auth.json` dated 2026-08-24). Fix = user `codex login`, then re-smoke. Not a product bug. |
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
