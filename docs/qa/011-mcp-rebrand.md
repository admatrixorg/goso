# QA — SPEC 011 MCP rebrand (goso-mcp)

Date: 2026-08-29. Clean-room. No production tokens. Demos ports not bound or killed.

SPEC: `.planning/specs/011-mcp-rebrand.md` (LOCKED 2026-08-20). PLAN: `.planning/plans/011-mcp-rebrand.md`.

Code already on branch (`mcp/` name `goso-mcp`, `GOSO_GATEWAY_URL`). This QA closes plan checkboxes after verify. No recook of tools.

## Commands

Worktree `mcp/node_modules` may be a symlink; `pnpm -C mcp verify` can refuse install. Equivalent script:

```
cd mcp && npm run verify
```

(`tsc --noEmit && vitest run && tsup`)

2026-08-29: **21 tests pass**, tsup ESM build success.

## AC

| AC | Result | Evidence |
|----|--------|----------|
| AC-01 name/env `GOSO_*` | PASS | `mcp/package.json` `name: goso-mcp`; `src/version.ts` `SERVER_NAME`; `src/config.ts` prefers `GOSO_GATEWAY_URL` (legacy `GOCLAW_*` fallback). `tests/unit/version.test.ts`, `config.test.ts` |
| AC-02 verify xanh | PASS | `npm run verify` — 5 files / 21 tests; tsup `index` + `http` |
| AC-03 README Claude/Cursor | PASS | `mcp/README.md` stdio + Streamable HTTP, `GOSO_GATEWAY_URL` / `GOSO_TOKEN` |

Tool count 66 `goso_*` asserted in `version.test.ts`. Dual transport kept.
