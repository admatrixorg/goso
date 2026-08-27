# SPEC 044 — Decision items (self-audit, self-decide)

Date: 2026-08-27. User delegated remaining DIs to the coordinator. No vendor spend. No product secrets in git.

| ID | Topic | Decision | Implement now | Notes |
|----|-------|----------|---------------|-------|
| DI-20 / model | Live LLM | **9Router** `http://127.0.0.1:20127/v1`, named provider `router9`, default model `cx/gpt-5.6-sol`. Key optional empty. Construct when `GOSO_ROUTER9_BASE_URL` set. | Yes (SPEC 044) | User-locked. `/v1/models` 200 (no auth). Chat **401 token_expired** — Codex `~/.codex/auth.json` mtime 2026-08-24. **Not a goso bug.** Fix = user `codex login`, then re-smoke. Do not fake a sol completion. |
| DI-search | `web_search` tool | Default **OFF** fail-closed. Enable with `GOSO_WEB_SEARCH=ddg` (DuckDuckGo Instant Answer, no key). Router `search:true` is a **model capability**, not a GOSO HTTP search API. | Yes (tool + flag) | No Brave/SerpAPI keys. |
| DI-09 | Gateway Postgres / pgvector | **Keep SQLite + FTS5** (036/037). Do not switch default store. | Docs only | Upgrade path: host Postgres 16 + `CREATE EXTENSION vector`; dual-write embeddings later; FTS5 remains lexical fallback. Blocked on **host providing** PG — parked, does not block 044. |
| DI-12 | Docker sandbox | **OFF** default. Tool `sandbox` → `not_configured`, no spawn. | Stub + UI flag | Needs image + policy later. |
| DI-browser | Browser overlay | **OFF**. `browser` → `not_configured`, no Chrome. | Stub + UI flag | `WITH_BROWSER` non-goal. |
| DI-media | Media overlay | **OFF**. `media` → `not_configured`, no ffmpeg. | Stub + UI flag | |
| DI-01..07 | Channel tokens | Adapters already shipped (040). UI `configured: false`. **Do not auto-fill.** | No | User supplies tokens later. |
| DI-19 | OAuth IdP / CRM SSO | **Skip (non-goal).** | No | Documented. |
| DI-apple | Apple notarize | **Skip.** | No | Unsigned zip remains 029. |
| DI-stripe | Stripe live charging | **Skip.** Count quota only. | No | |
| DI-k8s | Kubernetes overlay | **Skip.** | No | Compose/binary stay. |
| DI-18 | Grafana Cloud / OTLP SaaS | **Skip.** OTLP only if `GOSO_OTEL_ENDPOINT` (042). | No | |
| DI-tailscale | Tailscale | **Skip.** | No | |
| DI-redis | Redis overlay | **Skip.** In-memory rate limit stays. | No | |

## pgvector upgrade path (parked)

1. Provision Postgres 16 with `vector` extension (host decision).
2. New store backend behind `GOSO_DB_DRIVER=postgres` — SQLite remains default.
3. Vault/memory: keep FTS5/SQL `LIKE` for lexical; add embedding column + kNN later.
4. Do **not** block desktop Lite or current demo on PG.
