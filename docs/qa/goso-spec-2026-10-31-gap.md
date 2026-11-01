# Gap — GOSO combined spec 2026-10-31 (§0–§14)

Date: 2026-10-31. Source: `GOSO-SPEC-2026-10-31`, product + UI spec for Goso as Company Brain on GoClaw 3.14. Sample company in later specs is Minh An (demo), not a customer record.
Checkout surveyed: `main` at `012858a` (SPEC 131 merged). SPEC 132 (`origin/admatrixmdp/spec132-memory-revive`, 6 commits: cross-session recall, episodic prune, KG supersede, tenant fail-closed) is **not merged**. This gap does not vendor that branch.

Status words: **Có** = the operator surface or gateway behavior exists and matches the spec's intent. **Một phần** = a live page or API exists but misses the spec's contract. **Thiếu** = no corresponding implementation.

Legal constraints from the spec are recorded, not implemented: do not copy OpenKnowledge (GPL-3.0); do not change GoClaw licensing; do not wire Claude CLI / Codex OAuth for commercial use. This repo has `mcp/LICENSE` only. Upstream `nextlevelbuilder/goclaw` `LICENSE` is not in this tree.

## §0 Định vị

**Một phần.** The four layers exist as separate products, not as one Company Brain.

| Layer | Today | Cite |
|---|---|---|
| ① Nạp | Vault put/sync of markdown; storage upload is a different surface | `gateway/internal/vault/vault.go`, `control-plane/src/pages/VaultPage.tsx`, `gateway/internal/httpapi/handlers_storage.go` |
| ② Tri thức | L1 notes, L2 KG lexical, vault wikilinks | `gateway/internal/pipeline/memory.go`, `gateway/internal/store/store.go` (`KGEntity`), `control-plane/src/pages/KnowledgeGraphPage.tsx` |
| ③ Phát ra | Goso exposes an MCP server of `goso_*` tools, not the Vietnamese Company Brain tools | `mcp/src/server.ts` |
| ④ Làm việc | Chat, teams, channels, approvals | `control-plane/src/pages/ChatPage.tsx`, `gateway/internal/agent/runtime.go`, `gateway/internal/approval` |

Memory Facts, `boi_canh_phien`, `ghi_quyet_dinh`, and authority reconciliation are **Thiếu**.

## §1 Điều hướng

**Một phần.** Seven groups from SPEC 119 are live in `liveSide()` (`control-plane/src/App.tsx`) and `control-plane/src/api/hash-route.ts`. Labels are `CỐT LÕI` / `NĂNG LỰC`, not `CHÍNH` / `KHẢ NĂNG` (`control-plane/src/i18n/vi.ts`).

| Spec item | Status | Cite |
|---|---|---|
| Agent + Agent Link & Team merged into Agent Setting, 3 tabs | **Thiếu** | Separate `agents` and `teams` tabs. `TeamsPage` already has Teams \| Links (`control-plane/src/pages/TeamsPage.tsx`) |
| Hội thoại: Session, Tin nhắn chờ, Liên hệ | **Có** | `SessionsPage`, `PendingPage`, `ContactsPage` |
| Kết nối: Channel, Nodes, Workstations | **Có** | matching pages. AI Workers **Thiếu** |
| Khả năng: Skill, tools, MCP, TTS, cron, hooks | **Một phần** | `FunctionsPage` sections + `TTSPage` + `WebhooksPage`. Nav still says "MCP Servers" and "Cron". Lifecycle hooks are not the Webhooks page (`webhooks.notHooks`) |
| Decision, Decision Flow | **Thiếu** | no route |
| Bộ nhớ 7 tab | **Thiếu** | `MemoryPage` is episodic/durable lanes, not 7 tabs |
| Vault, KG, Storage | **Có** as separate pages | |
| Giám sát: traces, events, activity | **Có** as three pages plus Logs. Not one timeline | `TracesPage`, `Events.tsx`, `ActivityPage` |
| Top bar: language, theme | **Có** | `LangToggle`, `useTheme` in `App.tsx` |
| Timezone, account menu (orgs, API keys, logout) | **Thiếu** | avatar is static initials. API keys are a SYSTEM page |

## §2 Trò chuyện

**Một phần**, before SPEC 133. After SPEC 133 the agent/model axes below are **Có** for the listed slice.

| Spec | Before this change | Cite |
|---|---|---|
| 2.1 Claude-like layout, 250px list, 740px thread, greeting, quick prompts, round composer, stop button | **Một phần** | Split is `z-chat-split` (~280px list) in `App.tsx`. Empty state is "Chưa có tin nhắn", not a greeting. No stop control |
| 2.2 Agent picker, model "Theo agent", @ for one message, caption `agent · model`, switch line | **Thiếu** → SPEC 133 | Session had `agent_id` only (`store.Session`). Chat body had no `agent_id`. Composer badge was a static "Agent" label (`ChatPage.tsx`) |
| 2.2 Plain chat, retry with another model, save to vault, thumbs, vision fallback | **Thiếu** | |
| 2.3 Paste, drop, Ctrl+U, Ctrl+K, attachment chips | **Thiếu** (Ctrl+K focuses the agent select in SPEC 133 only) | Footer still says attach/voice/context are unavailable |
| 2.4 One-line activity, expand on click, R2 allow card | **Thiếu** | Stream badge only (`chat.stream.*`) |
| 2.5 Browser server vs "Của tôi" | **Một phần** | `gateway` browser/media notes in `docs/qa/086-sandbox-browser-media.md`. No user-Chrome pairing UI for chat |

