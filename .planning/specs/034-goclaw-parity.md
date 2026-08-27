# SPEC 034 — Full GoClaw feature parity (clean-room rewrite)

> LOCKED: 2026-08-27. North-star = GoClaw README (behavior), **not** SPEC 032/033 ship-tactics.
> License of reference: `goclaw-source` is **CC-BY-NC 4.0 READ-ONLY**. Do **not** copy, vendor, or paste source. Do **not** attribute `minhhaiphan` / `locphamnguyen` outside `.planning`. `./scripts/agpl-check.sh` (goso-crm) and goso equivalent must stay exit 0.
> Demos `:8082` / `:8091` / `:3000` / `:18080` / `:18088` — do **not** kill or rebind.

## Re-anchor

SPEC 032/033 **cut scope** (SSO, webhooks, Grafana, K8s, notarize, Stripe money) was a **ship tactic** for the CRM wedge, **not** a product decision to drop GoClaw features. User: *bê từ goclaw qua* = **full behavioral parity**, clean-room Go.

Context (read, never copy):

- `/Users/mqglobal/Documents/goclaw/goclaw-source/README.md`
- `/Users/mqglobal/Documents/goclaw/goclaw-source/_readmes/`
- `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/` (especially `00-architecture-overview.md`, `01-agent-loop.md`, `02-providers.md`, `05-channels-messaging.md`, `07-bootstrap-skills-memory.md`, `09-security.md`, `10-tracing-observability.md`, `11-agent-teams.md`, `23-multi-tenant-architecture.md`, `24-knowledge-vault.md`)

goso remains AGPL-clean vs ZaloCRM; vs GoClaw it is a **rewrite of behavior**, not a port of files.

## Goal (this SPEC is the umbrella)

Bring goso (and goso-crm where the feature lives) to **full feature parity** with the GoClaw README feature list, via sequential clean-room workers. **Đợt 1 (this lock): inventory + matrix + group SPEC plan only. No product rewrite until the coordinator publishes the matrix and Dat presents it.**

## Worker 1 — inventory + parity matrix (docs only)

Worktree on **goso**. Read-only: goclaw-source, sibling `goso-crm`.

### A) Feature inventory (from README, grouped)

Enumerate **every** README Core Feature plus Desktop Lite vs Standard deltas, Built-in Tools (8 categories), Webhook API, optional Compose flags (`WITH_BROWSER`, `WITH_OTEL`, `WITH_SANDBOX`, `WITH_TAILSCALE`, `WITH_REDIS`). Sub-rows when README names them:

1. **8-stage agent pipeline** — context → history → prompt → think → act → observe → memory → summarize
2. **4-mode prompt** — Full / Task / Minimal / None
3. **3-tier memory L0/L1/L2** — working / episodic / semantic (KG)
4. **Knowledge Vault** — `[[wikilinks]]`, hybrid FTS+pgvector, filesystem sync
5. **Agent Teams** — boards, delegation sync/async/bidirectional, 3 orchestration modes auto/explicit/manual
6. **Self-Evolution** — metrics → suggestions → auto-adapt (never identity)
7. **Multi-tenant Postgres** — per-user workspace, AES-256-GCM API keys, RBAC, isolated sessions
8. **20+ LLM providers** — Anthropic native HTTP+SSE prompt caching; OpenAI-compatible; Groq, DeepSeek, Gemini, Mistral, xAI, MiniMax, DashScope, Claude CLI, Codex, ACP, OpenRouter, any OpenAI-compatible
9. **7 channels** — Telegram, Discord, Slack, Zalo OA, Zalo Personal, Feishu/Lark, WhatsApp
10. **Security** — 5-layer permission, rate limit, prompt-injection detect, SSRF, AES-256-GCM
11. **Observability** — LLM spans + prompt-cache metrics, optional OTLP
12. **Single binary** — ~25MB, no Node runtime, <1s boot
13. **Tools** — filesystem, runtime/exec/browser, web, memory, media, skills, teams, automation/cron
14. **Desktop Lite** — Wails, SQLite, max 5 agents / 1 team, auto-update
15. **Webhook API** — Bearer + HMAC, sync/async
16. **Optional infra** — browser Chrome, Jaeger, sandbox, Tailscale, Redis

### B) Parity matrix

For each row: **GoClaw behavior (one sentence from README/docs)** | **goso status `CÓ` / `THIẾU` / `CẮT` / `PARTIAL`** | **evidence** (`path` + `git` commit on `origin/main`, or `chưa verify`). **Do not guess.** `CẮT` = we previously chose not to ship (032/033 tactics). `THIẾU` = never implemented. `PARTIAL` = interface/stub/echo/one-provider.

Repos to search: this goso tree + `/Users/mqglobal/Documents/goclaw-binary/goso-crm` (read-only).

### C) Decision items (do not auto-choose)

If a feature needs an external credential, vendor account, or product policy, list it as **decision item** (WhatsApp Business API, Discord bot token, Slack app, Feishu app, Zalo OA token, Brave search key, pgvector hosting, OTEL collector, Tailscale, sandbox image, …). Do not invent defaults that spend money or bind a vendor.

### D) Group SPEC plan (no code)

Priority (user): **pipeline → memory → vault → teams → providers → channels → security → observability**. One future SPEC id per group of `THIẾU`/`CẮT`/`PARTIAL`. Non-goals of 032/033 that are **infra overlays** (K8s, Grafana SaaS, Apple notarize, Stripe charging) stay **decision items**, not silent drops.

## Output (worker 1)

- `docs/qa/034-goclaw-parity-matrix.md` — inventory table + matrix + decision items + proposed SPEC ids 035+
- Commit on `admatrixmdp/spec034-parity-matrix` only. **Do not merge. Do not write `gateway/` product code.**

Worker 1 matrix (2026-08-27): [`docs/qa/034-goclaw-parity-matrix.md`](../../docs/qa/034-goclaw-parity-matrix.md)

## Later workers (after Dat presents matrix)

Self-written Go, tests, `go test` / `go build`, agpl-check 0, merge `--no-ff`, `docs/qa/03x-*.md`. Still no paste from goclaw-source.

## Non-goals for worker 1

Implementing pipeline/memory/providers, restarting demos, copying files from `goclaw-source` or `goclaw-docs`, AGPL ZaloCRM copy.

## QC (coordinator)

Evidence spots check: at least providers (goso `gateway/internal/llm` today anthropic+openai+echo), channels (`telegram`/`zalo_oa`/`zalo_personal` exist; Discord/Slack/Feishu/WhatsApp likely THIẾU), SQLite vs Postgres. `agpl-check` N/A if docs-only on goso; still no author ids outside `.planning`.
