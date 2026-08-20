# SPEC 009 — Desktop (Wails v2 Skeleton)

> LOCKED: 2026-08-20 — Bản desktop dùng Wails v2 + React + SQLite local, tái sử dụng gateway logic.

## Goal

Có khung **desktop** chạy được trên macOS/Windows: Wails v2 wrap Control Plane + gateway local (SQLite), mở cửa sổ hiển thị Control Plane.

## User stories

- **US-01** Chạy `wails dev` trong `desktop/` → cửa sổ desktop hiện Control Plane (agents/sessions/chat) như bản web.
- **US-02** Desktop lưu data vào SQLite local (`~/Library/Application Support/GOSO/goso.db` hoặc `data/goso.db`).

## Acceptance criteria

- [ ] AC-01 `desktop/wails.json` + `desktop/main.go` + `desktop/frontend/` (reuse Control Plane pages hoặc copy)
- [ ] AC-02 `wails build` sinh binary (ít nhất trên macOS)
- [ ] AC-03 Gateway logic tái sử dụng (store SQLite) — không duplicate domain
- [ ] AC-04 `make -C desktop verify` hoặc docs build xanh
- [ ] AC-05 Không commit binary lớn; `.gitignore` cho `build/bin`

## Non-goals

- Auto-update, code sign — để sau
- Bundle installer — để sau
- Tính năng desktop-only (tray, hotkey) — SPEC riêng

## Ghi chú

- Wails v2 + React + Go 1.25. Ưu tiên tái dùng `gateway/internal/*` làm lib.
- Nếu Wails chưa cần ngay, SPEC này có thể là stub (khung + docs + build script).
