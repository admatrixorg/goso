# QA — SPEC 066 Runtime provider routing + production security + compose

Date: 2026-08-28. Clean-room Go/React. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not break env `router9` + default `ocg/deepseek-v4-flash` when `GOSO_ENV` is not `production`.

Closes CTO-01, CTO-02, CTO-03, CTO-07 (partial).

## What changed

- `store.Agent.llm_provider` persisted (memory + SQLite `ALTER TABLE agents ADD COLUMN llm_provider TEXT`). POST/PATCH `/api/agents` accept `llm_provider` (empty = default). Unknown name → 400 after resolve.
- `llm.Resolve(store, name, model, fallback)`: empty name → fallback/`Preferred()` then clone+model override; env `Has(name)` wins; else SQLite row + `secrets.Get` → `llm.Build`; else `provider not found`. Registry singletons are cloned.
- `Runtime.Chat` / pipeline / cron `FireSessionChat` / WS + webhook chat resolve per session agent. Named miss is a public 400 (`provider not found`); other LLM errors stay 502. Env `GOSO_LLM_PROVIDER=router9` does not steal named sqlite rows.
- SSRF DNS-aware when `GOSO_SSRF=1` **or** `GOSO_ENV=production` with `GOSO_SSRF` unset: `LookupIP`; lookup fail denies; any loopback/private/link-local/unspecified/multicast / IPv6 unique-local / `169.254.169.254` denies. `CheckURL` on LLM probe + OpenAI/Anthropic chat; `GuardClient` on those HTTP clients. Demo / `GOSO_SSRF=0` still allows `http://127.0.0.1:20127/v1`.
- Production (`GOSO_ENV=production`, case-insensitive): `GOSO_WS_ORIGINS` empty → `security.CheckProduction()` refuse start (log + non-zero). Injection default **block** if unset. Query `?token=` ignored (Bearer only). Demo / unset still boot without WS origins; injection default log; query token kept.
- `compose.yml` and `compose.prod.yml` pass through `GOSO_MASTER_KEY` `GOSO_LLM_PROVIDER` `GOSO_ROUTER9_BASE_URL` `GOSO_ROUTER9_MODEL` `GOSO_SSRF` `GOSO_INJECTION` `GOSO_WS_ORIGINS` `GOSO_DEV_MODE` (empty default, no secrets in the file). `.env.example` comments the names.
- Control-plane `AgentsPage`: `llm_provider` select from `GET /api/providers` names plus empty = default. POST/PATCH send the field. i18n vi+en. StatusLine.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- Two httptest OpenAI-compat servers + sqlite providers `p-a`/`p-b` + two agents/sessions: each `POST /api/chat` hits the matching server/model; env `router9` only sees the empty-`llm_provider` agent (`TestChat_NamedProvidersHitOwnServers`).
- Unknown `llm_provider` POST/PATCH 400 (`TestAgents_UnknownLLMProvider400`).
- Memory + SQLite persist/clear `llm_provider` (`TestStore_AgentLLMProvider`, `TestSQLiteStore_AgentLLMProviderPersist`).
- Resolve: empty clones model without mutating singleton; env wins over sqlite `router9`; named sqlite not stolen by `GOSO_LLM_PROVIDER` (`TestResolve_*`).
- SSRF: hostname→127.0.0.1 blocked; example.com public IP allowed via lookup hook; lookup fail denies; literals including metadata + unique-local blocked; default off still allows demo loopback (`TestCheckURL_*`, `TestProbeAndChat_SSRFBlocksLoopback`).
- Production refuse vs demo allow (`TestCheckProduction_RefuseVsDemoAllow`, `TestNew_ProductionRequiresWSOrigins`, `TestNew_DemoAllowsEmptyWSOrigins`). Injection production default block (`TestInspectChat_ProductionDefaultBlock`). Query token ignored in production (`TestRequireToken_ProductionIgnoresQuery`).

## Non-goals

Streaming (068), durable webhooks (067), tenant PG (069), backup drill (070), CTO-09 author-id sweep (071). Live Discord tokens. Merge. SPEC 067.
