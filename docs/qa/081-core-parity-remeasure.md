# SPEC 081 — Core GoClaw vs goso parity remeasure

Date: 2026-08-29. Docs-only. Clean-room. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed.

goso HEAD this pass: **`f8eed70`** (`Merge SPEC 080 heartbeat stamp and sessions agent tools`).
GoClaw cite tree (READ-ONLY, CC-BY-NC): `/Users/mqglobal/Documents/goclaw/goclaw-source` — README + `_readmes` + `docs/*` only. No `.go` paste.

This file **does not inherit** status from `docs/qa/034-goclaw-parity-update-054.md`. Every CORE row was re-opened on `f8eed70` (QA 035–080 + the implementation path named in Evidence). The 054 column is a historical published snapshot only.

Status meanings (unchanged from 034):

| Status | Meaning |
|--------|---------|
| **CÓ** | Working implementation of that GoClaw behavior (may be simpler, not a stub). |
| **PARTIAL** | Interface, stub, one-of-many, fail-closed placeholder, or CRM-adjacent not gateway. |
| **THIẾU** | Not built in goso gateway. |
| **CẮT** | Explicitly scoped out (parked DI). |
| **unverified** | Not measured this pass. |

---

## 1. CORE definition (this pass)

Opened: `goclaw-source/README.md` **Core Features** (lines 61–75), Lite vs Standard table, 8 tool categories, Webhook API, `WITH_*` flags; plus docs `01-agent-loop.md`, `02-providers.md`, `03-tools-system.md`, `05-channels-messaging.md`, `07-bootstrap-skills-memory.md`, `08-scheduling-cron.md`, `09-security.md`, `18-http-api.md`, `22-heartbeat-system.md`, `23-multi-tenant-architecture.md`.

**In sample:** pipeline/prompt, memory L0–L2+KG, vault, teams/delegation/self-evolution, providers, channels, tools (fs/web/skills/cron/heartbeat/media), security/tenancy/RBAC, webhooks/WS, observability, plus every **existing 034 matrix row** (including Lite desktop, single-binary, compose, adjacent X*).

**Out of weighted %:** rows whose 081 status is **CẮT** (K8s / Stripe / notarize / Grafana SaaS / Tailscale / Redis / WITH_BROWSER / WITH_SANDBOX / CRM SSO / CRM outbound webhooks). They remain in the inventory because they were already 034 rows (user rule: exclude those DI from the sample *unless* they already have a matrix row).

**Not added as new rows:** SPEC 070 backup (ops, not a 034 ID). Cited under ops notes only.

Row IDs = 034 complete set: P1–P4, Q1–Q4, M1–M5, V1–V3, T1–T6, E1, N1–N8, R0–R17, C0–C10, S1–S13, O1–O6, B1–B3, K0–K10, D1–D8, W1–W3, F1–F7, X1–X4. **N = 115**.

---

## 2. Headline counts (counted from §4, not estimated)

| Stat | N | How |
|------|--:|-----|
| **N_CÓ** | **74** | `COUNT(status=CÓ)` |
| **N_PARTIAL** | **27** | `COUNT(status=PARTIAL)` |
| **N_THIẾU** | **2** | N1, X4 |
| **N_CẮT** | **12** | S12, S13, O6, D6, D8, W3, F1, F2, F3, F4, F5, F7 |
| **N_unverified** | **0** | B2 measured this pass (was unverified in 034/054) |
| Inventory | **115** | 74+27+2+12+0 |
| Weighted denominator | **103** | CÓ+PARTIAL+THIẾU+unverified (CẮT out) |

**Weighted %** = `(N_CÓ + 0.5×N_PARTIAL) / 103`

```
(74 + 0.5×27) / 103 = 87.5 / 103 = 175/206 = 0.8495145631067961
→ 84.95%
```

**Unweighted CÓ-only %** = `N_CÓ / 103`

```
74 / 103 = 0.7184466019417476
→ 71.84%
```

