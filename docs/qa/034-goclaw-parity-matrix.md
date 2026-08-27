# SPEC 034 — GoClaw vs goso parity matrix

Date: 2026-08-27. Worker 1 (docs only). North-star = GoClaw README Core Features + Desktop Lite deltas + 8 tool categories + Webhook API + `WITH_*` flags. Behavior rewritten clean-room; this file does **not** copy GoClaw source.

Compared trees:

| Repo | Ref | Commit |
|------|-----|--------|
| goso (this worktree) | `origin/main` | `ef42f77fa81bc20a703f0e70ca6d4cd45c584bf8` |
| goso-crm (sibling, read-only) | `origin/main` | `108e557ea750fe94e190d5826e2820fbcafd214e` |

Method: read GoClaw README + listed docs (architecture, loop, providers, channels, bootstrap/memory, security, tracing, teams, multi-tenant, vault). Opened every goso / goso-crm path cited on a **CÓ** or **PARTIAL** row. No guess: if the file was not opened, status is **THIẾU** or **chưa verify**.

Status:

| Status | Meaning |
|--------|---------|
| **CÓ** | Working implementation of that GoClaw behavior (may be simpler, but not a stub). |
| **PARTIAL** | Interface, stub, echo, one-of-many, or MCP client pointing at a missing `/v1` backend. |
| **THIẾU** | Never built in goso or goso-crm. |
| **CẮT** | Explicitly scoped out as a ship tactic (SPEC 008/009/010/012/024/027/029/032/033). |

CẮT is not a product drop: SPEC 034 re-anchors those rows for later SPECs unless the coordinator parks them as decision items (K8s, Grafana SaaS, Apple notarize, Stripe charging).

---

## 1. Feature inventory (SPEC 034 §A)

Grouped from the GoClaw README. Sub-rows are names the README (or the README Lite table / tools table / `WITH_*` table) actually lists.

### 1.1 8-stage agent pipeline

- Pluggable stages, always-on: context → history → prompt → think → act → observe → memory → summarize
- Think → Act → Observe loop (max iterations, tool calls, then final text)
- Setup/finalize around the iteration loop
- Agent lifecycle hooks on pipeline stages (SessionStart, UserPromptSubmit, Pre/PostToolUse, Stop, SubagentStart/Stop)

### 1.2 4-mode prompt

- Full / Task / Minimal / None
- Section gating
- Cache-boundary optimization
- Per-session mode resolution

### 1.3 3-tier memory (L0 / L1 / L2)

- L0 working (conversation)
- L1 episodic (session summaries)
- L2 semantic (knowledge graph)
- Progressive loading
- Memory flush before compaction / auto-summarize

### 1.4 Knowledge Vault

- Document registry
- `[[wikilinks]]` bidirectional
- Hybrid search (FTS / BM25 + pgvector)
- Filesystem sync (content on disk, registry hash/embedding)

### 1.5 Agent Teams & orchestration

- Shared task boards (Kanban lifecycle)
- Inter-agent delegation: sync / async / bidirectional
- 3 orchestration modes: auto / explicit / manual
- Lead + members, TEAM.md injection, mailbox
- Spawn / delegate / team_tasks tool gating

### 1.6 Self-evolution

- Metrics → suggestions → auto-adapt
- Guardrails (never change identity, name, or core purpose)
- May refine communication style / CAPABILITIES.md

### 1.7 Multi-tenant PostgreSQL

- Per-user workspaces
- Per-user context files
- Encrypted API keys (AES-256-GCM)
- RBAC
- Isolated sessions
- Dual-DB: PostgreSQL (Standard) / SQLite (Lite)

### 1.8 20+ LLM providers (README names)

- Anthropic — native HTTP + SSE + prompt caching
- OpenAI
- OpenRouter
- Groq
- DeepSeek
- Gemini
- Mistral
- xAI
- MiniMax
- DashScope
- Claude CLI
- Codex
- ACP
- Any OpenAI-compatible endpoint

### 1.9 7 messaging channels (README names)

- Telegram
- Discord
- Slack
- Zalo OA
- Zalo Personal
- Feishu / Lark
- WhatsApp

### 1.10 Production security

- 5-layer permission: transport, input, tool, output, isolation
- Rate limiting
- Prompt-injection detection (6 patterns)
- SSRF protection
- AES-256-GCM encryption at rest

### 1.11 Observability

- Built-in LLM call tracing with spans
- Prompt-cache metrics
- Optional OpenTelemetry OTLP export

