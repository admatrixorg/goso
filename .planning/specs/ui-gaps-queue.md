# UI-gaps queue — SPEC 057–065

> LOCKED: 2026-08-28. After SPEC 056 (Providers configure, `7b0f5a3`). Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091`. Coordinator may restart `:18080` / `:3000` after each merge.
> **Do not break** env `router9` + default `ocg/deepseek-v4-flash`.
> Sequential workers (one live at a time). Codex CTO audit (`audit-cto-034-056.md`) runs **in parallel** (docs-only).

Scan of live CP (`VITE_DEMO_MODE`) vs DEMO-only tabs + leftover surfaces after 043–056.

| SPEC | Title | Why now | Existing API (do not invent extra) |
|------|-------|---------|-------------------------------------|
| 057 | Agent editor | Create exists; PATCH `model`/`instructions` unused; English placeholders; empty key silently no-ops | `POST/GET/PATCH /api/agents/{id}` — **no DELETE**, **no PATCH display_name** |
| 058 | Session create | `POST /api/sessions` + `api.createSession` exist; SessionsPage is list-only; Chat empty = “chưa chọn phiên” with no New | `POST/GET /api/sessions` `{agent_id,label}` — **no DELETE/rename** |
| 059 | Chat session chrome | Compact session list has no create; no session title in chat header | reuse 058 POST; keep SSE `api.chatStream` |
| 060 | Command palette ⌘K | Sidebar “⌘K” is static chrome; `q` in App.tsx is unused | no new HTTP — jump live tabs only |
| 061 | Responsive shell | `minWidth: 1280`; sidebar 216 + chat 280 never collapse | CSS/layout only |
| 062 | Functions/cron UX | Cron card create silently returns if fields empty; no confirm delete | existing `/api/cron` — **no PATCH/enable** |
| 063 | Channels help | GET names + `configured` only; user cannot see which env to set | may add public `env` name on GET (never token). **No PATCH/secrets** |
| 064 | DEMO honesty | DEMO tabs (home/tasks/meetings/friends/calendar/gallery) stay mock; top search unused | no new HTTP; DemoBadge / hide dead search if 060 owns palette |
| 065 | Form validation sweep | Teams/Vault/Memory/Connectors/Marketing empty submit often silent | StatusLine + i18n; no new endpoints |

## Parked (not this queue)

- Channel token configure (needs secrets overlay like 056 — audit/DI).
- Webhook DELETE, session DELETE, agent DELETE (no HTTP).
- PATCH agent `display_name` (handler does not accept it).
- Settings CRM users/roles/nicks (talks `:8082` goso-crm — do not break that demo).
- Wiring DEMO mocks as live CRM.

## Worker / QC (every item)

Do not bind/kill 8082, 8091, 3000, 18080, 18088. Commit on `admatrixmdp/spec0NN-…`. `docs/qa/0NN-….md`. `npm run typecheck`. `go test ./...` + `go build` if Go changes. sibling `agpl-check.sh` 0. i18n vi+en. Coordinator merges then refreshes `:18080`/`:3000`.
