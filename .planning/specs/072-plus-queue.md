# SPEC 072+ candidate queue (after 071)

> Drafted 2026-08-28 from matrix `docs/qa/034-goclaw-parity-matrix.md`, appendix `docs/qa/034-goclaw-parity-update-054.md`, audit `docs/qa/audit-cto-2026-08-28.md`.
> **Re-scan evidence on main after 071** before locking numbers. Only THIẾU/PARTIAL that are clean-room and not DI-credential/host.

Priority: user-visible function → security → ops.

## Proposed (adjust on announce)

| SPEC | Rows | GoClaw cite (docs only) | goso gap | Park? |
|------|------|-------------------------|----------|-------|
| **072** Memory L2 KG + progressive load on **FTS5/Lite** | M3, M4, K4 | `docs/07-bootstrap-skills-memory.md` §17 L2 `kg_entities`/`kg_relations`, progressive `memory_search` / `memory_expand` | THIẾU L2; FTS5 exists (D5). pgvector = DI-09 documented in 071, do not block | no |
| **073** Self-evolution auto-adapt + guardrails | E1 | README self-evolution; `docs/21-agent-evolution-and-skill-management.md` | 038 suggestions only, not auto-adapt; never change identity/name/core purpose | no |
| **074** Tools depth: filesystem six-names, web fail-closed real, media fail-closed | K1, K3, K5 | `docs/03-tools-system.md` | 050 two file tools; web_search DDG fail-closed; media stub | browser/exec spawn = DI-12/13 park |
| **075** Skills BM25 + manage | K6 | `docs/14-skills-runtime.md`, `docs/15-core-skills-system.md` | `use_skill` loader only | no |
| **076** Prompt: full `cache_control` + persisted `prompt_mode` | Q2, Q3 | `docs/01-agent-loop.md` 4-mode; `docs/02-providers.md` cache | Anthropic cache only when CacheMode=full; mode not on session | no |
| **077** Pairing codes + RBAC matrix | C9, N5 | `docs/20-api-keys-auth.md`, `docs/23-ai-agent-permission-matrix.md` | origin allowlist only; view-token GET-only | live IdP = DI-19 park |
| **078** Channel config depth (UI/env names, no live tokens) | C1–C7 PARTIAL | `docs/05-channels-messaging.md` | adapters exist; live tokens DI-01..07 | **do not fill secrets** |
| **079** Injection pattern completeness | S2, S7 | `docs/09-security.md` | four of six patterns | after 066 prod default |
| **080** Cron heartbeat / sessions_* agent tools | K8 | `docs/08-scheduling-cron.md`, `docs/22-heartbeat-system.md` | ticker exists; no heartbeat tools | no |

## Stay parked (report when queue empty)

| ID | Why |
|----|-----|
| DI-01..07 | live channel credentials / native WhatsApp |
| DI-08 | paid search vendor |
| DI-09 | pgvector **host** (path documented in 071; FTS5 L2 in 072) |
| DI-10, DI-18 | OTEL collector / Grafana SaaS |
| DI-11 Tailscale, DI-12 sandbox image, DI-13 headless Chrome | |
| DI-14 Redis, DI-15 notarize/auto-update, DI-16 Stripe, DI-17 K8s, DI-19 SSO | |
| W3 CRM outbound | CẮT 033 |
| F1–F5, F7 WITH_* / Helm | CẮT unless Dat un-cuts |
| X4 DomainEventBus | THIẾU — only if still wanted after 080; not user-visible |

Provider stream leftover after 068, if any, folds into 076/R1 follow-up rather than a new empty SPEC.