### 1.12 Single binary

- ~25 MB static Go binary
- No Node runtime for the gateway
- <1s startup

### 1.13 Built-in tools — 8 categories (README names)

| Category | Named tools |
|----------|-------------|
| Filesystem | `read_file`, `write_file`, `edit_file`, `list_files`, `search`, `glob` |
| Runtime | `exec`, `browser` |
| Web | `web_search`, `web_fetch` |
| Memory | `memory_search`, `memory_get`, `knowledge_graph_search` |
| Media | `create_image`, `create_audio`, `create_video`, `read_*`, `tts` |
| Skills | `skill_search`, `use_skill`, `skill_manage` |
| Teams | `team_tasks`, `spawn`, `delegate`, `message` |
| Automation | `cron`, `heartbeat`, `sessions_*` |

### 1.14 Desktop Lite vs Standard (README table)

| Delta | Lite | Standard |
|-------|------|----------|
| Agents | Max 5 | Unlimited |
| Teams | Max 1 (5 members) | Unlimited |
| Database | SQLite | PostgreSQL |
| Memory search | FTS5 text | pgvector semantic |
| Channels | none | 7 channels |
| Knowledge graph | none | full |
| RBAC / multi-tenant | none | full |
| Auto-update | GitHub Releases | Docker / binary |
| App | Wails v2 + React, ~30 MB | server binary |

### 1.15 Webhook API

- Bearer and HMAC auth
- `POST /v1/webhooks/llm` (sync / async)
- Channel message webhook
- Admin CRUD of webhook registry

### 1.16 Optional Compose flags (README `WITH_*`)

| Flag | Service |
|------|---------|
| `WITH_BROWSER=1` | Headless Chrome (`browser` tool) |
| `WITH_OTEL=1` | Jaeger / OTLP UI |
| `WITH_SANDBOX=1` | Docker sandbox for untrusted exec |
| `WITH_TAILSCALE=1` | Tailscale private network |
| `WITH_REDIS=1` | Redis-backed cache |

---

## 2. Parity matrix (SPEC 034 §B)

Evidence SHA: goso `ef42f77` = `origin/main`; goso-crm `108e557` = `origin/main`. Paths are files opened this pass.

### 2.1 Pipeline

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| P1 | 8 pluggable stages context → history → prompt → think → act → observe → memory → summarize | **THIẾU** | No `pipeline` / `ContextStage` / `ThinkStage` in goso Go. `gateway/internal/agent/runtime.go` is a single `Chat`: persist user msg → keyword `matchTools` → one `LLM.Chat` → persist assistant. |
| P2 | Think → Act → Observe iteration (tool calls until final text, max ~20) | **PARTIAL** | `runtime.go` `Chat` + `matchTools` (opened): tools fire from substring/intent on the user text, not from LLM tool_use, and there is no second LLM turn after tool results. |
| P3 | History pipeline (limit turns, sanitize tool pairs, prune/compact) | **PARTIAL** | `store.ListMessages` in `gateway/internal/store/store.go` and `sqlite.go` (opened) returns the full session list with no prune/summarize. |
| P4 | Lifecycle hooks on pipeline stages | **THIẾU** | No hooks package or SessionStart/PreToolUse in goso. |

### 2.2 Prompt modes

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| Q1 | Four modes Full / Task / Minimal / None with section gating | **THIẾU** | No PromptFull/Task/Minimal/None in goso. Chat sends raw session messages (`runtime.go`). |
| Q2 | Cache-boundary optimization for prompt caching | **THIẾU** | `gateway/internal/llm/anthropic.go` (opened): Messages API, no `cache_control`, no SSE. |
| Q3 | Per-session mode resolution | **THIẾU** | Session model in `store.go` has id/agent/label only. |
| Q4 | Bootstrap context files (AGENTS/SOUL/IDENTITY/…) injected into system prompt | **THIẾU** | No bootstrap/templates in goso. |

### 2.3 Memory L0 / L1 / L2

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| M1 | L0 working memory = conversation | **PARTIAL** | Session `messages` table in `gateway/internal/store/sqlite.go` (opened). Conversation only; no MEMORY.md / flush. |
| M2 | L1 episodic session summaries | **THIẾU** | No episodic/summarize worker. |
| M3 | L2 semantic knowledge graph | **THIẾU** | No KG / embeddings in goso. `docs/DEPLOY.md` (opened): Postgres/pgvector “Chưa”. |
| M4 | Progressive L0→L1→L2 load | **THIẾU** | — |
| M5 | MCP memory CRUD client | **PARTIAL** | `mcp/src/tools/register-memory-tools.ts` + `mcp/src/client/endpoints/memory-endpoints.ts` (opened) call `/v1/memory`. Gateway has no `/v1/memory` (grep `gateway/**/*.go`: none). |

