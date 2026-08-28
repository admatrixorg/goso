# SPEC 034 appendix — status after 035–054

Date: 2026-08-28. Clean-room. Does **not** rewrite `docs/qa/034-goclaw-parity-matrix.md` (historical snapshot at goso `ef42f77`). This file is the post-queue refresh (SPEC 055). Parked DIs stay parked.

Compared against the original 034 table. **Now** is honest CÓ / PARTIAL / THIẾU / CẮT after SPECs 035–054. Evidence paths are the owning QA doc plus the implementation file opened for that row.

Status meanings are unchanged from 034: **CÓ** working (may be simpler); **PARTIAL** stub / one-of-many / fail-closed / half; **THIẾU** never built; **CẮT** scoped out (parked DI).

---

## Closed-row honesty

035–054 closed the 034 group plan plus the remaining queue (045–054). Rows that 034 called THIẾU/PARTIAL are **CÓ** only when a real gateway/CP path exists (not MCP-only, not a stub). Live vendor tokens remain DI-01..07 / DI-20.

Parked (unchanged): DI-01…DI-21 except DI-20 (router9 construct is in 044; production keys still not in git). pgvector (DI-09), sandbox/browser/media spawn (stubs 044), pairing codes (C9), OAuth/Apple/Stripe/K8s/Grafana/Tailscale/Redis.

---

## Matrix — original vs now

