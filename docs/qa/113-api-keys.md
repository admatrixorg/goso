# QA — SPEC 113 API Keys

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| API Keys: searchable table plus usage and create; create/inspect usage/refresh/revoke; scopes admin/read/write/approvals/pairing/provision; list masked; full key only at create | `docs/qa/090-goclaw-sidebar-ux.md` API Keys |

goso mapping (self-written): live tab `apikeys` in [App.tsx](../../control-plane/src/App.tsx) renders [ApiKeysPage](../../control-plane/src/pages/ApiKeysPage.tsx). Listing is `GET /api/api-keys` (`/v1/api-keys` alias) with `q` search. Rows are `{id,name,prefix,tenant_id,scopes,status,use_count,created_at,expires_at,last_used_at,revoked_at}` — no `secret`, `hash`, or `token`. `tenant_id` is issuance metadata on the inventory row; request isolation stays SPEC 112 (`X-Goso-Tenant` + admin token). Create is `POST /api/api-keys` `{name,tenant_id,scopes,expires_at}` and returns `secret` once. Later GET list/get omit it (store is SHA-256 hash + 11-char `gk_` prefix). Names/tenants that look like a minted `gk_` secret are rejected. Revoke is `POST /api/api-keys/{id}/revoke` `{confirm}` matching name, prefix, or id. Issued keys authenticate via Bearer after hash lookup; scopes gate methods (`admin` full, `read` GET view matrix, `write` non-privileged writes, `approvals`/`pairing`/`provision` their paths). View-token GET list 200; POST 403. Mutations append SPEC 110 audit rows (`entity=api_key`, actions create/revoke) with prefix/scopes only.

Out of scope: Packages (114). Approvals (115). Copying GoClaw chrome. Live vendor tokens. SPECs 114–118.

## What changed

- Live nav tab + page. Masked inventory, loading / empty / error. Create form with scopes, optional tenant and expiry. Usage panel. Copy acknowledgment hides the secret. Revoke with typed confirm.
- Create reveals the full key exactly once. GET never returns the secret (hash + prefix only). Audit create/revoke without logging the secret. Issued keys increment usage on Accept.
- i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/113-api-keys.md`.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/apikey ./gateway/internal/auth ./gateway/internal/httpapi ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` including `asPublic` dropping secret/`sk-` rows, `publicHasSecrets`, `asCreated` + `hideCopiedSecret`, `filterKeys`, `keyConfirmMatch`.
- `go test` apikey registry: create stores hash not plaintext, GET/list omit `secret`, revoke confirm, Accept usage, expired/revoked reject, secret-shaped name rejected. httpapi: create 201 includes secret once; GET list/get omit it; revoke; audit create/revoke without plaintext; `/v1/api-keys` alias; view-token GET 200 / POST 403; issued `read` GET 200 / POST 403, `write` can create agents but not keys. auth/serve: view GET `/api/api-keys` 200, POST 403; issued-key scopes.
- Page copy: “No API keys yet.” / “Chưa có API key.” List shows prefix with ellipsis, never the full secret after copy. Revoke confirm types name, prefix, or id. Expand is text, no `dangerouslySetInnerHTML`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Packages (114). Approvals (115). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge. SPECs 114-118.
