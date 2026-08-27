# SPEC 044 — router9 live provider + Functions page + DI close

> LOCKED: 2026-08-27. Clean-room. **No** copy from ZaloCRM or goclaw-source. No banned author ids.
> Demos **`:8082` / `:8091` must not be killed.** Worker must **not** bind/kill `:3000` `:18080` `:18088`. Coordinator refreshes `:18080` + `:3000` after merge.

User: (a) model AI = 9Router local `http://127.0.0.1:20127` OpenAI-compatible, **no key**; (b) remaining DIs = **self-audit self-decide**; (c) UI missing “function config” → Functions/tools page.

Advisor probe (2026-08-27): `GET http://127.0.0.1:20127/v1/models` → 200. Models include `cx/gpt-5.6-sol` (400K ctx, search+vision+tools) and `cx/gpt-5.6-sol-review`. Direct `POST /v1/chat/completions` with `cx/gpt-5.6-sol` returned **401 Codex token expired** at probe time; `ocg/glm-5.2` and `gcli/grok-4.6` returned real `PONG`. Worker must not fake a successful sol reply. Coordinator records **whatever the pipeline actually returns**.

## 1. Named provider `router9`

Catalog entry (alongside SPEC 039 OpenAI-compat names):

| Field | Value |
|-------|--------|
| Name | `router9` |
| Default BaseURL | `http://127.0.0.1:20127/v1` |
| Default model | `cx/gpt-5.6-sol` |
| Env URL | `GOSO_ROUTER9_BASE_URL` — **construct when this is set** (non-empty) |
| Env model | `GOSO_ROUTER9_MODEL` (empty → default) |
| Env key | `GOSO_ROUTER9_API_KEY` — **optional, may be empty** |
| Extra | `GOSO_LLM_PROVIDER=router9` forces Preferred() |

Do **not** require a non-empty key (unlike groq/openrouter). Empty key: omit `Authorization` or send `Bearer` empty; **do not** hit `missing API key`.

URL join: existing `OpenAI.complete` appends `/v1/chat/completions` to BaseURL. **Must not** produce `/v1/v1/chat/completions`. If BaseURL already ends with `/v1`, append `/chat/completions` only. Keep groq/openrouter (`BaseURL` without `/v1`) working.

Timeout: router9 client ≥ 120s (local router can be slow). Parse: tolerate trailing `data: [DONE]` after JSON (seen on 9Router glm).

`GET /api/providers` lists `router9` when constructed (name only, no secrets). Preferred order: `GOSO_LLM_PROVIDER` if that name exists, else `router9` if present, else existing anthropic/openai/named/echo.

Tests (`httptest`, no live 20127 in unit tests): construct on URL env + empty key; Chat 200 against fake `/v1/chat/completions`; absent URL env → router9 not in List(). Catalog test must include `router9` **and** keep empty-key skip for groq/etc.

## 2. Built-in tools (fail-closed)

| Tool | Default | When enabled | When off / missing config |
|------|---------|--------------|---------------------------|
| `web_search` | **OFF** | `GOSO_WEB_SEARCH=ddg` (or `1`): DuckDuckGo Instant Answer `https://api.duckduckgo.com/?q=&format=json&no_html=1&skip_disambig=1` via `httptest` in tests; live DDG only if env on. Router native `search:true` is a **model capability**, not a GOSO function — document, do not invent a 9Router search HTTP API. | Return `not_configured` (no network). |
| `sandbox` | **OFF** | never spawn; stay stub | `not_configured` — **do not exec/docker**. |
| `browser` | **OFF** | never spawn Chrome | `not_configured`. |
| `media` | **OFF** | never ffmpeg/download | `not_configured`. |

Advertise builtins in the tools list with `connector: "builtin"`, `enabled` from flags, `requires_approval: true` for sandbox/browser/media, `false` for web_search.

Persist UI toggles in SQLite `tool_flags(name PRIMARY KEY, enabled INTEGER)` (or equivalent). Env remains the **network** gate for web_search (enabled in UI but `GOSO_WEB_SEARCH` unset → still fail-closed, no DDG call).

## 3. HTTP (add only what is missing)

**Exists today:** `GET/POST /api/connectors`, `GET /api/connectors/{name}`, `GET/POST /api/agents/{id}/connectors`, `POST /api/tools/invoke`. `Runtime.ListTools` exists — **no** `GET /api/agents/{id}/tools`. `SetConnectorEnabled` exists — **no** PATCH HTTP. **No** UpdateConnector for endpoint/token.

Add:

| Method | Path | Body / response |
|--------|------|-----------------|
| GET | `/api/agents/{id}/tools` | `{tools:[{name, connector, description, requires_approval, enabled}]}` — `runtime.ListTools` + builtins. 404 if agent missing. |
| PATCH | `/api/agents/{id}/tools/{name}` | `{enabled: bool}` — builtin flag **or** if `name` is a connector-bound tool, toggle that connector via `SetConnectorEnabled`. |
| PATCH | `/api/connectors/{name}` | `{enabled?, endpoint?, token?}`. Token written to credential/secret store; **never** returned. GET connector/list: `token_set: bool`, mask `credential_ref` if it looks like a secret. Empty token field = leave unchanged. |

## 4. Control-plane Functions page

Clean-room React, match existing ZAgent pages (`Button`, `Card`, `EmptyState`, `SectionHeader`, `Icon`). Live sidebar (not DEMO-only): **Functions**. i18n **both** `vi.ts` and `en.ts` (`MsgKey`).

- Agent picker (`GET /api/agents`)
- Table: tool name, connector, approval badge, enable toggle → PATCH
- Connector config: endpoint + token input (password, never echo back). Save → PATCH connector. Show `token_set` yes/no only.
- Empty / 404 / error states. `npm run typecheck` must pass.

## 5. Docs

- `docs/qa/044-di-decisions.md` — decision table (copy from coordinator file if already present; keep in sync).
- `docs/qa/044-router9-functions.md` — wired endpoints, tests, missing, env. **Do not** claim live sol success.
- `docs/SETUP.md` / `.env.example`: `GOSO_ROUTER9_BASE_URL`, `GOSO_ROUTER9_MODEL`, `GOSO_ROUTER9_API_KEY` (empty ok), `GOSO_LLM_PROVIDER`, `GOSO_WEB_SEARCH`. No product keys.

## 6. Worker rules

- Worktree on **goso**, branch `admatrixmdp/spec044-router9-functions`.
- `go test ./...`, `go build ./gateway/cmd/goso-gateway`, `cd control-plane && npm run typecheck`, sibling `goso-crm/scripts/agpl-check.sh` exit 0.
- Commit. **Do not merge. Do not kill/bind demo ports.**
- Do not write live smoke transcripts as if sol succeeded.

## Non-goals

Postgres/pgvector (document only). OAuth/Apple/Stripe/K8s/Grafana/Tailscale/Redis. Filling channel tokens. Spawning sandbox/browser. Inventing GET webhook list. CRM UI. Killing `:8082`/`:8091`.