054 published mechanical index (not this pass): 56 CÓ, 41 PARTIAL, 4 THIẾU, 12 CẮT, 1 unverified → 75.00% weighted on 102. CTO (`docs/qa/audit-cto-2026-08-28.md`) scored parity **70%** at HEAD `533539e` because W1/W2, streaming, and provider routing were over-claimed. Those three were re-opened here after 066–080.

---

## 3. Breakdown by CORE axis

Weighted axis % uses the same formula; CẮT inside the axis is excluded from that axis denominator.

| Axis | Rows | CÓ | PARTIAL | THIẾU | CẮT | unverified | den | Weighted | Unweighted |
|------|------|---:|--------:|------:|----:|-----------:|----:|---------:|-----------:|
| pipeline / prompt | P1–P4, Q1–Q4 | 8 | 0 | 0 | 0 | 0 | 8 | **100.00%** (8/8) | 100.00% |
| memory L0–L2+KG | M1–M5 | 5 | 0 | 0 | 0 | 0 | 5 | **100.00%** (5/5) | 100.00% |
| vault | V1–V3 | 2 | 1 | 0 | 0 | 0 | 3 | **83.33%** (2.5/3) | 66.67% |
| teams / delegation / self-evolution | T1–T6, E1 | 7 | 0 | 0 | 0 | 0 | 7 | **100.00%** (7/7) | 100.00% |
| providers | R0–R17 | 14 | 4 | 0 | 0 | 0 | 18 | **88.89%** (16/18) | 77.78% |
| channels (+ WS C8) | C0–C10 | 4 | 7 | 0 | 0 | 0 | 11 | **68.18%** (7.5/11) | 36.36% |
| webhooks | W1–W3 | 2 | 0 | 0 | 1 | 0 | 2 | **100.00%** (2/2) | 100.00% |
| tools | K0–K10 | 6 | 5 | 0 | 0 | 0 | 11 | **77.27%** (8.5/11) | 54.55% |
| security / tenancy / RBAC | S1–S13, N1–N8 | 12 | 6 | 1 | 2 | 0 | 19 | **78.95%** (15/19) | 63.16% |
| observability | O1–O6 | 3 | 2 | 0 | 1 | 0 | 5 | **80.00%** (4/5) | 60.00% |
| ops / binary / compose | B1–B3, F1–F7 | 3 | 1 | 0 | 6 | 0 | 4 | **87.50%** (3.5/4) | 75.00% |
| desktop Lite | D1–D8 | 5 | 1 | 0 | 2 | 0 | 6 | **91.67%** (5.5/6) | 83.33% |
| adjacent (not README Core) | X1–X4 | 3 | 0 | 1 | 0 | 0 | 4 | **75.00%** (3/4) | 75.00% |
| **all CORE sample** | **115** | **74** | **27** | **2** | **12** | **0** | **103** | **84.95%** | **71.84%** |

Axis dens sum to 103 (every non-CẮT row appears in exactly one axis).

---

## 4. Full matrix (081 now = this pass)

Evidence SHA: goso **`f8eed70`**. Paths were opened this pass.

### 4.1 Pipeline / prompt

