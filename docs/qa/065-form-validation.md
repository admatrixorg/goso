# QA — SPEC 065 Form validation sweep (live tabs)

Date: 2026-08-28. Clean-room React in `control-plane/`. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not break env `router9` + default `ocg/deepseek-v4-flash`.

Today live create handlers on Teams, Vault, Memory, Connectors, and Marketing `return` without StatusLine when required fields are empty, so the user sees no error and no network. Events filters stay optional GET. Providers (056) already validates `providers.needName` on test.

## Wired

- Trim required fields on live create/POST. Missing → `StatusLine` kind=error with i18n key. No network call.
- Server errors still go through `formatPublicError` (Marketing switched from `String(e)`).
- Skip Providers (056 already validating). Skip Events filters (optional). DEMO mock pages untouched.
- Reuse StatusLine + `t("*.need*")` pattern from 057/058/062. New keys are page-namespaced so 057 `agents.needKey`, 058 `sessions.needAgent`, 062 `functions.cron.need*` are not duplicated.

| page | field | key |
|------|-------|-----|
| Teams | name | `teams.needName` |
| Teams | lead agent | `teams.needLead` |
| Teams | member agent | `teams.needMember` |
| Teams | task title | `teams.needTask` |
| Teams | message from | `teams.needFrom` |
| Teams | message body | `teams.needBody` |
| Teams | link from agent | `teams.needMember` |
| Teams | link to agent | `teams.needTo` |
| Vault | title | `vault.needTitle` |
| Memory | session | `memory.needSession` |
| Memory | note | `memory.needNote` |
| Connectors | name | `connectors.needName` |
| Connectors | endpoint | `connectors.needEndpoint` |
| Connectors | assign agent | `connectors.needAgent` |
| Connectors | assign connector | `connectors.needConnector` |
| Marketing | audience name | `mkt.needName` |
| Marketing | campaign name | `mkt.needCampaign` |
| Providers (skip) | test name | `providers.needName` (056) |

## Not wired (no API / out of scope)

- Schema library / HTML5-only validation without StatusLine.
- New HTTP routes — none.
- DEMO mock pages (home/tasks/meetings/friends/calendar/gallery).
- Providers create form (056 already has `providers.needName` on test; skip).
- Events filters (optional query params, empty = list all).
- Gateway / LLM / router9 changes — none.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Non-goals

Schema library, HTML5-only without StatusLine, new APIs, binding/killing demo ports, copying goclaw/ZaloCRM.

## Proof (2026-08-28)

- `cd control-plane && npm run typecheck` → exit 0 (`tsc --noEmit`, no errors)
- `go test ./...` → exit 0 (24 ok, 3 no-test pkgs, 0 fail). No `*.go` / `gateway/*` in `git diff HEAD`.
- `GOSO_ROOT=$PWD …/agpl-check.sh` → exit 0 (`OK`; no banned author ids outside `.planning`)
- Source: silent `return` on empty create replaced with `setErr(t("*.need*")); return;` before POST/PUT. Marketing create no longer calls `run()` (which would GET) on empty name.
- `formatPublicError` kept on Teams/Vault/Memory/Connectors; Marketing `load`/`run` now use it instead of `String(e)`. Error row is `StatusLine kind="error"`.
- i18n en+vi keys added and matched. Existing 057/058/062 keys unchanged.
- Diff: Teams/Vault/Memory/Connectors/Marketing pages, `en.ts`/`vi.ts`, this QA file. DEMO pages: no diff. Env router9 + default `ocg/deepseek-v4-flash` untouched.
- Ports `:8082` `:8091` `:3000` `:18080` `:18088` not bound or killed.
