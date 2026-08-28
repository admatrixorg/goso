# SPEC 060 — Command palette (⌘K)

> LOCKED: 2026-08-28. Clean-room React. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

`App.tsx` sidebar shows a static “⌘K” chip (`chrome.quickSearch`) that does nothing. Header `q` state is written but never filters. Users cannot keyboard-jump tabs.

## UI

- `⌘K` / `Ctrl+K` opens a small palette (dialog) listing **visible** tabs for this mode (live vs DEMO). Filter by `q`. Enter / click → `go(id)` and close.
- Escape closes. Overlay click closes. Focus trap not required if Escape works.
- Sidebar chip is a real button (opens the same palette).
- Header search box: typing filters the palette **or** filters sidebar labels; do not leave a dead `q`. Prefer one search (palette). If header search stays, it must do the same filter.
- Do **not** search CRM meetings / mock fixtures. Live tabs only + DEMO tabs when `VITE_DEMO_MODE`.
- i18n vi+en (`palette.title`, empty, hint).

`docs/qa/060-command-palette.md`. Commit `admatrixmdp/spec060-command-palette`. Do not merge.

## QC

`cd control-plane && npm run typecheck` · `go test ./...` · agpl-check 0.

## Non-goals

Full-text message search, server search API, vim mode.