| # | GoClaw behavior | 081 | Evidence (opened on f8eed70) |
|---|-----------------|-----|------------------------------|
| P1 | 8 stages context → history → prompt → think → act → observe → memory → summarize | **CÓ** | `gateway/internal/pipeline/runner.go` `Run`: comment + flow `context → history → prompt → loop(think, act, observe) → memory → summarize`. QA `docs/qa/035-agent-pipeline.md`. |
| P2 | Think→Act→Observe, LLM tool calls, max ~20 | **CÓ** | `runner.go` `MaxIterations=20`, `actObserve` from `llm.ToolCalls`. |
| P3 | History cap / sanitize | **CÓ** | `gateway/internal/pipeline/history.go` (HistoryCap 50). |
| P4 | Lifecycle hooks | **CÓ** | `hooks.go`: SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, Stop. No SubagentStart/Stop (simpler). |
| Q1 | Full / Task / Minimal / None + section gating | **CÓ** | `gateway/internal/pipeline/mode.go` `SystemPrompt` switch. |
| Q2 | Anthropic cache_control prefix | **CÓ** | `gateway/internal/llm/compat_test.go` asserts `cache_control` on system+bootstrap+last non-user when CacheMode/GOSO_PROMPT_CACHE=full. QA `docs/qa/076-prompt-cache-mode.md`. Opt-in env, not always-on. |
| Q3 | Per-session prompt_mode persist | **CÓ** | `mode.go` `ResolvePromptMode`; sessions column (076 `af0f468`). |
| Q4 | Bootstrap SOUL/IDENTITY/AGENTS inject | **CÓ** | `gateway/internal/pipeline/context.go`; QA `docs/qa/051-bootstrap-context.md`. |

### 4.2 Memory L0–L2 + KG

| # | GoClaw behavior | 081 | Evidence |
|---|-----------------|-----|----------|
| M1 | L0 working = conversation | **CÓ** | `messages` table `gateway/internal/store/sqlite.go`; `pipeline/memory.go` MemoryStage. QA 036. |
| M2 | L1 episodic summaries | **CÓ** | `SummarizeStage` `pipeline/memory.go`. |
| M3 | L2 knowledge graph | **CÓ** | SQLite `kg_entities`/`kg_relations` + FTS5 `kg_fts` (`sqlite.go`, `sqlite_kg.go` `3a3f0f2`). Embeddings/pgvector remain DI-09 (not this row’s Lite FTS graph). QA `docs/qa/072-memory-l2-fts.md`. |
| M4 | Progressive L0→L1→L2 load | **CÓ** | `store.SearchProgressive` `gateway/internal/store/kg.go`; tests `kg_test.go`. |
| M5 | Memory HTTP / MCP target | **CÓ** | `/api/memory` + `/v1/memory` (052). |

### 4.3 Vault

| # | GoClaw behavior | 081 | Evidence |
|---|-----------------|-----|----------|
| V1 | Document registry + `[[wikilinks]]` | **CÓ** | `gateway/internal/vault/vault.go` `ParseWikilinks`. QA 037. |
| V2 | Hybrid FTS + pgvector | **PARTIAL** | `vault_fts` FTS5 in `sqlite.go`. No pgvector host (DI-09). `docs/qa/071-pgvector-path.md`. |
| V3 | Filesystem sync | **CÓ** | `POST /api/vault/sync` in vault.go. |

### 4.4 Teams / self-evolution

| # | GoClaw behavior | 081 | Evidence |
|---|-----------------|-----|----------|
| T1 | Teams lead/members/board | **CÓ** | `teams`/`team_members`/`team_tasks` in `sqlite.go`. QA 038. |
| T2 | Delegate sync/async/bidirectional | **CÓ** | `gateway/internal/team/`. |
| T3 | auto / explicit / manual | **CÓ** | team modes + QA 048. |
| T4 | spawn / team_tasks / mailbox | **CÓ** | team tools in `team.go` ToolSpecs. |
| T5 | `/v1/teams` | **CÓ** | 052 aliases. |
| T6 | CP Teams UX | **CÓ** | `control-plane/src/pages` Teams (043/046/048). |
| E1 | Metrics → suggestions → auto-adapt, never identity | **CÓ** | `evolution_guardrails` in sqlite.go; tick apply/rollback; `GOSO_EVOLUTION_AUTO` default off. QA `docs/qa/073-self-evolution.md`. |

### 4.5 Tenancy / AES / RBAC