| ID | Original (034) | Now (after 035–054) | Evidence |
|----|----------------|---------------------|----------|
| P1 | THIẾU | **CÓ** | `docs/qa/035-agent-pipeline.md`; `gateway/internal/pipeline/runner.go` 8-stage |
| P2 | PARTIAL | **CÓ** | 035 think→act→observe from LLM `ToolCalls`, max 20; `gateway/internal/agent/runtime.go` |
| P3 | PARTIAL | **CÓ** | 035 cap last 50 then sanitize; `gateway/internal/pipeline/history.go` |
| P4 | THIẾU | **CÓ** | 035 in-process hooks; `gateway/internal/pipeline/hooks.go` |
| Q1 | THIẾU | **CÓ** | 035 Full/Task/Minimal/None; `gateway/internal/pipeline/mode.go` |
| Q2 | THIẾU | **PARTIAL** | 039 Anthropic `cache_control` when `CacheMode=full` only; `gateway/internal/llm/anthropic.go` |
| Q3 | THIẾU | **PARTIAL** | 035 `prompt_mode` on the request, not persisted on the session |
| Q4 | THIẾU | **CÓ** | 051 `GOSO_CONTEXT_DIR` SOUL/IDENTITY/AGENTS; `docs/qa/051-bootstrap-context.md` |
| M1 | PARTIAL | **CÓ** | 036 L0 messages + memory stage; `docs/qa/036-memory.md` |
| M2 | THIẾU | **CÓ** | 036 L1 episodic summaries; `gateway/internal/pipeline/memory.go` |
| M3 | THIẾU | **THIẾU** | DI-09 pgvector / L2 KG not built |
| M4 | THIẾU | **THIẾU** | DI-09 progressive L2 load not built |
| M5 | PARTIAL | **CÓ** | 036 `/api/memory`; 052 `/v1/memory` alias |
| V1 | THIẾU | **CÓ** | 037 registry + `[[wikilinks]]`; `docs/qa/037-knowledge-vault.md` |
| V2 | THIẾU | **PARTIAL** | 037 FTS5/substring only; semantic = DI-09 |
| V3 | THIẾU | **CÓ** | 037 `POST /api/vault/sync`; `gateway/internal/vault/vault.go` |
| T1 | THIẾU | **CÓ** | 038 teams lead/members/board; `docs/qa/038-teams-evolution.md` |
| T2 | THIẾU | **CÓ** | 038 sync/async/bidirectional delegate |
| T3 | THIẾU | **CÓ** | 038 modes + 048 PATCH/UI picker; `docs/qa/048-orchestration-mode.md` |
| T4 | THIẾU | **CÓ** | 038 spawn / team_tasks / mailbox |
| T5 | PARTIAL | **CÓ** | 038 MCP `/api/teams`; 052 `/v1/teams` |
| T6 | THIẾU | **CÓ** | 043 Teams page; 046/048 live states + mode column |
| E1 | THIẾU | **PARTIAL** | 038 suggestions + apply-to-instructions; not auto-adapt |
| N1 | THIẾU | **THIẾU** | DI-09 no gateway Postgres `tenant_id` |
| N2 | CÓ | **CÓ** | unchanged SQLite; `gateway/internal/store/sqlite.go` |
| N3 | THIẾU | **PARTIAL** | 051 context files, not per-user workspaces |
| N4 | THIẾU | **PARTIAL** | 041 AES-256-GCM `secrets` table; env keys stay env |
| N5 | THIẾU | **CÓ** | 041 `GOSO_VIEW_TOKEN` GET-only; 077 pairing + explicit POST-deny matrix (backup/kg/skills/evolution). No OAuth (DI-19). |
| N6 | PARTIAL | **PARTIAL** | CRM org isolation; not agent tenancy |
| N7 | PARTIAL | **PARTIAL** | CRM users/roles; unchanged |
| N8 | PARTIAL | **PARTIAL** | one SQLite file; no tenant_id |
| R0 | PARTIAL | **CÓ** | 039 named catalog + 044 `router9`; `gateway/internal/llm/registry.go` |
| R1 | PARTIAL | **PARTIAL** | 039 SSE when `Stream=true` + cache_control; default chat still non-stream |
| R2 | PARTIAL | **CÓ** | 039 OpenAI ChatTools + BaseURL; `gateway/internal/llm/openai.go` |
| R3 | THIẾU | **CÓ** | 039 openrouter adapter (key = DI-20) |
| R4 | THIẾU | **CÓ** | 039 groq adapter |
| R5 | THIẾU | **CÓ** | 039 deepseek adapter |
| R6 | THIẾU | **CÓ** | 039 gemini adapter |
| R7 | THIẾU | **CÓ** | 039 mistral adapter |
| R8 | THIẾU | **CÓ** | 039 xai adapter |
| R9 | THIẾU | **CÓ** | 039 minimax adapter |
| R10 | THIẾU | **CÓ** | 039 dashscope adapter |
| R11 | THIẾU | **PARTIAL** | 039 Claude CLI stub fail-closed |
| R12 | THIẾU | **PARTIAL** | 039 Codex stub fail-closed |
| R13 | THIẾU | **PARTIAL** | 039 ACP stub fail-closed |
| R14 | THIẾU | **CÓ** | 039 named OpenAI-compat endpoints |
| R15 | CÓ | **CÓ** | echo always present |
| R16 | PARTIAL | **CÓ** | 052 `GET /v1/providers`; `docs/qa/052-v1-aliases.md` |
| R17 | PARTIAL | **PARTIAL** | CRM AI draft; unchanged |
| C0 | PARTIAL | **CÓ** | 040 seven names on `GET /api/channels`; `gateway/internal/channel/catalog.go` |
| C1 | PARTIAL | **PARTIAL** | webhook-first Telegram; not long-poll/STT/forums |
| C2 | THIẾU | **PARTIAL** | 040 Discord adapter + fixtures; live token = DI-02 |
| C3 | THIẾU | **PARTIAL** | 040 Slack adapter; live = DI-03 |
| C4 | PARTIAL | **PARTIAL** | Zalo OA stub; live = DI-05 |
| C5 | PARTIAL | **PARTIAL** | Zalo Personal stub; live = DI-06 |
| C6 | THIẾU | **PARTIAL** | 040 Feishu adapter; live = DI-04 |
| C7 | THIẾU | **PARTIAL** | 040 WhatsApp Cloud-API shape; native vs Business = DI-01 |
| C8 | PARTIAL | **CÓ** | 040 WS JSON `ping`/`chat`; `gateway/internal/httpapi/ws.go` |
| C9 | THIẾU | **CÓ** | 040 origin allowlist; 077 one-time pairing codes → view grant. No QR vendor / browser-device pairing. |
| C10 | PARTIAL | **CÓ** | 052 `GET /v1/channels` |
| S1 | PARTIAL | **CÓ** | 041 constant-time Bearer, 1MiB, WS 512KiB; `docs/qa/041-security.md` |
| S2 | THIẾU | **PARTIAL** | 041 four injection patterns (not six) |
| S3 | PARTIAL | **PARTIAL** | 041 path jail + optional SSRF; sandbox = DI-12 |
| S4 | PARTIAL | **CÓ** | 041 token-shape redact + untrusted wrap |
| S5 | THIẾU | **PARTIAL** | 041 `GOSO_WORKSPACE` jail; Docker sandbox = DI-12 |
| S6 | CÓ | **CÓ** | rate-limit unchanged |
| S7 | THIẾU | **PARTIAL** | same scan as S2 |
| S8 | THIẾU | **PARTIAL** | 041 `GOSO_SSRF=1` default off |
| S9 | THIẾU | **CÓ** | 041 AES-256-GCM `secrets` |
| S10 | CÓ | **CÓ** | default-closed admin token |
| S11 | PARTIAL | **PARTIAL** | CRM cookie/org; unchanged |
| S12 | CẮT | **CẮT** | CRM SSO hidden; DI-19 |
| S13 | CẮT | **CẮT** | Stripe; DI-16 |
| O1 | PARTIAL | **CÓ** | 042 nested agent/llm/tool spans; `docs/qa/042-observability.md` |
| O2 | THIẾU | **PARTIAL** | 042 `cache_read_tokens` default 0; Anthropic parse |
| O3 | CẮT | **PARTIAL** | 042 optional `GOSO_OTEL_ENDPOINT` noop-by-default; Jaeger vendor = DI-10 |
| O4 | CÓ | **CÓ** | JSON access log |
| O5 | CÓ | **CÓ** | `/metrics` `/api/stats` |
| O6 | CẮT | **CẮT** | Grafana SaaS; DI-18 |
| B1 | CÓ | **CÓ** | static Go binary |
| B2 | chưa verify | **chưa verify** | size/boot not re-measured |
| B3 | PARTIAL | **PARTIAL** | Control Plane still a separate Vite app |
| K0 | PARTIAL | **CÓ** | 035 tool loop + 044 list/flags; `docs/qa/044-router9-functions.md` |
| K1 | THIẾU | **PARTIAL** | 050 `read_file`/`write_file` jail; not the full six names |
| K2 | THIẾU | **PARTIAL** | 044 `exec`/`browser` stubs `not_configured` |
| K3 | THIẾU | **PARTIAL** | 044 `web_search` fail-closed DDG |
| K4 | THIẾU | **PARTIAL** | 036 memory HTTP; no `knowledge_graph_search` |
| K5 | THIẾU | **PARTIAL** | 044 media stub `not_configured` |
| K6 | PARTIAL | **PARTIAL** | 049 `use_skill` loader; no BM25 / `skill_manage` |
| K7 | PARTIAL | **CÓ** | 038 spawn/delegate/team_tasks |
| K8 | PARTIAL | **PARTIAL** | 054 `/api/cron` ticker; no heartbeat / `sessions_*` agent tools; `docs/qa/054-cron.md` |
| K9 | CÓ | **CÓ** | connector MCP transport |
| K10 | PARTIAL | **PARTIAL** | 052 `/v1` aliases; MCP 66 tools not rewritten |
| D1 | CÓ | **CÓ** | Wails v2 |
| D2 | CÓ | **CÓ** | local SQLite |
| D3 | PARTIAL | **PARTIAL** | 053 CP SSE chat; no media/attachments; `docs/qa/053-chat-sse.md` |
| D4 | THIẾU | **CÓ** | 038 `GOSO_LITE=1` max 5 agents / 1 team |
| D5 | THIẾU | **CÓ** | 036 FTS5 memory search |
| D6 | CẮT | **CẮT** | auto-update; DI-15 |
| D7 | PARTIAL | **CÓ** | 055 CP Channels one-line lite off; adapters kept |
| D8 | CẮT | **CẮT** | notarize; DI-15 |
| W1 | THIẾU | **CÓ** | 040 Bearer `wh_` + HMAC `X-Goso-Signature`; 047 GET list |
| W2 | THIẾU | **CÓ** | 040 sync 200 / async 202 |
| W3 | CẮT | **CẮT** | CRM settings outbound webhooks |
| F1 | CẮT | **CẮT** | WITH_BROWSER; DI-13 |
| F2 | CẮT | **CẮT** | WITH_OTEL overlay; optional env is O3 PARTIAL |
| F3 | CẮT | **CẮT** | WITH_SANDBOX; DI-12 |
| F4 | CẮT | **CẮT** | WITH_TAILSCALE; DI-11 |
| F5 | CẮT | **CẮT** | WITH_REDIS; DI-14 |
| F6 | CÓ | **CÓ** | compose core + prod |
| F7 | CẮT | **CẮT** | K8s/Helm; DI-17 |
| X1 | CÓ | **CÓ** | quota 429 |
| X2 | CÓ | **CÓ** | connectors |
| X3 | CÓ (CRM) | **CÓ** (CRM) | not gateway channel parity |
| X4 | THIẾU | **THIẾU** | no DomainEventBus |