### 2.4 Knowledge Vault

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| V1 | Document registry + `[[wikilinks]]` | **THIẾU** | No vault package; no wikilink in goso/goso-crm. |
| V2 | Hybrid FTS + pgvector search | **THIẾU** | SQLite schema in `sqlite.go` has no FTS5/virtual table (opened). Decision 04 in `.planning/decisions.md` (opened) planned dual-DB; not implemented. |
| V3 | Filesystem sync of vault docs | **THIẾU** | — |

### 2.5 Teams / delegation / orchestration

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| T1 | Agent teams (lead/members, shared board) | **THIẾU** | Gateway store has agents/sessions/connectors only (`store.go`). Control Plane `AgentsPage.tsx` (opened) is agent CRUD, no team. |
| T2 | Delegation sync / async / bidirectional | **THIẾU** | No `delegate` tool in gateway. MCP `register-agent-tools.ts` (opened) has list/create delegation copy aimed at `/v1`. |
| T3 | Orchestration modes auto / explicit / manual | **THIẾU** | — |
| T4 | Spawn / team_tasks / mailbox | **THIẾU** | Gateway `Chat` does not spawn subagents. |
| T5 | MCP team CRUD client | **PARTIAL** | `mcp/src/tools/register-team-tools.ts` + `team-endpoints.ts` (opened) call `/v1/teams`. Gateway has no `/v1/teams`. |
| T6 | Control Plane “teams” UX | **THIẾU** | `ComingSoonPage.tsx` (opened) is a generic empty state; no team board. CRM Settings “team” group is org users/roles (`SettingsPage.tsx`), not agent teams. |

### 2.6 Self-evolution

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| E1 | Metrics → suggestions → auto-adapt, never identity | **THIẾU** | No evolution/SOUL/CAPABILITIES in goso or goso-crm. |

### 2.7 Multi-tenant Postgres + AES-256-GCM + RBAC

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| N1 | Gateway PostgreSQL + `tenant_id` isolation | **THIẾU** | Gateway persist is SQLite (`sqlite.go`, `compose.yml` `GOSO_DB_PATH=/data/goso.db`). No `tenant_id`. |
| N2 | Desktop/local SQLite | **CÓ** | `gateway/internal/store/sqlite.go`, `desktop/internal/host/host.go` (opened) default `~/Library/Application Support/GOSO/goso.db`. |
| N3 | Per-user workspace + context files | **THIẾU** | — |
| N4 | AES-256-GCM for API keys at rest | **THIẾU** | Keys are env vars (`llm/registry.go` `GOSO_ANTHROPIC_API_KEY`). No AES/GCM in goso or goso-crm (grep). |
| N5 | Gateway RBAC (admin/operator/viewer) | **THIẾU** | Single Bearer `GOSO_ADMIN_TOKEN` (`auth/auth.go`, `serve/serve.go`). |
| N6 | CRM org tenant header + scoped SQL | **PARTIAL** | Sibling CRM: `goso-crm/db/tenant.go` (opened) fail-closed `X-Org-ID`; Postgres 16 in `goso-crm/docker-compose.yml`. This is CRM org isolation, not agent workspace tenancy. |
| N7 | CRM users/roles settings | **PARTIAL** | `goso-crm/internal/orgset/store.go` + `handler.go` (opened): org User/Role/Flags. Not GoClaw agent permission matrix. |
| N8 | Isolated sessions per tenant | **PARTIAL** | Gateway sessions are global in one SQLite file (`sqlite.go`). CRM conversations are org-scoped (sqlc). |

