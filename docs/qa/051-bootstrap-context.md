# QA — SPEC 051 Bootstrap context files (SOUL / IDENTITY / AGENTS)

Date: 2026-08-27. Clean-room. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed.

Q4 was a cwd `AGENTS.md` stub in the prompt builder. This SPEC loads bootstrap markdown from `GOSO_CONTEXT_DIR` in the pipeline **prompt** stage.

## What changed

- Env `GOSO_CONTEXT_DIR` (optional). Empty/unset → no inject (not an error).
- When set: read **only** these filenames if present as **direct children**: `SOUL.md`, `IDENTITY.md`, `AGENTS.md` (optional `USER.md`). Nested paths, extra names, and `..` are ignored. Missing dir = no-op.
- Cap each file **32KiB** (truncate). Inject as extra system text labeled by filename (`SOUL.md:\n…`). Applied for full/task/minimal (same extra-system path as `TEAM.md`). `prompt_mode=none` skips bootstrap like team file and instructions.
- Do **not** overwrite agent identity fields (`display_name`, `agent_key`). Evolution apply still cannot rewrite those fields.
- The process-cwd `AGENTS.md` stub is gone: empty env no longer reads cwd.
- Control-plane Settings: one-line note (vi+en) explaining empty env = no inject vs set = labeled SOUL/IDENTITY/AGENTS/USER. No live gateway env probe.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- `gateway/internal/pipeline`: empty env no-op; missing dir no-op; `..` ignored; tempdir `SOUL.md` (+ IDENTITY/AGENTS/USER) labeled in `BootstrapText`; extra/nested names skipped; symlink escape ignored; 32KiB cap.
- Scripted LLM `Runner.Run`: tempdir `SOUL.md` appears in the system message; agent `display_name` / `agent_key` unchanged; missing dir and path escape do not inject.

## Non-goals

Parsing IDENTITY.md into identity fields, MEMORY.md / USER.md as a required file, cwd AGENTS.md fallback, live demo bind/kill, copying goclaw/ZaloCRM.