| # | GoClaw behavior | 081 | Evidence |
|---|-----------------|-----|----------|
| N1 | Gateway PostgreSQL + tenant_id | **THIẾU** | `GOSO_DATABASE_URL` postgres DSN fail-closed (`docs/qa/071-tenant-lite.md`). StoreIface stays SQLite. DI-09. |
| N2 | Desktop/local SQLite | **CÓ** | `sqlite.go`; `desktop/internal/host/host.go`. |
| N3 | Per-user workspace + context files | **PARTIAL** | Context files 051; `GOSO_WORKSPACE` jail 041. Not per-user workspace dirs. |
| N4 | AES-256-GCM API keys at rest | **CÓ** | `secrets` table + `gateway/internal/secrets/`. Env keys stay env. QA 041. |
| N5 | Gateway RBAC (admin/view) | **CÓ** | Admin Bearer + `GOSO_VIEW_TOKEN` GET-only + pairing `gateway/internal/auth/pairing.go` `dc36f84`. No OAuth (DI-19). QA 077. |
| N6 | CRM org tenant header | **PARTIAL** | Sibling `goso-crm/db/tenant.go` opened; CRM org, not agent tenancy. |
| N7 | CRM users/roles | **PARTIAL** | `goso-crm/internal/orgset/store.go` opened. |
| N8 | Isolated sessions per tenant | **CÓ** | `sessions.tenant_id`; 071 tests two-tenant chat 404. Default Mode 1 = `default` when `GOSO_MULTI_TENANT` unset. |

### 4.6 Providers

| # | GoClaw behavior | 081 | Evidence |
|---|-----------------|-----|----------|
| R0 | Registry + routing | **CÓ** | `llm/registry.go`; `llm/resolve.go`; per-agent `llm_provider` (066). QA `docs/qa/066-routing-security-compose.md`. |
| R1 | Anthropic HTTP + SSE + cache | **CÓ** | `llm/sse.go` `d1b3175` content_block_delta; cache_control 076. QA 068 + 076. |
| R2 | OpenAI Chat Completions | **CÓ** | `llm/openai.go` ChatTools + stream. |
| R3 | OpenRouter | **CÓ** | `OpenAICompatProviders()` in `llm/compat.go`. |
| R4 | Groq | **CÓ** | same catalog. |
| R5 | DeepSeek | **CÓ** | same. |
| R6 | Gemini | **CÓ** | same. |
| R7 | Mistral | **CÓ** | same. |
| R8 | xAI | **CÓ** | same. |
| R9 | MiniMax | **CÓ** | same. |
| R10 | DashScope | **CÓ** | same. |
| R11 | Claude CLI stdio | **PARTIAL** | `llm/stubs.go` `ClaudeCLI` `not_configured`. |
| R12 | Codex OAuth | **PARTIAL** | `stubs.go` `Codex`. |
| R13 | ACP JSON-RPC | **PARTIAL** | `stubs.go` `ACP`. |
| R14 | Named OpenAI-compat + router9 | **CÓ** | compat.go `router9` when `GOSO_ROUTER9_BASE_URL` set. |
| R15 | Echo fallback | **CÓ** | registry always `put("echo", Echo{})`. |
| R16 | `/v1/providers` | **CÓ** | 052. |
| R17 | CRM AI draft | **PARTIAL** | goso-crm `internal/ai` (unchanged sibling). |

### 4.7 Channels + WS

| # | GoClaw behavior | 081 | Evidence |
|---|-----------------|-----|----------|
| C0 | 7-name catalog | **CÓ** | `gateway/internal/channel/catalog.go` `Names` + `env_names` (`91a51be`). QA 078. Live GET this run: 7 names, no token values. |
| C1 | Telegram | **PARTIAL** | `channel/telegram.go` webhook-first, not long-poll/STT/forums. DI-07. |
| C2 | Discord | **PARTIAL** | `discord.go` HandleUpdate + tests. Live token DI-02. |
| C3 | Slack | **PARTIAL** | `slack.go`. DI-03. |
| C4 | Zalo OA | **PARTIAL** | `zalo_oa.go`. DI-05. |
| C5 | Zalo Personal | **PARTIAL** | `zalo_personal.go`. DI-06. |
| C6 | Feishu | **PARTIAL** | `feishu.go`. DI-04. |
| C7 | WhatsApp | **PARTIAL** | `whatsapp.go` Cloud-API shape. Native vs Business DI-01. |
| C8 | WS JSON chat (not echo-only) | **CÓ** | `httpapi/ws.go` `ping`/`chat`, `SetReadLimit`. Production empty origins refused (066). |
| C9 | Pairing codes / allowlists | **CÓ** | `auth/pairing.go`; `GOSO_WS_ORIGINS`. QA 077. |
| C10 | `/v1/channels` | **CÓ** | 052 + 078. |