### 2.8 Providers

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| R0 | Registry of 20+ adapters, capability routing | **PARTIAL** | `gateway/internal/llm/registry.go` (opened): `anthropic`, `openai`, always `echo`. |
| R1 | Anthropic native HTTP + SSE + prompt cache | **PARTIAL** | `gateway/internal/llm/anthropic.go` (opened): `net/http` Messages API, no stream, no cache_control. |
| R2 | OpenAI Chat Completions HTTP | **PARTIAL** | `gateway/internal/llm/openai.go` (opened): non-stream `/v1/chat/completions`. `BaseURL` exists but registry never sets OpenAI-compat variants. |
| R3 | OpenRouter | **THIẾU** | Not in `registry.go`. |
| R4 | Groq | **THIẾU** | — |
| R5 | DeepSeek | **THIẾU** | — |
| R6 | Gemini | **THIẾU** | — |
| R7 | Mistral | **THIẾU** | — |
| R8 | xAI | **THIẾU** | — |
| R9 | MiniMax | **THIẾU** | — |
| R10 | DashScope | **THIẾU** | — |
| R11 | Claude CLI (stdio) | **THIẾU** | — |
| R12 | Codex (OAuth Responses API) | **THIẾU** | — |
| R13 | ACP JSON-RPC subagents | **THIẾU** | — |
| R14 | Any OpenAI-compatible endpoint as a named provider | **THIẾU** | OpenAI struct has `BaseURL` (`openai.go`) but no registry entries / env for groq/ollama/etc. |
| R15 | Echo / missing-key fallback | **CÓ** | `provider.go` `Echo` + `registry.go` always registers `echo` (opened). |
| R16 | MCP provider CRUD vs `/v1/providers` | **PARTIAL** | `mcp/src/client/endpoints/provider-endpoints.ts` (opened) → `/v1/providers`. Gateway exposes `GET /api/providers` (`httpapi/handlers.go`), not `/v1/providers`. |
| R17 | CRM AI draft provider | **PARTIAL** | `goso-crm/internal/ai/provider.go` + `fake_provider.go` (opened): `Generate` interface; fake echoes “Draft: …”; no live Anthropic/OpenAI client in that package. |

### 2.9 Channels

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| C0 | Channel manager + 7 adapters | **PARTIAL** | `GET /api/channels` lists three names (`httpapi/handlers.go` `registerChannels`, opened). |
| C1 | Telegram (long poll, mention gating, STT, forums, HTML pipeline) | **PARTIAL** | `gateway/internal/channel/telegram.go` (opened): webhook text-only, session `telegram:chat_id`, injectable Sender. Not long-poll/STT/forum. SPEC 003 webhook-first. |
| C2 | Discord | **THIẾU** | No discord adapter. MCP blurb mentions Discord (`register-channel-tools.ts`) but client hits `/v1/channels`. |
| C3 | Slack | **THIẾU** | — |
| C4 | Zalo OA | **PARTIAL** | `gateway/internal/channel/zalo_oa.go` (opened): webhook text stub, injectable Sender. SPEC 004: no live OA protocol. |
| C5 | Zalo Personal | **PARTIAL** | `gateway/internal/channel/zalo_personal.go` (opened): webhook text stub. Live QR/inbound lives on CRM (`goso-crm/internal/zaloqr`, `internal/inbound/drain.go` opened) — CRM product, not gateway channel adapter. |
| C6 | Feishu / Lark | **THIẾU** | — |
| C7 | WhatsApp | **THIẾU** | — |
| C8 | WebSocket RPC (chat/agents/teams) | **PARTIAL** | `gateway/internal/httpapi/ws.go` (opened): echo prefix `"echo: "`, `CheckOrigin` always true. Not RPC v3. |
| C9 | Pairing codes / allowlists | **THIẾU** | — |
| C10 | MCP channel list/toggle | **PARTIAL** | `mcp/src/client/endpoints/channel-endpoints.ts` (opened) → `/v1/channels`. |

