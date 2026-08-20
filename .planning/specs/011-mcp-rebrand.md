# SPEC 011 — MCP Rebrand (goso-mcp)

> LOCKED: 2026-08-20 — Đổi thương hiệu MCP server từ `goclaw-mcp` sang `goso-mcp`, giữ tương thích.

## Goal

MCP server hiện tại (`goclaw-mcp` — 66 tools, dual transport) được rebrand thành `goso-mcp`, trỏ tới GOSO gateway, vẫn giữ transport `stdio` + `Streamable HTTP`.

## User stories

- **US-01** `npx goso-mcp` chạy như `goclaw-mcp` cũ nhưng name/version là `goso-mcp`, gọi `GOSO_GATEWAY_URL` thay vì `GOCLAW_SERVER`.
- **US-02** Claude Code config trỏ `goso-mcp` vẫn quản trị gateway (agents/sessions/channels/providers).

## Acceptance criteria

- [ ] AC-01 Fork/copy `goclaw-mcp` → `goso/mcp` hoặc repo `admatrixorg/goso-mcp`, đổi `name`/`version`, env var `GOSO_*`
- [ ] AC-02 `pnpm -C mcp verify` xanh
- [ ] AC-03 Docs `mcp/README.md` hướng dẫn cấu hình Claude/Cursor

## Non-goals

- Thêm tool mới — giữ 66 tools cũ, alias sau
- Thay đổi gateway — giữ nguyên

## Ghi chú

- Có thể để `mcp/` là thin wrapper re-export `goclaw-mcp` + đổi env var, không fork toàn bộ ngay.