### 4.8 Security

| # | GoClaw behavior | 081 | Evidence |
|---|-----------------|-----|----------|
| S1 | Transport: timing-safe, 1MiB, WS 512KiB, origins | **CÓ** | `security/compare.go`, `limit.go`, `ws.go`. QA 041. |
| S2 | 6 injection patterns | **CÓ** | `security/injection.go` six substrings including `you are now`, `end of system` (`5faa7be`). Not GoClaw regex tables. QA 079. |
| S3 | Tool: path jail, SSRF, exec approval | **PARTIAL** | path.go + approval gate + SSRF. No shell deny list / real exec. |
| S4 | Output redact + untrusted wrap | **CÓ** | `security/wrap.go`; eventstore redact. |
| S5 | Isolation: workspace + Docker sandbox | **PARTIAL** | `GOSO_WORKSPACE` jail. Docker sandbox DI-12. |
| S6 | Rate limit | **CÓ** | `ratelimit/limiter.go`. |
| S7 | Prompt-injection detect | **CÓ** | same ScanInjection as S2. |
| S8 | SSRF DNS-aware | **CÓ** | `security/ssrf.go` `SSRFEnabled`: production default on; `GOSO_SSRF=0` keeps demo loopback. QA 066. |
| S9 | AES-256-GCM | **CÓ** | same as N4. |
| S10 | Default-closed admin token | **CÓ** | `serve.go` + auth. |
| S11 | CRM session cookie | **PARTIAL** | goso-crm auth gate (sibling). |
| S12 | CRM SSO / integrations | **CẮT** | DI-19 / SPEC 033. |
| S13 | Stripe live charging | **CẮT** | DI-16. |

### 4.9 Observability / binary / compose

| # | GoClaw behavior | 081 | Evidence |
|---|-----------------|-----|----------|
| O1 | Nested LLM/tool spans | **CÓ** | `observe/` span trees. QA 042. In-memory ring 200, not a collector. |
| O2 | Prompt-cache metrics | **PARTIAL** | `llm/usage.go` `CacheReadTokens`; Anthropic SSE parse in `sse.go`. Default 0 when unused; no CP chart. |
| O3 | Optional OTLP | **PARTIAL** | `observe/export.go` Noop unless `GOSO_OTEL_ENDPOINT`. Jaeger vendor DI-10. |
| O4 | JSON access log + request id | **CÓ** | `observe` X-Request-ID. |
| O5 | `/api/stats` `/metrics` | **CÓ** | `observe/stats.go` + `last_heartbeat` omitempty (080 `1357dee`). |
| O6 | Grafana SaaS | **CẮT** | DI-18. |
| B1 | Static Go gateway, CGO off | **CÓ** | Dockerfile / `CGO_ENABLED=0 go build ./gateway/cmd/goso-gateway`. |
| B2 | ~25 MB and &lt;1s boot | **CÓ** | This pass: `CGO_ENABLED=0 go build` → **17908114 bytes (17.08 MiB)**; process start to `GET /healthz` **0.847 s** on **:18181** (not 8082/8091/18080). |
| B3 | CP separate Node/Vite | **PARTIAL** | `control-plane/` Vite :3000. Gateway has no embedded SPA. |
| F1 | WITH_BROWSER Chrome | **CẮT** | DI-13. |
| F2 | WITH_OTEL Jaeger overlay | **CẮT** | DI-10; optional env is O3. |
| F3 | WITH_SANDBOX | **CẮT** | DI-12. |
| F4 | WITH_TAILSCALE | **CẮT** | DI-11. |
| F5 | WITH_REDIS | **CẮT** | DI-14. |
| F6 | Core compose + prod overlay | **CÓ** | `compose.yml` / `compose.prod.yml` + 066 env pass-through. |
| F7 | K8s / Helm | **CẮT** | DI-17. |

