# SPEC 023 — goso/mcp rebrand sạch (MIT)

> LOCKED: 2026-08-26. Lot 1 isolated. **Only `mcp/`**. MIT keep dual copyright.

## Goal

- Rename `src/client/goclaw-client.ts` → `goso-client.ts`; class `GoClawClient` → `GosoClient`.
- Tool names `goclaw_*` → `goso_*` in `src/tools/**`, prompts, resources, tests.
- README: tool names are `goso_*`; drop “stay goclaw_*”.
- LICENSE: keep MIT + goclaw-mcp contributors note.
- `pnpm -C mcp verify` xanh.

## AC

- [ ] AC-01 No file named `goclaw-client.ts`.
- [ ] AC-02 `rg 'goclaw_' mcp/src mcp/tests` empty (except LICENSE/README history sentence if needed — prefer zero).
- [ ] AC-03 `pnpm -C mcp verify`.
- [ ] AC-04 No edits outside `mcp/`.

## Non-goals

Gateway rewrite, new MCP tools, AGPL files.
