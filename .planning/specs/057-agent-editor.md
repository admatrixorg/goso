# SPEC 057 — Agent editor (model + instructions)

> LOCKED: 2026-08-28. Clean-room React in `control-plane/`. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.
> **Do not break** env `router9` + `ocg/deepseek-v4-flash`.

Today `AgentsPage` can create (`POST /api/agents` with `agent_key` + `display_name` only) and patch orchestration mode. Empty `agent_key` **silently returns**. Placeholders are raw English `agent_key` / `display_name`. `PATCH /api/agents/{id}` already accepts `model` and `instructions` (`gateway/internal/httpapi/handlers.go` `handlePatchAgent`) but the form never sends them. Client `createAgent` omits `instructions`; `updateAgent` already types `model` / `instructions`.

## UI

`AgentsPage`:

- Click a row → edit panel (not a new route). Fields: display_name (create-only / read-only on edit — **do not invent PATCH display_name**), model, instructions (textarea), orchestration mode (existing select).
- Create: require `agent_key` (trim). Empty → `StatusLine` error via i18n (`agents.needKey`), do not POST. Optional model + instructions on create (`POST` already accepts both).
- Save edit: `PATCH` `{model?, instructions?, orchestration_mode?}`. StatusLine loading/error. Refresh list.
- i18n vi+en for all new labels/placeholders/errors (`MsgKey`). No English leftover placeholders.
- Do **not** add DELETE (no API). Do not show API keys.

`npm run typecheck`. Keep DEMO tabs untouched.

## Worker

Commit `admatrixmdp/spec057-agent-editor`. `docs/qa/057-agent-editor.md` (wired fields vs missing DELETE/display_name). Do not merge.

## QC

- `cd control-plane && npm run typecheck`
- `go test ./...` (must stay green; Go change not required)
- sibling `goso-crm/scripts/agpl-check.sh` 0

## Non-goals

DELETE agent, PATCH display_name, new Go endpoints, CRM UI, killing demos.
