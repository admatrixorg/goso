# QA — SPEC 038 Teams, delegation, self-evolution

Date: 2026-08-27. Clean-room. Closes matrix rows **T1–T5, E1, D4**. **T6** Control Plane teams UX is **skipped** (HTTP+tests are AC; time-boxed). Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. No goclaw copy. No banned author ids.

## What changed

Teams persist as `teams` (id, name, lead_agent_id), `team_members` (role `lead|member`), Kanban `team_tasks` (status `todo|doing|done`, optional assignee), mailbox `team_messages`. Delegation edges live in `agent_links` (bidirectional = two rows).

HTTP (Bearer like other `/api/*`):

| Method | Path |
|--------|------|
| POST, GET | `/api/teams` |
| GET, PUT, DELETE | `/api/teams/{id}` |
| GET, POST | `/api/teams/{id}/members` |
| DELETE | `/api/teams/{id}/members/{agent_id}` |
| GET, POST | `/api/teams/{id}/tasks` (`?status=` filters Kanban) |
| PATCH | `/api/teams/{id}/tasks/{tid}` |
| GET, POST | `/api/teams/{id}/messages` |
| GET, POST | `/api/agents/{id}/links` (`bidirectional`) |
| GET | `/api/agents/{id}/evolution` |
| POST | `/api/agents/{id}/evolution/{sid}/apply` |

Control Plane team board UI was **not** added. MCP `goso_team_*` now calls `/api/teams` (unwraps `{teams:[…]}`).

### Orchestration modes

User wording **auto / explicit / manual**. Persist `orchestration_mode` on the agent when set; else resolve: team membership → auto; else delegate links → explicit; else manual.

| Mode | Tools advertised |
|------|------------------|
| `manual` (default, no team) | none of `spawn` / `delegate` / `team_tasks` |
| `explicit` (has links) | `delegate` |
| `auto` (team member) | `spawn`, `delegate`, `team_tasks` |

Pipeline **act** stage dispatches those three names itself (not the connector layer). Existing connector tools unchanged.

- **sync** delegate: child `Chat`, wait (timeout **10s**).
- **async**: enqueue mailbox (same team) or task / session message; return id immediately.
- **bidirectional**: both directions in `agent_links`.
- **spawn**: clone agent (same `model`) + child session; honors Lite cap.

### TEAM.md injection

If the agent is on a team, prompt prepends `Team: {name}; members: …`. When `GOSO_VAULT_DIR/TEAM.md` exists it is appended (not a file-tree dump). Agent `instructions` (prompt prefix) are also appended except prompt mode `none`.

### Self-evolution (E1)

In-memory or SQLite `agent_metrics` counts chat runs / tool errors / per-tool uses. `GET /api/agents/{id}/evolution` returns `{suggestions:[{id,rule,text,status}]}`. Rules: high tool-error rate; advertised tools never used. Suggestion bodies **must not** treat `display_name`, `agent_key`, or `identity` as write targets. `POST …/apply` only appends a prompt prefix on `instructions`. Applies that rename are rejected.

### Lite caps (D4)

`GOSO_LITE=1`: 6th `POST /api/agents` → **400**; 2nd `POST /api/teams` → **400**. Unset = no cap.

## Commands

```
go test ./...
gofmt -l gateway
go vet ./gateway/...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
/Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- Teams + Kanban filter + mailbox + members (`TestTeamsAPI_CRUDKanbanMailbox`, `TestStore_TeamsKanbanMailboxAndLinks`).
- Sync / async / bidirectional delegate (`TestDelegateSyncAsyncBidir`, scripted chat `TestChat_TeamToolsAndTEAMNote`).
- Mode gating: auto advertises three tools; manual advertises none (`TestToolSpecsGating`, `TestChat_ManualModeHidesTeamTools`).
- TEAM.md + team system note in the prompt (`TestNoteAndTEAMFile`, `TestChat_TeamToolsAndTEAMNote`).
- Evolution suggestions omit protected field names; apply writes `instructions` only (`TestEvolutionGuardrail`, `TestEvolutionAPI_Guardrail`).
- Lite on and off (`TestStore_LiteCaps`, `TestStore_LiteOffNoCap`, `TestTeamsAPI_LiteCapsAndBearer`).
- Echo still text-only; scripted LLM drives tool_use. Connector tools still go through `Runtime.CallTool`.

## Non-goals

pgvector, Discord UI polish, copying a goclaw teams package, real multi-agent LLM (Echo/scripted is enough), Control Plane teams UX (T6 skipped).
