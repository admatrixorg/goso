# SPEC 061 — Responsive control-plane shell

> LOCKED: 2026-08-28. Clean-room React. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

Root shell is `minWidth: 1280` + `overflow: hidden`. Sidebar 216px and Chat session column 280px never collapse. Below ~1280px the UI is clipped.

## UI

- Remove `minWidth: 1280`. Allow `100%` / `min-width: 0`.
- `<900px`: collapse sidebar to icon-only **or** a hamburger that toggles overlay drawer. Current tab remains reachable.
- Chat: below ~800px stack compact sessions **above** the transcript (not a clipped 280px column).
- Providers / tables: horizontal scroll inside the card is OK; do not clip action buttons.
- Do not rewrite DEMO mock pages’ internal grids except if they overflow the new shell.
- Check desktop (~1280) still looks like today (sidebar visible).

`docs/qa/061-responsive-shell.md` notes breakpoints. Commit `admatrixmdp/spec061-responsive`. Do not merge.

## QC

`cd control-plane && npm run typecheck` · agpl-check 0. Coordinator browser-checks 1280 and ~390 widths after merge.

## Non-goals

New design system, mobile native, changing gateway.
