# SPEC 024 — Desktop Wails (close SPEC 009 AC)

> LOCKED: 2026-08-26. Lot 1 isolated under `desktop/`. Auth SPEC 016: empty token is 401 unless `GOSO_DEV_MODE=1` **or** desktop sets a local admin token on first run.

## Goal

`wails build` (darwin) produces a runnable app showing Control Plane skin with **5 live tabs**: Agent, Phiên, Chat, Kết nối, Nhật ký (plus CRM metrics if URL set). Document `scripts/run-desktop.sh`. Code-sign / installer = **non-goal** (note in README).

## AC

- [ ] AC-01 `make -C desktop verify` or documented `wails build` exit 0 on this Mac.
- [ ] AC-02 Desktop gateway: healthz 200; `/api/agents` needs token unless GOSO_DEV_MODE=1 (prefer generate+store local token in app support dir, never log it).
- [ ] AC-03 UI lists the 5 real tabs; DEMO marketing/home hidden (reuse VITE_DEMO_MODE=false).
- [ ] AC-04 README: unsigned build; no notarization.

## Non-goals

Code-sign, dmg installer, Windows/Linux CI, Zalo OAuth.
