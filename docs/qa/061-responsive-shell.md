# QA — SPEC 061 Responsive control-plane shell

Date: 2026-08-28. Clean-room React in `control-plane/`. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not break env `router9` + default `ocg/deepseek-v4-flash`.

Root shell was `minWidth: 1280` + `overflow: hidden`. Sidebar 216px and Chat session column 280px never collapsed. Below ~1280px the UI clipped.

## Breakpoints

| Width | Shell | Sidebar | Chat | Tables |
|-------|-------|---------|------|--------|
| ≥1280 | `width: 100%`, `min-width: 0` (no 1280 floor). Looks like today: sidebar 216px with labels, chat sessions 280px column, top tabs visible. | 216px labeled | 280px column left of transcript | no extra scroll if card is wide enough |
| 900–1279 | same layout, chrome may scroll in the topbar | 216px labeled | 280px column | card `overflow-x: auto` if a row is wider than the card |
| **&lt;900** | top tabs / brand wordmark / theme caption hidden; current tab stays in the icon rail | **52px icon-only** (`aria-label` + `title` on each item) | 280px column until 800 | horizontal scroll inside the card (`TableScroll`, inner min-width 640px) |
| **&lt;800** | settings inner rail, teams 280+detail, vault 2-col, friends DEMO rail stack vertically | icon-only | **compact sessions stacked above** the transcript (`max-height: 38%`) | same |
| **&lt;480** | header search and gateway chip hidden (palette still from sidebar search / ⌘K) | icon-only | stacked | same |

Coordinator browser-checks **1280** and **~390** after merge. At ~390: icon rail remains; chat stacks; tables scroll inside the card so action buttons are not clipped.

## Wired

- Removed `minWidth: 1280` on the shell and `--view-min-width: 1280px` on `html/body`. Shell is `100%` / `min-width: 0`.
- `<900px`: sidebar collapses to icon-only. Group titles and labels hide. Current tab is still a reachable icon (`aria-current="page"`).
- Chat `<800px`: compact `SessionsPage` sits **above** the transcript, not a clipped 280px column. Composer input is `min-width: 0`; the Agent chip label hides with `.z-wide-only` so send stays on-screen at ~390.
- Providers and other row-tables wrap in `TableScroll` so the card scrolls horizontally; cron/tools/settings/marketing action buttons stay on the row.
- Marketing inner 210px rail uses the same stack-as-Settings split below 800. KPI cards wrap.
- DEMO mock pages were not redesigned. Friends inner rail stacks below 800; Meetings and Calendar tables/grids scroll inside the card.
- Desktop ~1280 still shows the labeled 216px sidebar and 280px chat column.

i18n vi+en: `chrome.nav`.

## Not wired (out of scope)

- New design system, native mobile app, gateway / `router9` / `ocg/deepseek-v4-flash` changes.
- Hamburger overlay (icon-only satisfies the spec’s “or”).
- SPEC 062 Functions/cron UX.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Non-goals

New design system, mobile native, changing gateway, SPEC 062, binding/killing demo ports.

## Proof (2026-08-28)

- `cd control-plane && npm run typecheck` → exit 0 (`tsc --noEmit`, no errors)
- `go test ./...` → exit 0 (24 ok, 3 no-test pkgs, 0 fail). No `*.go` / `gateway/*` in the SPEC 061 diff.
- `GOSO_ROOT=$PWD …/agpl-check.sh` → exit 0 (`OK`; no banned author ids outside `.planning`)
- Diff: shell/CSS (`App.tsx`, `styles/index.css`, `tokens/spacing.css`), `TableScroll` in `ui/Card.tsx`, i18n `chrome.nav`, table wraps on live list cards, overflow-only Friends/Meetings, this QA file. No gateway/llm files. No vite/dev server started. Ports `:8082` `:8091` `:3000` `:18080` `:18088` not bound/killed (pre-existing listeners left alone).
- Source: shell class `z-shell` has no `minWidth: 1280`. `--view-min-width` is `0`. `@media (max-width: 899px)` sets `.z-sidebar { width: 52px }` and hides `.z-wide-only` labels. `@media (max-width: 799px)` sets `.z-chat-split { flex-direction: column }` with `.z-chat-sessions` on top (`max-height: 38%`). `.z-row-table { min-width: 640px }` inside `.z-scroll-x`. At 1280px none of those media queries apply (sidebar 216, chat sessions 280).
- i18n en+vi: `chrome.nav` present. Key sets equal (419/419). `en: Record<MsgKey, string>` typecheck covers match.
- Env router9 + default `ocg/deepseek-v4-flash` untouched (no gateway/llm files in diff).
- No control-plane unit tests (no CP test runner).
