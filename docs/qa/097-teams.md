# QA — SPEC 097 Teams operator surface

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Agent Teams / Agent Links: searchable list, team detail/create, members + lead, directed links, empty “No teams yet.” | `docs/qa/090-goclaw-sidebar-ux.md` Agent Link & Team |

goso mapping (self-written): live tab `teams` in [App.tsx](../../control-plane/src/App.tsx) still renders [TeamsPage](../../control-plane/src/pages/TeamsPage.tsx). Tasks and mailbox stay because `GET/POST /api/teams/{id}/tasks` and `/messages` already exist. Evolution stays behind `auto_adapt` / `locked`; suggestion copy is truncated and the UI never renders agent `instructions`.

Out of scope: Tenants admin (112). Copying GoClaw dialogs. Live vendor tokens. SPECs 098–102.

## What changed

- List: search by name/id/lead, empty / filter-empty / loading / error. Detail shows members + lead badge, directed vs bidirectional links (`→` / `↔`), Kanban tasks, mailbox.
- Create and edit team (name + lead). Add member (`lead`/`member`). Set lead. Cannot remove the current lead until another lead is set. Delete team requires typing the team name. Remove member and unlink use a confirm that names the target.
- `DELETE /api/agents/{id}/links/{to_id}` removes a directed edge; `?pair=1` also drops the reverse. GET/POST links now include `bidirectional`. `DELETE /api/teams/{id}` and `DELETE /api/teams/{id}/members/{agent_id}` were already present and are wired in the UI.
- Evolution: `auto_adapt` / `min_runs` / locked badges (`display_name`, `agent_key`, `identity`). Suggestion text is capped; apply reloads suggestions and does not dump the agent prompt.
- i18n vi+en. CP typecheck. Helper tests + store/httpapi unlink. agpl 0.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/httpapi ./gateway/internal/store ./gateway/internal/team -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` covers `filterTeams`, `validateTeamDraft`, `linkDirection` / `linkArrow`, `namedConfirmTarget`, `safeEvolutionText` (prompt dump truncated, no secret policy body) in `teams-ops.ts`.
- `go test` store + httpapi + team: create/update/delete team, remove member, bidirectional GET, `DELETE` unlink with `pair=1`, evolution apply still cannot rename identity.
- GET `/api/agents/{id}/links` includes `bidirectional` and no `instructions`. Evolution panel shows locked fields, not system prompts.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Tenants admin (112). Copying GoClaw dialogs. Live vendor tokens. Binding/killing demo ports. Merge. SPECs 098–102.