## §3 Agent Setting

**Một phần.**

- 3.1 List: search, status, provider filter, create, inline edit, enable (`AgentsPage.tsx`, `control-plane/src/api/agents.ts`). **Thiếu:** grid, pagination, star default, "Đang tiến hóa" badge, context-window chip, transfer (explicitly unavailable: `agents.transferUnavailable`).
- 3.2 Detail is one edit card, not 6 tabs, not a 3-step wizard. Files AGENTS.md / SOUL.md / … are **Thiếu**. Per-agent Decision and allowed-models are **Thiếu**.
- 3.3 Teams and links **Có** on `TeamsPage` (members, lead, tasks, mailbox, evolution guardrails, bidirectional links). Long-term coordination graph **Thiếu**. Gateway tables: `gateway/internal/store/store.go` (`Team`, `AgentLink`), `gateway/internal/team/team.go`.

## §4 Decision

**Thiếu** as a product. Nearby pieces that are not the spec:

- In-process hooks: `session_start`, `user_prompt_submit`, `pre_tool_use`, `post_tool_use`, `stop` (`gateway/internal/pipeline/hooks.go`). No `subagent_start` / `subagent_stop`. No hook UI, no HTTP hook registry. Timeout/SSRF circuit for outbound hooks **Thiếu**.
- Tool approval gate is in-memory (`gateway/internal/approval`) with low/medium/high, not R0–R3 × confidence. UI: `ApprovalsPage.tsx`.
- Orchestration mode auto/explicit/manual (`docs/qa/048-orchestration-mode.md`). Not Jev/Clef.
- No shadow mode, no question bank, no LarkBase sync, no "người sửa đáp án" metric.

## §5 Decision Flow

**Thiếu.** No graph of questions → actions. KG page is entity links, not a decision requirements diagram (`KnowledgeGraphPage.tsx`).

## §6 Bộ nhớ

**Một phần.** SPEC 132 (unmerged) adds cross-session recall and admission on top of this, and still does not add Memory Facts.

| Spec | Status | Cite |
|---|---|---|
| L0 working / L1 episodic / L2 semantic | **Một phần** | Messages are L0 (`pipeline/memory.go`). L1 `SummarizeStage` + `memories`. L2 KG + FTS (`docs/qa/072-memory-l2-fts.md`). Episodic 90-day prune is on SPEC 132 only |
| Memory Facts (subject, attribute, value, validity, authority) | **Thiếu** | |
| Reconciliation create/extend/supersede/dispute/skip | **Thiếu** | KG supersede is SPEC 132 only, and is document-level |
| Recall weights, merged search weights, sliders | **Thiếu** | |
| Admission + "thử một câu hỏi" | **Thiếu** on main. SPEC 132 has recall admission, not the question screen | |
| Dreaming retry button | **Thiếu** | Dreaming called out as a non-goal in `docs/qa/072-memory-l2-fts.md` |
| Scope twice, soft-hide vs forget, passive channel memory | **Một phần** | Tenant on rows (`store` tenant_id). No hide/forget split, no channel passive memory |
| L0 flush before compaction | **Thiếu** | |
| 7 tabs, facts default shadow | **Thiếu** | `MemoryPage.tsx` lanes |

## §7 Vault + Docling

**Một phần.** Live vault matches the "đã dựng lại" note more than the LLM Wiki addition.

**Có:** doc list, type/agent/team filters, wikilink neighborhood, list mode, detail, sync, health (`VaultPage.tsx`, `gateway/internal/vault`, `control-plane/src/api/vault-ops.ts`). Scopes are agent/team fields, not `agents/` `ws/` `tenants/` trees plus `wiki/raw|sources|topics`.

**Thiếu:** wiki layers, `status` / `superseded_by` frontmatter (parser only reads type/agent/team), Docling, naming rule `{YYYY-MM-DD}_{loại}_{chủ-đề}`, 1 MB markdown split by heading, SHA-256 duplicate warning on upload, background "đang xử lý". Upload of PDF/Office is not the vault path (`PUT` JSON body). No Docling reference in the repo.

## §8 Hiểu media

**Thiếu** the spec's screen. No `read_image` / `read_document` / `read_audio` / `read_video`. `builtin` `media` is ffmpeg when `GOSO_MEDIA=1` (`gateway/internal/builtin`). `image_gen` / `tts` return `not_configured` unless configured (`docs/qa/044-di-decisions.md`, `TTSPage.tsx` is the TTS settings page, not a vision chain).

