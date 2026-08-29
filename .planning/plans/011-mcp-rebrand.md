# PLAN 011 — MCP Rebrand

> SPEC: `specs/011-mcp-rebrand.md` (LOCKED 2026-08-20)

| # | Task | File | Verify |
|---|------|------|--------|
| T01 | Copy/fork mcp skeleton | `mcp/package.json`, `mcp/src/*` | `pnpm -C mcp build` |
| T02 | Đổi env var GOSO_* + name | `mcp/src/config.ts` | `pnpm -C mcp test` |
| T03 | Docs + verify | `mcp/README.md` | `pnpm -C mcp verify` |

## Rationale

- **Thin wrapper re-export**: nhanh, giữ 66 tools.
- **Đổi env GOSO_***: tránh nhầm với GoClaw.
- **Dual transport giữ nguyên**: tương thích Claude/Cursor.

## Trạng thái

- [x] T01 — skeleton
- [x] T02 — env rebrand
- [x] T03 — docs/verify

## QA 2026-08-29

Code already present; no recook. Evidence: `docs/qa/011-mcp-rebrand.md`.

| Task | Proof |
|------|--------|
| T01 | `mcp/package.json` `goso-mcp`; `pnpm`/`npm run verify` build |
| T02 | `src/config.ts` `GOSO_GATEWAY_URL`; `tests/unit/config.test.ts` |
| T03 | `mcp/README.md`; `cd mcp && npm run verify` 21 tests |
