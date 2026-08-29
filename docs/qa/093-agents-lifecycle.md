# QA — SPEC 093 Agents operator lifecycle

Date: 2026-08-29. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Agents is a searchable/filterable list with active/inactive status and provider/model context; create/edit identity, model, instructions, context mode, enable/disable, delete with named confirmation | `docs/qa/090-goclaw-sidebar-ux.md` Agents row |

goso mapping (self-written): live tab `agents` in [App.tsx](../../control-plane/src/App.tsx) still renders [AgentsPage](../../control-plane/src/pages/AgentsPage.tsx). List omits system prompts; GET `/api/agents/{id}` keeps instructions for the editor. No transfer API existed; transfer is not invented.

Out of scope: Agent Link & Team (097). Copying GoClaw dialogs. Full prompt dump in the list.

## What changed

- Agents list: search, status (active/inactive), provider filter; columns key, name, status, provider, model, orchestration mode. Instructions are not shown in the list.
- `GET /api/agents` strips `instructions`. `GET /api/agents/{id}` still returns them. Clicking a row loads the detail GET before editing.
- Create/edit: identity (key/name, name create-only), provider, model, instructions, orchestration mode, enable/disable. Empty key is a validation error (`agents.needKey`) and does not POST.
- Concurrent edit: PATCH accepts `if_updated_at`; mismatch returns **409** `agent was modified`. Last-write without the stamp still works and bumps `updated_at`.
- Delete: named confirmation (`agents.confirmDelete` with `{name}`). `DELETE /api/agents/{id}` removes the agent and its sessions. Team-lead delete is **409** `agent is team lead`. Inactive agents refuse chat with **409** `agent is inactive`.
- i18n vi+en. Loading/empty/filter-empty/error. CP typecheck. Tests for new helpers. agpl 0.

Transfer: no existing API; not added (SPEC: “if API already exists”).

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/store ./gateway/internal/httpapi ./gateway/internal/team -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` covers `filterAgents`, status/provider filters, `agentDisplayName`, `validateAgentKey`, `isConflictStatus` / `agentConflictKind`.
- `go test` store + httpapi + team: create enabled by default, list omits instructions, GET keeps them, disable + stale PATCH 409, last-write without stamp 200, inactive chat 409, delete 200 then 404, team-lead delete 409.
- GET agent list has no `instructions` field when a prompt is stored. Delete confirm interpolates the agent name. List status uses `enabled`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Agent Link & Team (097). Transfer (no API). Copying GoClaw dialogs. Binding/killing demo ports. Merge. Inventing live vendor tokens. PATCH `display_name` / `agent_key` (identity remains create-only / locked).