### 2.10 Security

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| S1 | Layer 1 transport: CORS allowlist, WS 512KB, HTTP 1MB, timing-safe token | **PARTIAL** | Bearer 401 in `auth/auth.go` (opened) uses `got != token` (not constant-time). WS origin allow-all (`ws.go`). Rate-limit exists (S6). No MaxBytesReader seen in opened handlers. |
| S2 | Layer 2 input: 6 injection patterns + configurable action | **THIẾU** | Grep injection/prompt-inject in goso: none. |
| S3 | Layer 3 tool: shell deny, path traversal, SSRF, exec approval | **PARTIAL** | Connector approval gate `gateway/internal/approval/gate.go` (opened). No FS/exec tools, no SSRF checker, no shell deny list. |
| S4 | Layer 4 output: credential scrub + untrusted-content wrap | **PARTIAL** | `eventstore.Redact` in `gateway/internal/eventstore/store.go` (opened) redacts JSON keys (token/password/…). Not GoClaw regex token families / `<<<EXTERNAL_UNTRUSTED_CONTENT>>>`. |
| S5 | Layer 5 isolation: per-user workspace, Docker sandbox, read-only FS | **THIẾU** | No sandbox/workspace isolation. `docs/DEPLOY.md`: sandbox “Ngoài scope”. |
| S6 | Rate limit (token bucket per user/IP) | **CÓ** | `gateway/internal/ratelimit/limiter.go` (opened): per-IP window, `GOSO_RATE_LIMIT`, 429 + Retry-After. In-memory, not Redis. Wired in `serve.go`. |
| S7 | Prompt-injection detect | **THIẾU** | Same as S2. |
| S8 | SSRF (hostname / private IP / DNS pin) | **THIẾU** | Connector HTTP client in `connector/http.go` / `mcp.go` (opened mcp.go) has no pin/blocklist. |
| S9 | AES-256-GCM | **THIẾU** | Same as N4. |
| S10 | Default-closed admin token | **CÓ** | `serve.go` + SPEC 016: empty token without `GOSO_DEV_MODE=1` → 401; `/healthz` bypass (`auth.go` opened). |
| S11 | CRM session cookie + org token | **PARTIAL** | `goso-crm/internal/auth/gate.go` (opened): cookie + `X-Org-Token`; SPEC 033 Secure/`__Host-`. Not gateway 5-layer. |
| S12 | CRM settings SSO / outbound webhooks / export / integrations | **CẮT** | `goso-crm/.planning/specs/033-audit-fix.md` (opened): hide nav. goso `SettingsPage.tsx` (opened) already omits those items (SPEC 033-cp). |
| S13 | Stripe live charging | **CẮT** | SPEC 010/027/033 non-goal. Gateway quota is count-based (`billing/quota.go` opened), `stripe=false` on CRM billing. |

### 2.11 Observability

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| O1 | LLM traces with nested spans (agent/llm/tool/embedding) persisted | **PARTIAL** | `observe/trace.go` + `observe.go` (opened): in-memory ring 200, provider/model/latency/error/tokens; no span tree, no DB flush. `Observer.Wrap` records one row per `Chat`. |
| O2 | Prompt-cache metrics | **THIẾU** | Anthropic path has no cache fields (`anthropic.go`). |
| O3 | Optional OTLP export | **CẮT** | SPEC 008 non-goals (opened): no OTel/Jaeger. `docs/DEPLOY.md` + `compose.prod.yml` (opened): jaeger/otel overlay not shipped. |
| O4 | JSON access log + request id | **CÓ** | `observe/log.go` (opened): `X-Request-ID`, JSON method/path/status/latency, no query/headers. |
| O5 | `/api/stats` `/metrics` counters | **CÓ** | `observe/stats.go` (opened): uptime, request_count, llm_call_count; Prometheus text on `GET /metrics`. SPEC 018 aliases `/api/metrics`. |
| O6 | Grafana / Prometheus SaaS dashboards | **CẮT** | SPEC 008/012/033: Grafana/K8s non-goal. |

### 2.12 Single binary

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| B1 | Static Go gateway binary, no CGO, no Node in the gateway image | **CÓ** | `Dockerfile` (opened): `CGO_ENABLED=0` `go build ./gateway/cmd/goso-gateway`. `Makefile` `build` same. `go.mod` (opened) Go 1.25 + `modernc.org/sqlite`. |
| B2 | ~25 MB size and <1s boot | **chưa verify** | Not measured this pass (would require `go build` artifact size / timing; no product change). |
| B3 | Control plane is a separate Node/Vite app | **PARTIAL** | `control-plane/` + compose service on :3000 (`compose.yml` opened). Gateway itself has no embedded SPA. |

