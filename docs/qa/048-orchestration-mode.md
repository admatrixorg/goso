# QA — SPEC 048 Agent orchestration_mode auto/explicit/manual

Date: 2026-08-27. Clean-room. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. `store.Agent.OrchestrationMode` and `UpdateAgent` already existed; POST create already accepted the field. This SPEC adds HTTP PATCH + UI picker.

## What changed

- `PATCH /api/agents/{id}` `{orchestration_mode?: "auto"|"explicit"|"manual", model?: string, instructions?: string}` → agent JSON. Uses `store.UpdateAgent`. Invalid mode → 400. Missing agent → 404. Omitted fields keep current values (handler loads the agent first so instructions are not wiped).
- GET list/get already return `orchestration_mode` when stored (`omitempty`).
- Control-plane `AgentsPage`: mode column + select auto|explicit|manual; PATCH on change. StatusLine loading / error.
- `TeamsPage`: each member shows mode; select when the agent list includes `orchestration_mode`.
- i18n vi+en (`agents.col.mode` `agents.mode.auto|explicit|manual|unset` `teams.col.mode`).

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Coordinator after merge: restart `:18080` with new `goso-gateway`; Vite `:3000` proxies to `:18080`.

## Proof

- httptest: create with `orchestration_mode=auto` and `instructions=keep-me`; PATCH to `manual`; GET shows `manual` and still `keep-me`; bad mode and present-empty mode 400; missing id 404 (`TestPatchAgentOrchestrationMode`).
- Same test PATCHes optional `model` + `instructions` without clearing the mode.

## Non-goals

MCP `/v1/agents` update, DELETE agent, live demo bind/kill, copying goclaw/ZaloCRM, rewriting `agent_key` / `display_name`.
