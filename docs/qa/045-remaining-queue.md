# Remaining SPEC queue (post-044)

Date: 2026-08-27. Coordinator audit of matrix 034 (stale as-of 035–044) + QA 035–044 + live demo. Parked DIs do **not** block.

Live smoke (env, before 045 merge): `POST /api/chat` router9 `ocg/deepseek-v4-flash` → **200** `reply=OK` (~1.6s). Codex/`cx/*` out of default test loop.

| SPEC | Item | Why |
|------|------|-----|
| **045** | Catalog default `ocg/deepseek-v4-flash` + **restart-safe IDs** | User-locked model. `newID()` seq reset 502s chat after gateway restart. |
| **046** | CP live-tab loading/empty/error; Chat surfaces 502; DEMO tabs stay gated | Functions/Teams/Vault already have EmptyState; Chat/loading gaps; DEMO home/tasks/meetings/friends/calendar/gallery stay `VITE_DEMO_MODE` only — do not silently wire mocks as live. |
| **047** | `GET /api/webhooks` list + Webhooks page registry | 043 documented missing GET; user asked CRUD registry. |
| **048** | Agents/Teams UI: `orchestration_mode` auto/explicit/manual | Field on POST `/api/agents`; no PATCH agent + no picker. |
| **049** | Skills loader + `use_skill` (K6) | THIẾU. Fail-closed without `GOSO_SKILLS_DIR`. |
| **050** | Filesystem tools in `GOSO_WORKSPACE` jail (K1) | THIẾU. No spawn outside jail. |
| **051** | Bootstrap context files SOUL/IDENTITY/AGENTS (Q4) | THIẾU. Inject into pipeline prompt stage. |
| **052** | MCP `/v1/*` aliases to gateway `/api/*` | R16/M5/T5/K10 PARTIAL. |
| **053** | Chat SSE in control-plane | D3 PARTIAL. Gateway stream reader exists (039). |
| **054** | Cron / heartbeat tools (K8) | THIẾU. SQLite schedule, fail-closed. |
| **055** | Refresh matrix 034 + optional e2e router9 + desktop lite channel hide | Docs/QC drift. |

## Parked (do not block queue)

pgvector (DI-09), sandbox/browser/media spawn (stubs in 044), pairing (C9), OAuth/Apple/Stripe/K8s/Grafana/Tailscale/Redis, live channel tokens (DI-01..07), Codex `cx/*` as default.