### 2.13 Tools (8 categories)

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| K0 | Tool registry + policy + workspace routing | **PARTIAL** | Connector `Registry` + `Runtime.CallTool` (`connector/connector.go`, `agent/runtime.go` opened): remote HTTP/MCP connectors, not the 8 built-in groups. |
| K1 | Filesystem tools | **THIẾU** | No `read_file`/`write_file` in gateway. |
| K2 | Runtime `exec` + `browser` | **THIẾU** | Approval is for connector mutations, not shell. No Chrome. |
| K3 | Web `web_search` / `web_fetch` | **THIẾU** | — |
| K4 | Memory tools | **THIẾU** | Gateway; MCP client only (M5). |
| K5 | Media generate/read/tts | **THIẾU** | — |
| K6 | Skills BM25 + `use_skill` / `skill_manage` | **PARTIAL** | `mcp/src/tools/register-skill-tools.ts` (opened) → `/v1/skills`. No skill loader in gateway. |
| K7 | Team tools | **PARTIAL** | MCP only (T5). |
| K8 | Automation `cron` / `heartbeat` / `sessions_*` | **PARTIAL** | `mcp/src/tools/register-cron-tools.ts` (opened) → cron CRUD on `/v1`. Gateway `cmd/goso-gateway/main.go` has version/doctor/gateway only. Sessions HTTP CRUD exists (`handlers.go`) but not as agent tools. |
| K9 | MCP bridge (stdio / SSE / streamable HTTP) as **connector transport** | **CÓ** | `gateway/internal/connector/mcp.go` (opened): HTTP + stdio MCP client used to invoke remote tools. |
| K10 | goso-mcp management server (66 tools, dual transport) | **PARTIAL** | `mcp/README.md` + `mcp/src/client/http-client.ts` (opened): talks GoClaw-shaped `/v1/*`, not gateway `/api/*`. |

### 2.14 Desktop Lite

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| D1 | Native Wails v2 + React app | **CÓ** | `desktop/app.go`, `desktop/wails.json`, `desktop/README.md` (opened). |
| D2 | Local SQLite, zero Postgres | **CÓ** | `desktop/internal/host/host.go` (opened). |
| D3 | Chat with agents (streaming, tools, media, attachments) | **PARTIAL** | Control Plane `ChatPage.tsx` (opened) posts to `/api/chat` (non-stream). Connector tools via runtime, not media/attachments. |
| D4 | Max 5 agents / max 1 team | **THIẾU** | `AgentsPage.tsx` has no cap; no teams. |
| D5 | FTS5 memory search | **THIẾU** | No FTS5 (see V2). |
| D6 | Auto-update from GitHub Releases | **CẮT** | SPEC 009 non-goal; SPEC 029 + `scripts/package-desktop.sh` (opened): unsigned zip, **no auto-update**, no notarize. |
| D7 | Lite has no channels | **PARTIAL** | Desktop embeds the same gateway, so channel webhook routes exist if tokens set (`host.go` → `gateway.OpenLocal`). No Lite feature-flag hiding them. |
| D8 | Apple notarize / signed DMG | **CẮT** | SPEC 029/033. |

### 2.15 Webhook API

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| W1 | Tenant webhook registry + Bearer `wh_…` + HMAC `X-GoClaw-Signature` | **THIẾU** | No `/v1/webhooks/llm` or HMAC signing in gateway. Channel POSTs under `/api/channels/*/webhook` are inbound platform webhooks, not this API. |
| W2 | Sync + async LLM trigger + callbacks | **THIẾU** | — |
| W3 | CRM Settings “webhooks” page (outbound HTTP) | **CẮT** | SPEC 033 inventory (opened). |

### 2.16 Optional `WITH_*` / overlays

| # | GoClaw behavior | goso | Evidence |
|---|-----------------|------|----------|
| F1 | `WITH_BROWSER=1` Chrome | **CẮT** | `docs/DEPLOY.md` (opened): browser “Ngoài scope”. `compose.yml` / `compose.prod.yml` have no Chrome service. |
| F2 | `WITH_OTEL=1` Jaeger | **CẮT** | SPEC 008; DEPLOY.md otel row “Chưa”. |
| F3 | `WITH_SANDBOX=1` | **CẮT** | DEPLOY.md sandbox “Ngoài scope”. |
| F4 | `WITH_TAILSCALE=1` | **CẮT** | DEPLOY.md tailscale “Ngoài scope”. |
| F5 | `WITH_REDIS=1` | **CẮT** | DEPLOY.md redis “Chưa — rate-limit in-memory (SPEC 006)”. CRM compose **does** run Redis (`goso-crm/docker-compose.yml` opened) for CRM, not gateway cache. |
| F6 | Core compose + prod overlay | **CÓ** | `compose.yml` + `compose.prod.yml` (opened): gateway, control-plane, SQLite volume, backup sidecar. |
| F7 | K8s / Helm | **CẮT** | SPEC 012 non-goal (opened). |

### 2.17 Adjacent (not README Core Feature, but appears in GoClaw docs / goso today)

Listed so later SPECs do not confuse them with GoClaw parity.