## §9 AI Workers + MCP phát ra

**Một phần.**

- ACP / Claude CLI / Codex providers are stubs that return not configured (`gateway/internal/llm/stubs.go`). Do not wire personal-subscription OAuth for a commercial build.
- Team `spawn` / `delegate` / `team_tasks` exist (`gateway/internal/team/team.go`).
- Nodes and workstations are devices and SSH/Docker targets, not an AI Workers page (`NodesPage.tsx`, `WorkstationsPage.tsx`).
- Gateway consumes external MCP (`gateway/internal/connector/mcp.go`). Goso's own MCP server is `goso_*` (`mcp/src/server.ts`), not `tra_cuu`, `tra_cuu_fact`, `boi_canh_phien`, `ghi_nho`, `ghi_quyet_dinh`, or the write/history/lint set. No mật labels, no per-platform access log.

## §10 OpenKnowledge ideas

**Thiếu.** No lint/audit tab, no `status`/`superseded_by`, no `entities/`, no meeting ingest idempotency key, no per-document hand-vs-agent history, no STOP gate. WYSIWYG / git sync / Slidev / public share links stay out of scope, matching the spec. No OpenKnowledge code is imported.

## §11 Màn hình GoClaw đã dựng lại

**Một phần / Có** as operator pages from SPECs 091–118. Overview KPIs exist (`OverviewPage.tsx`). Usage 24h/7/30 and Usage Cap charts are not the spec's analytics. Session context `K/200K` is not on `SessionsPage`. Pending compaction "50 → 15" is not shown as that policy. Channel create, skills, MCP connectors, TTS, cron, traces, realtime events, and activity each have a page; depth varies (see `docs/qa/091`–`118`). This gap does not re-audit those pages.

## §12 Nhận định

All five notes are **Thiếu** as product changes:

1. Advanced drawer hiding Nodes, Workstations, Hooks, MCP, TTS — they are still top-level.
2. Agent wizard — **Thiếu** (§3).
3. One monitoring timeline — three pages.
4. Admin group for Decision / hooks / permissions / Usage Cap — Decision does not exist; SPEC 119 locked Decision out of the nav on purpose (method A in the new spec puts Decision in Khả năng instead).
5. Overview channel count vs empty Channel page — not verified live here. Two sources (config vs UI-created) are still a risk: `ChannelsPage.tsx` vs gateway channel config (`docs/qa/094-channels-operator.md`).

## §13 Lộ trình

Phase 1 (Vault wiki + Docling, Memory Facts shadow, multi-model chat, paste/drop) is **not done**. Multi-model chat's agent/model axes are SPEC 133. The rest is queued in `.planning/specs/goso-2026-10-31-queue.md`.

Phase 2 (MCP out to Claude + ChatGPT, mật labels, access log, lint) **Thiếu**.
Phase 3 (`ghi_nho`, `ghi_quyet_dinh`, Decision leaves shadow, shop flow) **Thiếu**.
Phase 4 (user browser, AI Workers, entity vault, connectors, first SME sale) **Thiếu**.

## §14 Việc còn treo

| Item | Status |
|---|---|
| Read GoClaw LICENSE + counsel | **Open.** Not in this repo. Do not treat `mcp/LICENSE` as the GoClaw grant |
| Docling OCR on 10 Vietnamese scanned judgments | **Open.** No Docling |
| Confirm `read_image` trace on a live gateway | **Open.** Tool does not exist here |
| `read_document` on scanned PDF | **Open** |
| Verify 34 real tool names | **Open.** Builtin set is goso's, not a checked copy of the v3.11 list |
| Lock 8 Decision questions + named R0–R3 owners into LarkBase | **Open.** No product surface yet |
| Choose the repo for ingest + Goso MCP | **Open.** Ingest would extend `gateway/internal/vault`; MCP would extend `mcp/` unless a new module is chosen |

## Design rule for later SPECs

The 31/10 prototype is the visual source. Chat (SPEC 133) now follows it inside `.z-chat-split` only: white / `#fafbfc` panel, hairline `#e8eaef`, ink `#1d2433`, muted `#636b7d`, faint `#9aa1b0`, accent `#2563eb`, and the matching dark set. The rest of the control plane still uses the existing tokens in `control-plane/src/styles/tokens/`. An app-wide token move is a separate SPEC. Until then, new screens stay sparse: no technical footers, no extra badges, thread width 740px, controls inside the surface they belong to. The full note is in `.planning/specs/goso-2026-10-31-queue.md`.

## SPEC 132 note

Do not rebase this gap's first slice onto `admatrixmdp/spec132-memory-revive`. SPEC 133 does not read recall or KG supersede. SPEC 137 (Memory Facts) **should stack on 132** if 132 is still unmerged, because both touch `gateway/internal/store` memory/KG and `gateway/internal/pipeline`.
