# SPEC 051 — Bootstrap context files (SOUL / IDENTITY / AGENTS)

> LOCKED: 2026-08-27. Clean-room Go. No copy from goclaw-source or ZaloCRM. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

Q4 THIẾU: pipeline prompt stage does not inject bootstrap markdown.

## Behavior

- Env `GOSO_CONTEXT_DIR` (optional). Empty → no inject (not an error).
- When set: read **only** these filenames if present: `SOUL.md`, `IDENTITY.md`, `AGENTS.md` (and optional `USER.md`). Cap each 32KiB. Jail: files must be direct children of the dir (no `..`).
- Inject into pipeline **prompt** stage as extra system text, labeled by filename. Do **not** overwrite agent identity fields (`display_name`, `agent_key`). Evolution guardrail still applies.
- Tests: tempdir with SOUL.md appears in prompt/system messages (scripted LLM or captured pipeline ctx); missing dir = no-op; path escape ignored.

## UI (optional, keep small)

Settings or Functions note: “context dir set/not set”. Or skip UI if only env. Prefer a one-line on Settings if easy. i18n vi+en if UI.

`docs/qa/051-bootstrap-context.md`. Commit `admatrixmdp/spec051-bootstrap`. Do not merge.