| # | Behavior | goso | Evidence |
|---|----------|------|----------|
| X1 | Billing quota 429 on chat | **CÓ** (goso-specific) | `gateway/internal/billing/quota.go` (opened), wired from chat handlers. Not GoClaw Stripe. |
| X2 | Connector HTTP + fake + health | **CÓ** (goso-specific) | `connector/` (opened). Maps to MCP-bridge subset of GoClaw tools. |
| X3 | CRM live Zalo QR / inbound drain | **CÓ** on CRM, **not** GoClaw channel parity | `goso-crm/internal/zaloqr`, `inbound/drain.go` (opened). |
| X4 | DomainEventBus + consolidation workers | **THIẾU** | No eventbus/consolidation in goso. |

---

## 3. Decision items (SPEC 034 §C)

Do **not** pick vendors or spend money in a worker. Coordinator asks Dat.

| ID | Topic | Why it blocks implementation | Notes (no default) |
|----|--------|------------------------------|--------------------|
| DI-01 | WhatsApp Business API vs native protocol | Channel C7 | GoClaw Standard uses a native stack; Business Cloud API is a different vendor/account. |
| DI-02 | Discord bot token + intents | Channel C2 | Application + bot token + privileged intents. |
| DI-03 | Slack app (Socket Mode vs HTTP) | Channel C3 | Slack app, bot token, signing secret. |
| DI-04 | Feishu / Lark app (app_id / app_secret) | Channel C6 | Feishu open platform app; China vs international host. |
| DI-05 | Zalo OA access token + OA secret | Channel C4 | Official Account; webhook vs long-poll. Gateway stub already reads `GOSO_ZALO_OA_ACCESS_TOKEN`. |
| DI-06 | Zalo Personal live protocol | Channel C5 | Cookie/QR vs CRM sidecar `zca-js`. Product choice: deepen gateway adapter vs reuse CRM sidecar. |
| DI-07 | Telegram bot token | Channel C1 | `GOSO_TELEGRAM_BOT_TOKEN` already in compose; production bot vs demo. |
| DI-08 | Brave (or other) search API key | Tool `web_search` | README names Brave + DuckDuckGo. Do not bind a paid search vendor in a SPEC. |
| DI-09 | pgvector host | Memory L2 + Vault + Standard DB | Self-host Postgres 18+pgvector vs managed. Gateway is SQLite today. |
| DI-10 | OTEL collector / Jaeger | Observability O3, `WITH_OTEL` | Self-host Jaeger vs Grafana Cloud vs none. SPEC 008 cut Grafana SaaS. |
| DI-11 | Tailscale auth / tailnet | `WITH_TAILSCALE` | Account + ACL; security overlay. |
| DI-12 | Sandbox image | `WITH_SANDBOX`, isolation L5 | Which image, resource limits, network policy. |
| DI-13 | Headless Chrome image | `WITH_BROWSER` | Chrome vs Chromium tag; RAM on $5 VPS. |
| DI-14 | Redis | `WITH_REDIS` | Gateway rate-limit is in-memory; CRM already has Redis. Same cluster or not. |
| DI-15 | Apple Developer ID / notarize / Sparkle | Desktop D6/D8 | SPEC 029 CẮT. Revisit only if Dat wants Lite auto-update. |
| DI-16 | Stripe / live charging | S13 | SPEC 010/027 CẮT. Quota-without-money can stay. |
| DI-17 | K8s / Helm | F7 | SPEC 012 CẮT. Infra overlay, not agent parity. |
| DI-18 | Grafana SaaS | O6 | Same. Keep `/metrics` scrape if Dat wants; do not silently drop *or* buy Grafana Cloud. |
| DI-19 | OAuth IdP (SSO) | S12 | SPEC 033 CẮT CRM SSO. GoClaw has OAuth module for Codex/etc. Separate from CRM SSO. |
| DI-20 | Anthropic / OpenAI (and later) API keys | Providers R1–R14 | Already env-shaped; which production keys, prompt-cache billing. |
| DI-21 | Media vendors (image/audio/video/TTS) | Tool K5 | README names multi-provider media; each vendor is a credential. |

---

## 4. Group SPEC plan (SPEC 034 §D)

Order locked by user: **pipeline → memory → vault → teams → providers → channels → security → observability**. One SPEC per group. Infra overlays from 032/033 stay decision items (DI-15…DI-18), not silent drops and not hidden inside these SPECs.

