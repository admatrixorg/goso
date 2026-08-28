# QA — SPEC 064 DEMO chrome honesty

Date: 2026-08-28. Clean-room React in `control-plane/`. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not break env `router9` + default `ocg/deepseek-v4-flash`.

`VITE_DEMO_MODE` still mounts home / tasks / meetings / friends / calendar / gallery. Live tabs talk to `:18080`. Risk: chrome looks like one product; mock actions look clickable as if they persist.

## Wired

- `DemoBadge` on all six DEMO pages: home, tasks, meetings, friends, calendar, gallery.
- DEMO home shows `home.liveGateway` (vi+en): live Agent/Chat/Providers talk to the gateway; this page is mock and does not persist to CRM.
- `demo/mock.ts` imports stay on Home/Tasks/Meetings/Friends only.
- Header search still opens CommandPalette (`onFocus` / `onChange`). Not hidden — after 060 it is not a no-op.
- Settings still CRM-proxy (`settingsApi` → `crmRequest`) + theme toggle. No placeholders nav item.

## Not wired (no API / out of scope)

- Replacing DEMO CRM with live goso-crm pages.
- Deleting DEMO mode.
- Disabling mock-looking buttons on DEMO pages.
- New HTTP routes — none.
- Gateway / LLM changes — none.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Non-goals

Replacing DEMO CRM with live goso-crm pages, deleting DEMO mode, binding/killing demo ports.

## Diff-aware mode

Analyzed 2 changed files vs `HEAD` (`admatrixmdp/spec064-demo-honesty` = `5782c5f` + unstaged).

- **Changed:** `control-plane/src/demo/i18n.ts`, `control-plane/src/pages/HomePage.tsx`
- **Mapped:** none (no CP test runner; no co-located `*.test.ts`)
- **Unmapped:** both files — [!] no tests for `HomePage` / `demoText` `home.liveGateway`
- **Not in diff:** `App.tsx`, Settings, live pages, `gateway/`, `llm/` — static-verified only
- Ran Go suite (no mapped CP tests; Go files unchanged): 24 ok, 0 fail, 3 no-test pkgs

## Acceptance checks

| # | Check | Result | Evidence |
|---|--------|--------|----------|
| 1 | DemoBadge on home/tasks/meetings/friends/calendar/gallery | **PASS** | Home `21`, Tasks `20`, Meetings `19`, Friends `13`+`32`, Calendar `29`, Gallery `17` |
| 2 | `demo/mock.ts` only on Home/Tasks/Meetings/Friends; never Agents/Sessions/Chat/Providers | **PASS** | 4 importers only; live pages 0 matches |
| 3 | Header search still opens CommandPalette (`onFocus`/`onChange`) | **PASS** | `App.tsx` `277–284` `setPaletteOpen(true)` / `openPalette()`; palette mounted `529–541`. File not in 064 diff |
| 4 | Settings uses settingsApi (CRM-proxy) + theme | **PASS** | `settingsApi` from `api/settings.ts` (`crmRequest`). Menu: account/users/roles/nicks/quotas/templates/billing/theme. Theme toggle `417–424`. No `id: "placeholders"` nav |
| 5 | `demo/i18n.ts` `home.liveGateway` in vi and en | **PASS** | vi `5–6`, en `58–59`. Key sets 48/48 equal. Home renders `d("home.liveGateway")` at `24` |
| 6 | No gateway/llm file changes | **PASS** | `git diff --name-only` = the two CP files only |
| 7 | `cd control-plane && npm run typecheck` | **PASS** | exit **0** (`tsc --noEmit`, no errors) |
| 8 | `go test ./...` | **PASS** | exit **0** (24 ok cached, 3 no-test, 0 fail) |
| 9 | agpl-check | **PASS** | exit **0** (`OK`; no banned author ids outside `.planning`) |
| 10 | Did not bind/kill reserved ports; no vite/dev server | **PASS** | PIDs unchanged after QC: 8082 `goso-crm 85417`, 8091 `node 83346`, 3000 `node 65863`, 18080 `goso-gate 1553`, 18088 `goso-gate 39199` |

## Test Results Overview

- **Total Go pkgs:** 27 (`go test ./...`)
- **Passed:** 24 | **Failed:** 0 | **No test files:** 3 (`gateway/cmd/goso-gateway`, `internal/config`, `internal/health`)
- **CP unit tests:** none (no runner)
- **Duration:** Go cached (sub-second per pkg); `tsc --noEmit` clean

## Coverage Metrics

| Metric | Value | Threshold | Status |
|--------|-------|-----------|--------|
| CP lines | n/a — no CP test runner | 80% | N/A |
| Go | not collected (`-cover` not in spec QC) | — | N/A |
| `home.liveGateway` | type-enforced `en: Record<keyof typeof vi, string>` | — | PASS |

Uncovered: HomePage liveGateway render; DemoBadge presence. No CP tests existed pre-064.

## Failed Tests

None.

## Performance Metrics

- Typecheck: `tsc --noEmit` exit 0
- Go tests: all cached, 0 fail
- No slow tests identified
- No vite/dev server started

## Build Status

- **typecheck:** PASS (exit 0)
- **go test:** PASS (exit 0)
- **agpl-check:** PASS (exit 0)
- **Warnings:** none
- **Dependencies:** resolved (`control-plane/node_modules` present)

## Critical Issues

None blocking.

## Recommendations

1. **Low** — residual honesty gap out of 064 scope: DEMO buttons still look armed with no persist (`home.addSource`, `meetings.upload`, `cal.create`, `gallery.upload`, `friends.refresh`; home `agentChips` `cursor:pointer`). Spec only required DemoBadge + one home line.
2. **Low** — Calendar/Gallery do not import `demo/mock.ts` (inline empty placeholders). Matches tester mapping (mock only Home/Tasks/Meetings/Friends).
3. **Low** — add CP tests if a runner appears: i18n key parity, DemoBadge on six DEMO routes, live pages must not import `demo/mock`.

## Next Steps

1. Commit on `admatrixmdp/spec064-demo-honesty`. Do not merge.
2. No gateway/llm follow-up.

## Proof (2026-08-28)

- `cd control-plane && npm run typecheck` → exit 0 (`tsc --noEmit`, no errors)
- `go test ./...` → exit 0 (24 ok, 3 no-test pkgs, 0 fail). No `*.go` / `gateway/*` / `llm/*` in the SPEC 064 diff.
- `GOSO_ROOT=$PWD …/agpl-check.sh` → exit 0 (`OK`; no banned author ids outside `.planning`)
- Diff: `control-plane/src/demo/i18n.ts` (`home.liveGateway` vi+en), `control-plane/src/pages/HomePage.tsx` (renders that line). No vite/dev server started. Ports `:8082` `:8091` `:3000` `:18080` `:18088` not bound/killed (pre-existing listeners left alone).
- Source: DemoBadge on six DEMO pages. `demo/mock.ts` only Home/Tasks/Meetings/Friends. Header search `onFocus`/`onChange` opens palette. Settings `settingsApi` + theme. No placeholders nav.
- Env router9 + default `ocg/deepseek-v4-flash` untouched (no gateway/llm files in diff).
- No control-plane unit tests (no CP test runner).

## Unresolved Questions

- None for 064 acceptance. Residual mock-button clickability is documented non-goal, not a fail.
