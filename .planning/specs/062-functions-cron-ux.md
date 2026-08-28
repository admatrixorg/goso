# SPEC 062 — Functions / cron form UX

> LOCKED: 2026-08-28. Clean-room React. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

Functions already has tools + connector patch + cron card (`cronApi`, SPEC 054). `createCron()` returns silently when spec/session/message empty. Delete has no confirm. Empty session list does not point at 058.

## UI

- Missing spec / session / message → `StatusLine` i18n error (`functions.cron.need*`), no POST.
- Delete: confirm (`window.confirm` with i18n is OK) then DELETE.
- If `sessions.length === 0`, empty copy: create a session first (Sessions tab).
- Show `enabled` if the JSON already has it; **do not invent PATCH enable** (054 non-goal).
- Connector save with empty name still no-op — StatusLine `functions.needConnector`.
- i18n vi+en.

`docs/qa/062-functions-cron-ux.md`. Commit `admatrixmdp/spec062-functions-cron-ux`. Do not merge.

## QC

`cd control-plane && npm run typecheck` · `go test ./...` · agpl-check 0.

## Non-goals

Cron PATCH/run-now, MCP cron rewrite, OS crontab.