| Future SPEC | Closes matrix rows | Notes |
|-------------|-------------------|--------|
| **035 — Agent pipeline + 4-mode prompt** | P1–P4, Q1–Q4, K0 (loop side) | Clean-room 8-stage runner, Full/Task/Minimal/None, history sanitize, hooks. Replace keyword `matchTools` with LLM tool_use iterations. |
| **036 — Memory L0/L1/L2** | M1–M5, D5, N2 (FTS5 on Lite), K4, K8 `sessions_*` as memory-adjacent | Conversation already PARTIAL (M1). Add episodic summarize, semantic KG, progressive load, `/v1/memory` or `/api/memory` that MCP can target. pgvector host = DI-09. |
| **037 — Knowledge Vault** | V1–V3, K4 `knowledge_graph_search` overlap | Wikilinks, hybrid FTS+vector, FS sync. Depends on 036 store + DI-09. |
| **038 — Teams, delegation, self-evolution** | T1–T6, E1, D4, K7, K8 `cron`/`heartbeat` | Boards, sync/async/bidirectional, auto/explicit/manual, spawn/delegate/team_tasks. Self-evolution guardrails (never identity). Lite caps (max 5 agents / 1 team) here. |
| **039 — Providers** | R0–R14, R16, Q2 (prompt cache with Anthropic/OpenAI), O2 | SSE, prompt cache, named OpenAI-compat (OpenRouter, Groq, DeepSeek, Gemini, Mistral, xAI, MiniMax), DashScope, Claude CLI, Codex, ACP. Credentials = DI-20. Do not invent extra vendors. |
| **040 — Channels + webhook API + WS RPC** | C0–C10, W1–W2, D3 streaming/media, D7, K8 channel `message` | Discord/Slack/Feishu/WhatsApp + deepen Telegram/Zalo. Real WS RPC (replace echo). GoClaw webhook API (Bearer+HMAC, sync/async). Credentials DI-01…DI-07. W3 CRM settings webhooks stay CẮT unless Dat un-cuts. |
| **041 — Security 5-layer + tenancy** | S1–S9, S11–S12 as optional, N1, N3–N5, N8, F3, K1–K3 policy, K2 `exec` | Timing-safe auth, injection, SSRF, AES-256-GCM, workspace isolation, sandbox (DI-12), Postgres tenant_id (DI-09). Filesystem/exec/web tools belong here with deny-lists. SSO = DI-19. |
| **042 — Observability OTLP** | O1–O3, O6 as DI, F2 | Span tree + prompt-cache metrics + optional OTLP. Grafana SaaS remains DI-18. Keep existing JSON logs + `/metrics` (O4, O5 already CÓ). |

### Rows already CÓ (do not reopen unless a later SPEC extends them)

S6 rate-limit, S10 default-closed token, N2 SQLite, R15 echo, B1 static Go binary, O4 logs, O5 stats/metrics, D1–D2 Wails+SQLite, F6 compose core+prod, K9 connector MCP transport, X1 quota, X2 connectors, X3 CRM Zalo QR.

### Stretch / attach (not a ninth product SPEC unless Dat wants)

- **Tools filesystem/runtime/web/media**: implement inside 035 (call path) + 041 (policy) + DI-08/DI-13/DI-21.
- **MCP `/v1` vs gateway `/api`**: fix when 036/038/039/040 add real backends; today MCP is PARTIAL against a missing contract.
- **Self-evolution** rides 038, not a solo SPEC.

### Stay decision items (not SPECs)

K8s (DI-17), Grafana SaaS (DI-18), Apple notarize/auto-update (DI-15), Stripe charging (DI-16), Tailscale (DI-11), Redis overlay (DI-14) unless a later SPEC needs them.

---

## 5. QC

- CÓ/PARTIAL rows cite a path opened this pass (see tables).
- No GoClaw source pasted; no upstream author ids outside `.planning`.
- Demos / ports 8082, 8091, 3000, 18080, 18088 not touched.
- No `gateway/` product code in this worker.
- goso-crm searched read-only; AES/GCM, pgvector, Discord/Slack/Feishu/WhatsApp, OTLP: not present.

Coordinator spot-checks from SPEC 034:

| Check | Result |
|-------|--------|
| Providers | PARTIAL: anthropic + openai + echo (`llm/registry.go`) |
| Channels | telegram / zalo-oa / zalo-personal PARTIAL; Discord/Slack/Feishu/WhatsApp THIẾU |
| SQLite vs Postgres | Gateway SQLite CÓ; gateway Postgres THIẾU; CRM Postgres PARTIAL (org tenant, not agent tenancy) |
