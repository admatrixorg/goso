# SPEC 064 — DEMO chrome honesty

> LOCKED: 2026-08-28. Clean-room React. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

`VITE_DEMO_MODE` still mounts home / tasks / meetings / friends / calendar / gallery from `demo/mock.ts`. Live tabs (agents…traces) talk to `:18080`. Risk: chrome looks like one product; mock actions look clickable as if they persist.

## UI

- Every DEMO-only page already using `DemoBadge` must keep it; add if missing on those six.
- Do not wire `demo/mock.ts` into live Agents/Sessions/Chat/Providers/etc.
- After 060, if header search is still a no-op, **hide it** in both DEMO and live (palette owns search).
- Settings stays live CRM-proxy + theme (033). Do not re-add placeholders nav.
- i18n: one line on DEMO home that live Agent/Chat/Providers talk to the gateway.

`docs/qa/064-demo-honesty.md`. Commit `admatrixmdp/spec064-demo-honesty`. Do not merge.

## QC

`cd control-plane && npm run typecheck` · agpl-check 0.

## Non-goals

Replacing DEMO CRM with live goso-crm pages, deleting DEMO mode.
