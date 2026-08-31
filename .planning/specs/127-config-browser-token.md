# SPEC 127 — Config browser admin token (write-only)

Status: implemented (worker). Merge `--no-ff` and Vite `:3000` restart stay with advisor/CTO after SPEC 119–126 resume QC PASS (`de43d0e`).
Owner: Grok implementation; advisor/CTO merge and live QC.
Base: `origin/main` at `de43d0e`.

## Goal

Operators can give this Control Plane the gateway admin bearer so left-nav menus stop 401-ing, without opening DevTools and without the UI showing the secret again.

## Operator question

“How do I give this Control Plane the gateway admin token so menus stop 401-ing, without opening DevTools and without the UI showing the secret again?”

## Cause (verified)

Control Plane `authHeader()` in `control-plane/src/api/client.ts` reads `(import.meta.env.VITE_GOSO_ADMIN_TOKEN) || localStorage.goso_token`. Vite demo does not set `VITE_GOSO_ADMIN_TOKEN`. There is no login page. Config → Gateway → Auth previously showed only `token_set` / `view_token_set` / `master_key_set` booleans plus `settings.secretHint`. Gateway `GET /api/agents` and `GET /api/config` without a bearer return 401 `{"error":"unauthorized"}`.

This is not a per-menu permission bug.

## Behavior

Add a **browser session token** control on **Config → Gateway → Auth** (`SettingsPage` `page === "gateway"`), always visible even when gateway inventory is 401/blocked. Saving this field must not require a successful GET `/api/settings` or `/api/config`.

1. Password input, `autocomplete="off"`, empty on load. Never hydrate from localStorage, env, or any GET.
2. Status line (no secret value):
   - env-owned if `VITE_GOSO_ADMIN_TOKEN` is non-empty → input + Save/Clear disabled; env wins over localStorage (same as `authHeader()`).
   - else if `localStorage.goso_token` non-empty → “browser token set” (no body).
   - else → “browser token not set” (this is the 401 cause).
3. **Save**: trim, reject empty, `localStorage.setItem("goso_token", value)`, clear the input, then `location.reload()`. Do not PUT this value to `/api/settings` or `/api/config`. Do not send it to the gateway as a config field.
4. **Clear**: `localStorage.removeItem("goso_token")`, clear input, reload. Disabled when env-owned or when no browser token is set.
5. After save/clear, do not claim vendor/S3/Grafana/SSO/channel success. Optional non-secret probe from `GET /api/agents` status only: accepted / still unauthorized / unreachable. Never log or display the token or response bodies.
6. This field is the **Control Plane browser bearer**, distinct from gateway process `auth.token_set`. Keep both on the Auth card.
7. i18n vi + en. Do not copy GoClaw/Dewee wording.
8. Tests: empty-on-load, env-owned disables write, save writes localStorage and does not keep the typed value in state after save, clear removes the key, 401 inventory still allows this control. `npm test`, `npm run typecheck`, `npm run build` in `control-plane`. Both AGPL scripts exit 0.

## Constraints

- Clean-room React/TS. Inventory from live goso `:3000` + this spec.
- Secrets never in chat/QA/git (no token literals, no contents of process token files).
- Worker does not merge and does not restart Vite `:3000`. Never touch CRM `:8082` or sidecar `:8091`.

## Non-goals

- A separate login route (App chrome has none).
- Gateway Go auth, CRM, pairing, channel vendor tokens, or Backup.
- Auto-fill from a local token file.
- Copying GoClaw layouts/source.
- Merge `--no-ff` and Vite restart (advisor/CTO).

## Acceptance criteria

1. Config → Gateway → Auth always shows the write-only browser token control, including when `/api/config` is 401.
2. Input is empty on load; env-owned disables write; Save writes `goso_token` only to localStorage then reloads; Clear removes the key then reloads.
3. Gateway process `token_set` / `view_token_set` / `master_key_set` remain on the Auth card when inventory is available.
4. Probe copy, if shown, is status-only and never a vendor/S3/Grafana/SSO/channel success claim.
5. Vietnamese and English are complete.
6. `cd control-plane && npm test && npm run typecheck && npm run build` pass.
7. `GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh` and `./scripts/agpl-check-docs.sh` exit 0.
8. Delivery is two commits (feat vs docs) like SPEC 125/126. Merge stays with advisor/CTO.
