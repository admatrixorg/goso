# QA — SPEC 102 Settings operator surface

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA. No invented live vendor tokens.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Config: typed forms for server/behavior/quota/tools/integrations; env-managed controls disabled with an explanation; gateway auth token not returned | `docs/qa/090-goclaw-sidebar-ux.md` Config |

goso mapping (self-written): live tab `settings` in [App.tsx](../../control-plane/src/App.tsx) still renders [SettingsPage](../../control-plane/src/pages/SettingsPage.tsx). Nav groups remain account / team / messaging / system. Existing CRM cards (account, users, roles, nicks, quotas, templates, billing) plus backup, pairing, and theme stay. System now includes a Gateway page for process configuration.

Gateway contract: `GET /api/config` and `PUT /api/config` (aliased at `/v1/config`). GET groups `server`, `auth`, `behavior`, `quota`, `tools`, `integrations`. Auth is `token_set` / `view_token_set` / `master_key_set` booleans only — never the token or DSN. Env-owned fields have `env_owned: true` and `editable: false`. PUT validates overlay keys (`log_level`, `quota_day`, `injection`, `ssrf`, `heartbeat`, `kg_extract`, `cache_mode`); 400 on bad values or secret keys; 409 `field is env-owned` when the process env owns that key; 409 `config was modified` on stale `updated_at`. Overlay is persisted in `gateway_settings` and applied in-process so quota/injection/ssrf/heartbeat/kg/cache readers see it when env is unset.

Out of scope: API Keys (113). Tenants admin (112). Packages (114). Copying GoClaw Config tabs. Live vendor tokens. Backup S3 (117).

## What changed

- Settings sections: account / team / messaging / system. Loading / error / save feedback on the operator surface. Required-name and quota validation before create/save.
- Gateway page: server (port/host/env/log_level), auth (`*_set` only), behavior, quota, tools, integrations. Env-owned fields marked read-only. Backup, pairing, theme kept.
- `GET /api/config` never returns `GOSO_ADMIN_TOKEN` / view token / master key / database URL values. Client `publicHasSecrets` is a second gate.
- PUT overlay + 409 conflict / env-owned. i18n vi+en. CP typecheck. Tests. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/config ./gateway/internal/store ./gateway/internal/httpapi ./gateway/internal/billing ./gateway/internal/security ./gateway/internal/heartbeat ./gateway/internal/pipeline ./gateway/internal/llm ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` 91/91 including `formFromSnapshot` skipping env-owned writes, `validateGatewayForm`, `settingsConflictKind` 409, `publicHasSecrets` allowing `token_set` and flagging a raw `token` value.
- `go test` config: env wins overlay; PUT validation; snapshot with `GOSO_ADMIN_TOKEN` set does not contain the token string. store: memory + sqlite stale `updated_at` → `ErrConflict`. httpapi: GET omits live admin token; PUT 400 negative quota and secret key; PUT 409 stale stamp and env-owned injection; `/v1/config` GET. billing: overlay `quota_day` applies when env is empty, env still wins. Existing security/heartbeat/pipeline/llm/serve tests pass.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

API Keys (113). Tenants admin (112). Packages (114). Copying GoClaw Config tabs. Live vendor tokens. Binding/killing demo ports. Merge. Backup S3 / preflight (117).
