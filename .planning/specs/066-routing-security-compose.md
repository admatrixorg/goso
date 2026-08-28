# SPEC 066 — Runtime provider routing + production security + compose

> LOCKED: 2026-08-28. Clean-room Go/React. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.
> **Do not break** demo env `router9` + `ocg/deepseek-v4-flash` when `GOSO_ENV` is not `production`.

Closes CTO-01, CTO-02, CTO-03, CTO-07 (partial). Audit: `docs/qa/audit-cto-2026-08-28.md`.

## 1. Chat must use persisted provider + agent.model

Today `serve.DefaultProvider()` / `Runtime.LLM` is one process-wide env provider. `PATCH /api/agents/{id}` `{model}` and SQLite `llm_providers` never reach `Runtime.Chat`.

### Data

Add `LLMProvider string \`json:"llm_provider,omitempty"\`` on `store.Agent`. Persist memory + SQLite (`ALTER TABLE agents ADD COLUMN llm_provider TEXT`). POST/PATCH `/api/agents` accept `llm_provider` (empty = default). Unknown name → 400 after resolve attempt.

### Resolve (env wins)

`Resolve(store, name, model) (llm.Provider, error)`:

1. If `name` empty → `DefaultProvider()`, then if `model` non-empty set it on OpenAI/Anthropic/echo-compatible types (clone; do not mutate registry singleton).
2. If env registry `Has(name)` → `Get(name)` then apply `model` override.
3. Else SQLite `GetLLMProvider(name)` + `secrets.Get` api_key → `llm.Build(...)` then apply `model`.
4. Else error `provider not found`.

### Chat path

`Runtime.Chat` / pipeline: from session → agent → `Resolve(agent.LLMProvider, agent.Model)` **per request**. Do not only use `rt.LLM`. Fallback `rt.LLM` if resolve fails **only** when name empty; named miss is 400/502 with public error.

Cron `FireSessionChat` and WS chat must use the same resolve.

### Tests (mandatory)

Two `httptest` OpenAI-compat servers. Create sqlite (or memory+secrets) providers `p-a`/`p-b` with those base URLs. Create agents `a`/`b` with `llm_provider` + distinct `model`. Two sessions. POST `/api/chat` each session: **server A must see A's model, server B must see B's**. Env `GOSO_LLM_PROVIDER=router9` must **not** steal these named sqlite rows. Empty `llm_provider` still uses DefaultProvider (router9/echo) so demo chat stays.

## 2. SSRF DNS-aware + LLM endpoints

`security.CheckURL` today parses host as IP only; hostnames skip. When `GOSO_SSRF=1` (or production — §4):

- `LookupIP` (or `LookupIPAddr`); if lookup fails → deny.
- Deny if **any** address is loopback/private/link-local/unspecified/multicast, or IPv6 unique-local, or `169.254.169.254`.
- `GuardClient` already re-checks redirects; use it on provider HTTP clients.

Call `CheckURL` on provider `base_url` in `llm.Probe` **and** live Chat HTTP (OpenAI/Anthropic). Connector path stays.

Tests: hostname that resolves to `127.0.0.1` blocked when SSRF on; public IP/example.com allowed (httptest can use `LookupIP` hook **or** unexported test helper). Literal `127.0.0.1` still blocked. `GOSO_SSRF=0` + demo router9 `http://127.0.0.1:20127/v1` still works.

## 3. Injection + WS origin + query token

- `GOSO_INJECTION=log|block` already. **Production:** default **block** if unset. Dev/demo: default log.
- **Production:** `GOSO_WS_ORIGINS` empty → **refuse start** (log + non-zero). Dev: empty = allow-all.
- **Production:** ignore `?token=`; only `Authorization: Bearer`. Dev: keep query token.

`GOSO_ENV=production` (case-insensitive) is the gate. `GOSO_ENV=demo` (current :18080) must still boot without WS origins.

Startup helper `security.CheckProduction()` called from `main`/`serve.New`. Tests for production refuse vs demo allow.

## 4. Compose

`compose.yml` **and** `compose.prod.yml` gateway `environment` must pass through (empty default OK except prod notes):

`GOSO_MASTER_KEY`, `GOSO_LLM_PROVIDER`, `GOSO_ROUTER9_BASE_URL`, `GOSO_ROUTER9_MODEL` (if that env exists in code), `GOSO_SSRF`, `GOSO_INJECTION`, `GOSO_WS_ORIGINS`, `GOSO_DEV_MODE`.

Do not put real secrets in the file. `.env.example` comment the new names (placeholders only).

## 5. Control plane

`AgentsPage`: select `llm_provider` from `GET /api/providers` names (plus empty = default). PATCH/POST send `llm_provider`. i18n vi+en. StatusLine. typecheck.

## QC

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD <sibling>/goso-crm/scripts/agpl-check.sh
```

`docs/qa/066-routing-security-compose.md`. Commit `admatrixmdp/spec066-routing-security`. Do not merge. Do not start 067.

## Non-goals

Streaming (068), durable webhooks (067), tenant PG (069), backup drill (070), CTO-09 author-id sweep (071). Live Discord tokens.
