# QA — SPEC 073 Self-evolution auto-adapt + guardrails

Date: 2026-08-29. Clean-room Go/React. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not merge. Do not start SPEC 074.

Closes matrix **E1** (was PARTIAL: suggestions + apply-to-instructions; now auto-adapt with guardrails, still never identity/name/key).

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Metrics → suggestions → auto-adapt; 3-stage workflow (analyze / pending / apply + baseline) | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/01-agent-loop.md` Self-Evolution System |
| Guardrails: min data points, rollback on quality drop, locked params | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/01-agent-loop.md` Adaptation Guardrails |
| Never silently change identity / name / core purpose | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/01-agent-loop.md` Adaptation Guardrails (`locked_params` / identity anchoring) |
| Optional self-evolve toggle default off; SOUL.md style only (goso maps to instructions, does not write SOUL.md) | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/21-agent-evolution-and-skill-management.md` Self-Evolution |

goso mapping (self-written): persist `evolution_guardrails` JSON (`auto_adapt` default **false**, `min_runs` default 20, `locked` always includes `display_name`, `agent_key`, `identity`). `POST /api/agents/{id}/evolution/tick` applies the first pending unlocked suggestion to **instructions only**. A later tick with higher `error_rate` (`tool_errors / chat_runs`) restores the pre-apply instruction snapshot. Name/key apply stays **400**. Manual `POST …/evolution/{sid}/apply` from SPEC 038 is unchanged. In-process ticker only when `GOSO_EVOLUTION_AUTO=1` (default **off**). Demo / unset auto_adapt is a no-op tick.

## What changed

- Store (in-memory + SQLite): `evolution_guardrails(agent_id, payload)`. Defaults on missing row. `Put` cannot drop name/key from `locked`.
- HTTP (Bearer like other `/api/*`):
  - `GET /api/agents/{id}/evolution` → `{suggestions, guardrails}` (`guardrails` is public: auto_adapt, min_runs, locked).
  - `PATCH /api/agents/{id}/evolution` `{auto_adapt?, min_runs?, locked?}`. `locked: []` still re-inserts `display_name` + `agent_key` + `identity`. `min_runs <= 0` → 400.
  - `POST /api/agents/{id}/evolution/tick` → `{action: noop|applied|rolled_back, reason?, suggestion_id?, agent?, guardrails}`.
  - `POST /api/agents/{id}/evolution/{sid}/apply` unchanged (instructions prefix; display_name / agent_key → 400).
- `GOSO_EVOLUTION_AUTO` default off. When `1`, serve starts a 1-minute in-process loop calling tick per agent. Per-agent `auto_adapt` is still required. Tests do not start the loop (`testing.Testing()`).
- Control Plane Teams evolution card: checkbox **Tự thích nghi** + min_runs. i18n vi+en. StatusLine loading/error.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 074.

## Proof

- auto_adapt off → tick no-op (`TestTick_AutoAdaptOffNoop`, `TestEvolutionAPI_TickAndRollback`).
- on + enough runs → instructions change; display_name / agent_key unchanged (`TestTick_AppliesFirstUnlockedPending`).
- name/key apply still 400 (`TestEvolutionAPI_Guardrail`, `TestApply_NameStillRejected`).
- rollback restores previous instructions (`TestTick_RollbackOnErrorRateDrop`).
- PATCH `locked: []` cannot unlock name/key (`TestEvolutionAPI_TickAndRollback`).
- SQLite persist (`TestStore_EvolutionGuardrailsDefaultAndPersist`).
- Existing 038 apply-to-instructions still works (`TestEvolutionAPI_Guardrail`).
- `GOSO_EVOLUTION_AUTO` default off (`TestAutoEnabled_DefaultOff`).

## Non-goals

Live quality ML. Changing SOUL.md files. Copying goclaw. Binding demo ports. Merge. SPEC 074.
