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

- [ ] T01 — skeleton
- [ ] T02 — env rebrand
- [ ] T03 — docs/verify
