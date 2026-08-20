# PLAN 009 — Desktop (Wails v2 Skeleton)

> SPEC: `specs/009-desktop.md` (LOCKED 2026-08-20)

| # | Task | File | Verify |
|---|------|------|--------|
| T01 | Khung Wails (wails.json, main.go, frontend) | `desktop/wails.json`, `desktop/main.go`, `desktop/frontend/` | `wails build` hoặc `go vet` |
| T02 | Reuse gateway store (SQLite) | `desktop/` bind gateway logic | `wails dev` |
| T03 | Docs + verify | `desktop/README.md`, `make verify` | `make -C desktop verify` |

## Rationale

- **Wails v2**: giữ Go cho desktop, reuse gateway domain.
- **Reuse Control Plane pages**: tránh duplicate UI.
- **SQLite local**: đồng nhất với gateway file mode.

## Trạng thái

- [ ] T01 — khung Wails
- [ ] T02 — reuse gateway
- [ ] T03 — docs/verify
