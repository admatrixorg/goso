# SPEC 038 — Teams, delegation, self-evolution

> LOCKED: 2026-08-27. Clean-room. Closes **T1–T6, E1, D4**. Lite caps: max **5 agents**, max **1 team** when `GOSO_LITE=1` (desktop default can set this later; gateway honors the env).
> Demos do not kill. No goclaw copy. No banned author ids.

## Teams

- Store: `teams` (id, name, lead_agent_id), `team_members` (team_id, agent_id, role lead|member).
- Task board: `team_tasks` (id, team_id, title, status `todo|doing|done`, assignee_agent_id optional). Kanban = list filtered by status.
- Mailbox: `team_messages` (id, team_id, from_agent_id, body, created_at).
- HTTP `/api/teams` CRUD; `/api/teams/{id}/members`; `/api/teams/{id}/tasks`; `/api/teams/{id}/messages`. Bearer.
- Control plane: **optional** thin list if cheap; not required if time-boxed — HTTP+tests are AC. Document if UI skipped.

## Delegation + 3 modes

Modes on agent (field or team membership):

| Mode | Tools advertised in pipeline ToolSpec |
|------|----------------------------------------|
| `manual` / spawn (default, no team) | none of team tools (or only `spawn` stub) |
| `explicit` / delegate (has delegate links) | `delegate` |
| `auto` / team (member of a team) | `spawn`, `delegate`, `team_tasks` |

User wording: **auto / explicit / manual**. Map: manual=no team tools; explicit=delegate links; auto=full team tools. Persist `orchestration_mode` if set; else resolve: team membership → auto; else links → explicit; else manual.

- **sync** delegate: run child Chat and wait (timeout 10s in tests).
- **async**: enqueue mailbox/task, return id immediately.
- **bidirectional**: link both directions in `agent_links`.

`spawn`: create child session/agent clone (same provider), optional Lite cap. Tests with Echo/scripted LLM.

Pipeline **act** stage: if ToolCall name is `delegate`/`spawn`/`team_tasks`, dispatch here (not connector). Existing connector tools unchanged.

## TEAM.md injection

If team exists, prompt stage prepends a short “Team: {name}; members: …” system note (not a goclaw file dump). Optional file `GOSO_VAULT_DIR/TEAM.md` if present after 037.

## Self-evolution (E1)

- Metrics: count tool errors / chat runs per agent (in-memory ring or SQLite table `agent_metrics`).
- `GET /api/agents/{id}/evolution` → suggestions `{id, rule, text, status}`.
- Rules (deterministic, no extra LLM required): high tool-error rate; never-used tools. **Guardrail:** never suggest changing agent `display_name`, `agent_key`, or “identity”. Tests: suggestion body must not contain those field names as write targets.
- `POST /api/agents/{id}/evolution/{sid}/apply` may tweak a **prompt prefix** string on the agent (`other`/`instructions` field) only. Reject applies that rename.

## Lite caps

`GOSO_LITE=1`: 6th `POST /api/agents` → 400; 2nd team → 400. Tests both flags.

## Non-goals

pgvector, Discord UI polish, copying goclaw teams package, real multi-agent LLM (Echo is enough).

## QC

`go test ./...`, build, agpl-check 0, `docs/qa/038-teams-evolution.md`. Commit, do not merge.
