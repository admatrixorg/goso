# QA — SPEC 096 Functions operator surface

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live paid vendor keys. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Skills inventory/create/archive/search; Built-in Tools grants + approval/config state; MCP stdio/SSE/HTTP add/test; Cron list/create/enable | `docs/qa/090-goclaw-sidebar-ux.md` Skills / Built-in Tools / MCP Servers / Cron |

goso mapping (self-written): live tab `functions` in [App.tsx](../../control-plane/src/App.tsx) still renders [FunctionsPage](../../control-plane/src/pages/FunctionsPage.tsx). Connector GET remains `{name,transport,endpoint,token_set,source,env_owned,env_set,enabled,…}` with no `token`. Skill scripts are never executed.

Out of scope: TTS (118), Packages (114), copying GoClaw dialogs, live paid MCP marketplaces, Nodes/Workstations, SPECs 097–102.

## What changed

- Tools: per-agent grant/enable (`agent_tool_flags`). `requires_approval` badge. `configured` vs `not_configured`. Connector-bound PATCH no longer toggles global connector enable. Tool lists never include token fields.
- Skills: BM25 search, create, archive/delete with confirm, loading/empty/error. No script exec.
- MCP/connectors: list + add with stored transports `http` / `mcp-http` (SSE alias) / `mcp-stdio`. `POST /api/connectors/{name}/test`. Token write-only; GET never returns token values. Env-owned (`credential_ref` env name) rejects token PATCH (`env overlay`). Disabled state explicit.
- Cron: list/create, `PATCH /api/cron/{id}` enable, session bind, `last_run` / redacted `last_error` if present, empty/error. Failed ticks store last_error and do not mark last_run.
- i18n vi+en. Loading/empty/error per card. CP typecheck. Helper tests + httpapi never-leak. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/httpapi ./gateway/internal/store ./gateway/internal/cron ./gateway/internal/connector ./gateway/internal/skill -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` covers `normalizeTransport`, `connectorWriteBody` (blank token omitted), `isConnectorEnvOwned`, `formatConnectorTest` (Bearer/token redacted, no extra `token` field), `toolListLeaksSecret`.
- `go test` httpapi: per-agent flags independent; connector-bound PATCH does not disable global connector; env-owned GET has no token value and PATCH token is 400 `env overlay`; test disabled → `health:disabled`; cron PATCH enable; last_error redacted on GET.
- GET `/api/connectors` and GET `/api/agents/{id}/tools` omit token values. Password inputs stay empty on load.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

TTS (118). Packages (114). Copying GoClaw dialogs. Live paid MCP marketplaces. Nodes/Workstations. Binding/killing demo ports. Merge. Inventing live vendor tokens. Injecting MCP process env into stdio subprocesses.