---

## Decision items (still parked)

Same IDs as 034 §C. 044 constructed named provider `router9` when `GOSO_ROUTER9_BASE_URL` is set; that does **not** close DI-20 (production keys) or DI-01..07 (live channel tokens).

| ID | Topic | Status |
|----|--------|--------|
| DI-01 | WhatsApp native vs Business | parked (Cloud-API shape only) |
| DI-02 | Discord bot token | parked |
| DI-03 | Slack app | parked |
| DI-04 | Feishu app | parked |
| DI-05 | Zalo OA live | parked |
| DI-06 | Zalo Personal live | parked |
| DI-07 | Telegram production bot | parked |
| DI-08 | search API vendor | parked (DDG Instant Answer only) |
| DI-09 | pgvector host | parked |
| DI-10 | OTEL collector / Jaeger | parked (env optional) |
| DI-11 | Tailscale | parked |
| DI-12 | sandbox image | parked |
| DI-13 | headless Chrome | parked |
| DI-14 | Redis overlay | parked |
| DI-15 | Apple notarize / auto-update | parked |
| DI-16 | Stripe charging | parked |
| DI-17 | K8s / Helm | parked |
| DI-18 | Grafana SaaS | parked |
| DI-19 | OAuth IdP | parked |
| DI-20 | production LLM keys | parked (adapters exist) |
| DI-21 | media vendors | parked |

---

## QC

- Original 034 snapshot file left in place.
- No GoClaw source pasted; no banned author ids.
- Demos / ports `:8082` `:8091` `:3000` `:18080` `:18088` not bound or killed.
