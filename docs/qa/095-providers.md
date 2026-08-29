# QA — SPEC 095 Providers operator surface

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live paid vendor keys. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Providers: searchable cards, add/edit, enable, connect/test, type and key/connection state; configured key-based rows show only “API key set” | `docs/qa/090-goclaw-sidebar-ux.md` Providers row |

goso mapping (self-written): live tab `providers` in [App.tsx](../../control-plane/src/App.tsx) still renders [ProvidersPage](../../control-plane/src/pages/ProvidersPage.tsx). GET remains `{name,type,base_url,model,key_set,source,enabled}` with no `api_key`. Subscription marketplace UI is out of scope.

Out of scope: live paid vendor keys, Packages/API Keys pages (113/114), copying GoClaw dialogs, subscription marketplace UI, SPECs 096–102.

## What changed

- List: search, type, source (`env`/`sqlite`), optional enabled filter. `key_set` badge. Empty / filter-empty / loading / error. Env overlay rows stay read-only (existing `env overlay` 400 on PATCH/DELETE).
- Add/edit: name is create-only. Type, base URL, model, optional enabled. Write-only `api_key` (blank on load). Empty PATCH omits/keeps the boxed key. Rotate is a non-empty PATCH. Test models/chat shows `ok`, `latency_ms`, model ids or reply, redacted error — no raw JSON dump.
- New `DELETE /api/providers/{name}/key` clears sqlite-boxed `provider:{name}:api_key`. Missing 404. Env-owned name 400 `env overlay`. GET never includes `api_key`. Env still wins on name clash after clear.
- i18n vi+en. CP typecheck. Helper tests + httpapi never-leak. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/httpapi ./gateway/internal/llm ./gateway/internal/store ./gateway/internal/secrets -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` covers `filterProviders`, enabled/source/type, `canClearProviderKey`, `providerWriteBody` (blank key omitted), `formatProviderTest` (no extra `api_key` field, bearer/sk redacted).
- `go test` httpapi: empty PATCH keeps boxed key; DELETE clears box and GET has no `api_key`; env DELETE 400; env-wins after clear; test 401 body with `api_key` does not echo the key.
- GET `/api/providers` still omits `api_key`. Password input stays empty on load. Test panel is structured fields, not `JSON.stringify`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Live paid vendor keys. Packages/API Keys (113/114). Copying GoClaw dialogs. Subscription marketplace UI. Binding/killing demo ports. Merge. Inventing live vendor tokens.
