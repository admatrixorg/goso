# QA — SPEC 100 Webhooks operator surface

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA. No invented live webhook URLs with secrets.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Hooks: lifecycle interceptors (script/HTTP/prompt/built-in) | `docs/qa/090-goclaw-sidebar-ux.md` Hooks |
| HTTP delivery + secret lifecycle maps to goso Webhooks, not lifecycle hooks | `docs/qa/090-goclaw-sidebar-ux.md` Hooks / Webhooks |

goso mapping (self-written): live tab `webhooks` in [App.tsx](../../control-plane/src/App.tsx) still renders [WebhooksPage](../../control-plane/src/pages/WebhooksPage.tsx). Operator list is `GET /api/webhooks` with `status`, `endpoint`, `last_delivery`, `token_prefix`, `secret_set`. Create/rotate return `token` and `hmac_key` once. Later GET/list omit those fields. Test/replay POST a signed `{event}` envelope without `input`, `reply`, `token`, or `hmac_key`. Copy states these are HTTP webhooks, not lifecycle hooks.

Out of scope: Lifecycle hook interceptors (GoClaw Hooks). API Keys page (113). Copying GoClaw dialogs. Live vendor tokens / live webhook URLs with secrets. SPECs 101–102.

## What changed

- List: `GET /api/webhooks` rows include status (`active` / `revoked` / `failing`), endpoint (stored outbound URL or inbound `/api/webhooks/llm`), last delivery `{id,status,at}` without payloads. Empty / loading / error. Revoked rows stay visible with status.
- Create: optional name + outbound URL. One-time secret reveal (token + hmac_key). GET never returns them later. Userinfo is stripped from stored URLs.
- Rotate / revoke with named confirm. Rotate mints a new secret (shown once). Revoke → inbound 401. Audit events `rotate` / `revoke` / `test` / `replay` on connector `webhooks` with redacted summaries.
- Test / replay: `POST /api/webhooks/{id}/test` and `/replay` deliver a signed envelope without secret-bearing payloads. Require an outbound HTTP endpoint (not the inbound path).
- Copy: vi+en states HTTP webhooks, not agent lifecycle hooks.
- i18n vi+en. CP typecheck. Helper tests + webhook HTTP. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/store ./gateway/internal/webhook ./gateway/internal/httpapi -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` covers `asPublic` list columns, `webhookStatus` revoked/failing, inbound endpoint fallback, `lastDeliveryLabel` empty, `listTargetName`, one-time `asCreated` + `hideCopiedSecret`, `canTestOrReplay`, `publicHasSecrets`.
- `go test` store + webhook + httpapi: endpoint persist + latest job; operator list includes revoked without secrets; test/replay httptest bodies omit token/hmac_key/input/reply; GET list/get never contain create/rotate secrets; audit tools `test` `replay` `rotate` `revoke`; test without endpoint is 400.
- Existing durable webhook tests still pass (Bearer/HMAC, rotate invalidates bearer, delete revokes, callback 2xx, list public-only).
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Lifecycle hook interceptors. API Keys (113). Copying GoClaw dialogs. Live vendor tokens. Invented secret-bearing webhook URLs. Binding/killing demo ports. Merge. SPECs 101–102.
