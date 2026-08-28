# QA — SPEC 057 Agent editor (model + instructions)

Date: 2026-08-28. Clean-room React in `control-plane/`. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not break env `router9` + default `ocg/deepseek-v4-flash`.

Today `AgentsPage` could POST `{agent_key, display_name}` and PATCH `orchestration_mode` only. Empty `agent_key` silently returned. Placeholders were raw English. `PATCH /api/agents/{id}` already accepted `model` and `instructions`; the form never sent them.

## Wired

- Click a list row → in-page edit panel (no new route). Fields: `display_name` (create-only; read-only on edit), `model`, `instructions` (textarea), orchestration mode.
- Create: trim `agent_key`. Empty → `StatusLine` error `agents.needKey`, no POST. Optional `model` + `instructions` on POST.
- Save edit: `PATCH /api/agents/{id}` `{model?, instructions?, orchestration_mode?}`. StatusLine loading/error. Refresh list.
- i18n vi+en for labels, placeholders, `agents.needKey` (human copy, not raw `agent_key`). No leftover English `agent_key` / `display_name` placeholders.
- Client `createAgent` now types optional `instructions` and `orchestration_mode`.

## Not wired (no API / out of scope)

- DELETE agent — handler does not exist; no delete control.
- PATCH `display_name` — `handlePatchAgent` does not accept it; edit field is disabled.
- New Go endpoints — none. Existing POST/GET/PATCH only.
- API keys — not shown.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Non-goals

DELETE agent, PATCH display_name, new Go endpoints, CRM UI, DEMO tab changes, SPEC 058, binding/killing demo ports.

## Proof (tester 2026-08-28)

- `cd control-plane && npm run typecheck` → exit 0
- `go test ./...` → exit 0 (24 ok, 3 no-test pkgs, 0 fail). No `*.go` diff vs origin/main.
- `GOSO_ROOT=$PWD …/agpl-check.sh` → exit 0
- DEMO pages Home/Tasks/Meetings/Friends/Calendar/Gallery: no diff
- Full tester report: [260828-1256-spec057-agent-editor-qa.md](./260828-1256-spec057-agent-editor-qa.md)
