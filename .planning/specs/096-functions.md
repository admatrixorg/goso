# SPEC 096 — Functions operator surface (skills, tools, MCP, cron)

> After 095. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 096. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Skills / Built-in Tools / MCP Servers / Cron.

## Goal

Reshape **FunctionsPage** into a coherent capabilities surface: per-agent tool grants and approval flags; skill inventory/create/archive/search; MCP/connector registration with connection testing; cron list/create/edit/run history. MCP env values and connector tokens stay write-only. Separate global configuration from agent grants. Explicit empty/error/disabled states per capability.

## AC

- [ ] Tools: per-agent grant/enable, `requires_approval` visible, configured vs not_configured. Do not expose connector tokens in tool lists.
- [ ] Skills: search (existing BM25), create, delete/archive with confirm, empty/error. No script exec.
- [ ] Connectors/MCP: list + add (stdio / SSE / HTTP as already stored transports). Test connection. Env/token fields write-only; GET never returns token values. Env-owned/disabled states explicit.
- [ ] Cron: list/create/enable, session bind, last-run/error if present, empty/error. No live vendor secrets.
- [ ] i18n vi+en. Loading/empty/error per card. CP typecheck. Tests. agpl 0. `docs/qa/096-functions.md`.

## Out of scope

TTS page (118). Packages (114). Copying GoClaw dialogs. Live paid MCP marketplaces. Nodes/Workstations.
