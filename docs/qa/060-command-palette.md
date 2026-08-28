# QA — SPEC 060 Command palette (⌘K)

Date: 2026-08-28. Clean-room React in `control-plane/`. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not break env `router9` + default `ocg/deepseek-v4-flash`.

`App.tsx` sidebar showed a static “⌘K” chip (`chrome.quickSearch`) that did nothing. Header `q` was written but never filtered. Users could not keyboard-jump tabs.

## Wired

- `⌘K` / `Ctrl+K` opens a dialog listing **visible** tabs for this mode (live sidebar + Settings; DEMO adds home/tasks/meetings/friends/calendar/gallery via existing `side` construction). Filter by shared `q` on label or tab id.
- Enter / click → `go(id)` and close. Escape closes. Overlay click closes. Focus trap not required.
- Sidebar chip is a real `<button>` (`aria-haspopup="dialog"`) that opens the same palette.
- Header search is the same `q`: focus or type opens the palette and filters it. Palette owns the focused search input. Close clears `q`.
- Do **not** search CRM meetings / mock fixtures — tab labels/ids only.
- i18n vi+en: `palette.title`, `palette.empty`, `palette.hint`.

## Not wired (no API / out of scope)

- Full-text message search, server search API, vim mode.
- New HTTP routes — none.
- SPEC 061 responsive shell.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Non-goals

Full-text message search, server search API, vim mode, SPEC 061, binding/killing demo ports.

## Proof (2026-08-28)

- `cd control-plane && npm run typecheck` → exit 0 (`tsc --noEmit`, no errors)
- `go test ./...` → exit 0 (24 ok, 3 no-test pkgs, 0 fail). No `*.go` / `gateway/*` in the SPEC 060 diff.
- `GOSO_ROOT=$PWD …/agpl-check.sh` → exit 0 (`OK`; no banned author ids outside `.planning`)
- Diff: `control-plane/src/App.tsx`, `control-plane/src/ui/CommandPalette.tsx` (new), `control-plane/src/i18n/en.ts`, `control-plane/src/i18n/vi.ts`, this QA file. No DEMO page diffs. No vite/dev server started. Ports `:8082` `:8091` `:3000` `:18080` `:18088` not bound/killed (pre-existing listeners left alone).
- Source: `CommandPalette` window `metaKey||ctrlKey` + `k` toggles dialog (`e.repeat` ignored so a held key does not open-then-close). Overlay click closes only when `target === currentTarget`. Escape calls `closePalette`. Enter/click → `go(id)` then close. Sidebar chip is `<button type="button" aria-haspopup="dialog">`. Header search `q` onFocus/onChange opens the same palette; close blurs the header and skips one focus restore so Escape/overlay does not bounce the dialog back open. Filter is `label`/`id` includes only (no CRM/mock fixtures). Close clears `q`. Visible items = unique `side` tabs + Settings (`visibleTabItems`). DEMO extras only when `DEMO` builds `side` with `demoOverviewItems` / `demoWorkExtra`.
- i18n en+vi: `palette.title`, `palette.empty`, `palette.hint` present. Key sets equal (418/418). `en: Record<MsgKey, string>` typecheck covers match.
- Env router9 + default `ocg/deepseek-v4-flash` untouched (no gateway/llm files in diff).
- No control-plane unit tests (no CP test runner).