### 4.10 Tools

| # | GoClaw behavior | 081 | Evidence |
|---|-----------------|-----|----------|
| K0 | Tool registry + LLM tool loop | **CÓ** | `builtin.go` + `runtime.go` Call from ToolCalls. |
| K1 | Filesystem six names | **PARTIAL** | `read_file` `write_file` `list_files` `edit` `send_file` in `builtin.go`/`fs.go`. **No `search` / `glob`.** QA 074. |
| K2 | Runtime exec + browser | **PARTIAL** | stubs `not_configured` (DI-12/13). |
| K3 | web_search / web_fetch | **PARTIAL** | `web_search` DDG Instant Answer fail-closed + httptest. **No `web_fetch`.** QA 074. |
| K4 | Memory tools | **CÓ** | `memory_search` / `memory_expand` `pipeline/kg.go`. |
| K5 | Media generate/read/tts | **PARTIAL** | `media`/`image_gen`/`tts` `not_configured` unless test double. DI-21. |
| K6 | Skills BM25 + manage | **CÓ** | `skill_search` BM25 + POST/DELETE `/api/skills`. QA 075. |
| K7 | Team tools | **CÓ** | spawn/delegate/team_tasks. |
| K8 | cron / heartbeat / sessions_* | **CÓ** | cron 054; `gateway/internal/heartbeat/heartbeat.go` `1357dee`; `pipeline/sessions.go` `sessions_list`/`sessions_history` cap 50. QA 080. Live: `POST /api/system/heartbeat` 200 then `/api/stats` includes `last_heartbeat`. |
| K9 | MCP connector transport | **CÓ** | `connector/mcp.go`. |
| K10 | goso-mcp 66 tools | **PARTIAL** | `mcp/` still dual `/v1` client; not rewritten to 66 GoClaw tools. |

### 4.11 Desktop Lite

| # | GoClaw behavior | 081 | Evidence |
|---|-----------------|-----|----------|
| D1 | Wails v2 + React | **CÓ** | `desktop/`. |
| D2 | Local SQLite | **CÓ** | host.go. |
| D3 | Chat streaming, tools, media, attachments | **PARTIAL** | Real SSE 068; tools yes; **no media/attachments**. |
| D4 | Max 5 agents / 1 team | **CÓ** | `LiteMaxAgents=5` when `GOSO_LITE=1` `store.go`. |
| D5 | FTS5 memory search | **CÓ** | `memory_fts` virtual table `sqlite.go`. |
| D6 | Auto-update GitHub Releases | **CẮT** | DI-15. |
| D7 | Lite channels flag | **CÓ** | 055 CP lite-off line; adapters remain. |
| D8 | Apple notarize | **CẮT** | DI-15. |

### 4.12 Webhooks / adjacent

| # | GoClaw behavior | 081 | Evidence |
|---|-----------------|-----|----------|
| W1 | Registry + Bearer `wh_` + HMAC + persist | **CÓ** | sqlite `webhooks` + `webhook_jobs`; HMAC freshness 300s / replay 320s. QA 067. |
| W2 | Sync 200 / async 202 + jobs | **CÓ** | same 067. |
| W3 | CRM Settings outbound HTTP | **CẮT** | SPEC 033 / 072+ parked. |
| X1 | Billing quota 429 | **CÓ** | `billing/quota.go`. |
| X2 | Connector HTTP + fake | **CÓ** | `connector/`. |
| X3 | CRM Zalo QR / inbound | **CÓ** (CRM, not gateway C5) | sibling zaloqr (not re-scored as C5). |
| X4 | DomainEventBus + consolidation | **THIẾU** | no eventbus package in gateway. Not user-visible. |

