# SPEC 048 — Agent orchestration_mode auto/explicit/manual (API + UI)

> LOCKED: 2026-08-27. Clean-room. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

`store.Agent.OrchestrationMode` and `UpdateAgent` already exist. `POST /api/agents` accepts `orchestration_mode`. **No PATCH /api/agents/{id}.** Teams page has no mode picker.

## HTTP

- `PATCH /api/agents/{id}` `{orchestration_mode?: "auto"|"explicit"|"manual", model?: string, instructions?: string}` → agent JSON. Invalid mode → 400. Missing agent → 404.
- GET list/get already return `orchestration_mode` if stored.
- Tests: create with auto; PATCH to manual; GET shows it; bad mode 400.

## UI

- `AgentsPage`: column/select auto|explicit|manual; PATCH on change. StatusLine loading/error.
- `TeamsPage`: show each member's mode; optional select if agent list includes the field.
- i18n vi+en. `npm run typecheck`.

`docs/qa/048-orchestration-mode.md`. Commit `admatrixmdp/spec048-orch-mode`. Do not merge.