---

## 5. Remaining THIẾU / PARTIAL + recommendation

### THIẾU (2)

| ID | Gap | Rec |
|----|-----|-----|
| **N1** | No gateway PostgreSQL. SQLite `tenant_id` exists (071). | **DI-09** host. Do not open a SPEC until Dat picks a PG16+pgvector host. Path already in `docs/qa/071-pgvector-path.md`. |
| **X4** | DomainEventBus | Only if Dat wants it. Not README Core, not user-visible. Stay parked. |

### PARTIAL that are DI / live vendor (do not fill secrets)

| ID | Gap | Rec |
|----|-----|-----|
| C1–C7 | Webhook adapters exist; live bots/protocol depth missing | DI-01..07. Catalog/UI already 078. |
| R11–R13 | claude-cli / Codex / ACP `not_configured` | Optional follow-up; fail-closed is honest. |
| K2 | exec/browser stubs | DI-12 / DI-13. |
| K5 | media stubs | DI-21. |
| V2 | no pgvector hybrid | DI-09 (same as N1). |
| O3 | OTLP env only | DI-10 collector. |
| N6 N7 S11 R17 | CRM org/AI, not gateway | Stay sibling; not a goso SPEC. |

### PARTIAL that are clean-room product (no new vendor)

| ID | Gap | Rec |
|----|-----|-----|
| K1 | no `search` / `glob` filesystem tools | Small SPEC if Dat wants six-name completeness. |
| K3 | no `web_fetch` | Small SPEC; SSRF already 066. |
| K10 | MCP 66-tool rewrite | Low user-visible; `/v1` aliases exist (052). |
| N3 | not per-user workspace dirs | Fold into DI-09 Standard tenancy, or skip (Lite-shaped). |
| S3 | no shell deny / real exec | Tied to K2/DI-12. |
| S5 | no Docker sandbox | DI-12. |
| O2 | cache tokens not a CP metric | Optional chrome; parse exists. |
| B3 | SPA not embedded in gateway | Product choice; Vite CP works. |
| D3 | no chat media/attachments | After K5/DI-21. |

**No 082+ queue is implied.** 072–080 closed the non-DI THIẾU/PARTIAL that were user-visible. Next work is DI-gated or explicitly picked from the small clean-room list (K1 glob/search, K3 web_fetch).

Parked DI table unchanged from `docs/qa/034-goclaw-parity-update-054.md` § Decision items (DI-01..21). 080 did not unpark any DI.

---

## 6. Method / QC

Re-opened this pass:

- `docs/qa/034-goclaw-parity-matrix.md` (row IDs).
- `docs/qa/034-goclaw-parity-update-054.md` (historical 75% only).
- `docs/qa/audit-cto-2026-08-28.md` (CTO 70% at pre-066 HEAD).
- QA 035–080 files listed under `docs/qa/`.
- Implementation paths in §4 (gateway pipeline, store sqlite/kg, llm registry/sse/compat/stubs, channel catalog+adapters, security injection/ssrf, auth pairing, observe export/stats, builtin tools, heartbeat, sessions tools, vault wikilinks, webhook tables).
- Sibling CRM `db/tenant.go`, `internal/orgset/store.go` (N6/N7).
- GoClaw README Core Features + named docs (behavior cite only).

B2: built and booted on **127.0.0.1:18181** then killed. PIDs `:8082` 85417, `:8091` 83346, `:18080` 47458 untouched.

Arithmetic: 74+27+2+12+0=115; den=103; weighted 87.5/103=175/206; unweighted 74/103. Axis dens sum to 103.

No goso product code in this SPEC. No goclaw Go. No secrets.

## Non-goals

Copying goclaw source. Changing gateway behavior. Unparking DI. Binding/killing demo ports. Merge of product branches.
