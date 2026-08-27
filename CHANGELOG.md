# Changelog

Mọi thay đổi đáng chú ý của GOSO được ghi ở đây.

## [Unreleased]

### Added

- `PATCH /api/agents/{id}` `{orchestration_mode?: auto|explicit|manual, model?, instructions?}` (SPEC 048). Invalid mode 400; missing agent 404. Agents page mode select PATCHes on change; Teams page shows each member's mode (select when the agent list includes the field). i18n vi+en. StatusLine loading/error.
- `GET /api/webhooks` list `{webhooks:[{id, token_prefix}]}` (SPEC 047); never token/hmac. Control-plane Webhooks tab loads the registry (StatusLine loading/empty/error); last-created secret still once then redacted.
- router9 catalog default `ocg/deepseek-v4-flash` (SPEC 045); `GOSO_ROUTER9_MODEL` still overrides including `cx/*`. SQLite IDs use a random hex suffix so gateway restart no longer collides on `YYYYMMDD-1`.
- Named provider `router9` + Functions page (SPEC 044): construct when `GOSO_ROUTER9_BASE_URL` is set (API key optional empty); OpenAI-compat URL join avoids `/v1/v1`; timeout ≥120s; trailing `data: [DONE]` tolerated. Builtin tools `web_search`/`sandbox`/`browser`/`media` default OFF fail-closed (DDG Instant Answer only when `GOSO_WEB_SEARCH=ddg|1`; no process spawn). HTTP `GET /api/agents/{id}/tools`, `PATCH /api/agents/{id}/tools/{name}`, `PATCH /api/connectors/{name}` (token never returned; `token_set` boolean). Control-plane Functions tab (vi+en). Do not treat live `cx/gpt-5.6-sol` 401 as a product success.
- Nested in-memory spans + optional OTLP (SPEC 042): chat run records `agent` parent with `llm`/`tool` children; `GET /api/traces` returns `{traces, spans}`; `cache_read_tokens` defaults to 0 (Anthropic `cache_read_input_tokens` parsed when present). `GOSO_OTEL_ENDPOINT` empty → noop exporter; set → thin HTTP JSON export. Fake exporter in tests. No Grafana Cloud keys (DI-18). JSON access log and `/metrics` unchanged.
- Security 5-layer + AES-256-GCM secrets (SPEC 041): constant-time Bearer compare; `/api` MaxBytesReader 1MiB; WS 512KiB read limit; chat injection scan (`GOSO_INJECTION=log|block`); connector SSRF when `GOSO_SSRF=1`; untrusted tool wrap `GOSO_UNTRUSTED_BEGIN/END`; optional `GOSO_WORKSPACE` write jail; `secrets(name, nonce, ct)` with `GOSO_MASTER_KEY`; optional `GOSO_VIEW_TOKEN` GET-only. No Postgres `tenant_id` (DI-09). No product secrets in git.
- Channels + webhook API + WS RPC (SPEC 040): Discord/Slack/Feishu/WhatsApp adapters (Cloud-API-shaped WhatsApp stub; native vs Business = DI-01); `GET /api/channels` lists 7 names with `configured` from env; `POST /api/webhooks` (secret once) + `POST /api/webhooks/llm` Bearer `wh_` or HMAC `X-Goso-Signature`; WS JSON `ping`/`pong` + `chat` (not echo-only). Empty `GOSO_WS_ORIGINS` keeps allow-all. Live tokens = DI-01..07 (not in git).
- Named LLM providers (SPEC 039): OpenAI-compat `openrouter` `groq` `deepseek` `gemini` `mistral` `xai` `minimax` `dashscope` via BaseURL+env; `GET /api/providers` lists configured names; Claude CLI / Codex / ACP fail-closed stubs; SSE parser for `stream=true`. Empty env → provider absent, echo fallback. Live keys = DI-20 (not in git).
- Knowledge vault (SPEC 037): `[[wikilink]]` bidirectional registry, FTS5/substring `GET /api/vault/search`, filesystem sync under `GOSO_VAULT_DIR`, HTTP `/api/vault/docs` `/links` `POST /api/vault/sync`. Lexical only; semantic = DI-09.
- Memory L0/L1 (SPEC 036): episodic session summaries, FTS5/substring `GET /api/memory/search`, `GET`/`POST /api/memory`. Pipeline summarize/memory stages filled; no pgvector.
- Hardening (SPEC 013): `make verify` chạy gitleaks + semgrep (bắt buộc trên CI, skip nếu thiếu tool ở local) và `scripts/e2e.sh`.
- `docs/RUNBOOK.md` — khởi động, backup SQLite, xoay token, sự cố.
- `docs/RELEASE.md` — checklist version / changelog / tag.
- Pre-commit và GitHub Actions cài + chạy gitleaks 8.24.3, semgrep 1.110.0 (fail khi có secret).

## [0.1.0] — 2026-08-20

### Added

- Gateway Go: `version`, `doctor`, HTTP `/healthz`, agents/sessions/messages, chat echo/LLM, Telegram + Zalo webhooks.
- Auth Bearer (`GOSO_ADMIN_TOKEN`) + rate limit (`GOSO_RATE_LIMIT`).
- SQLite persist (`GOSO_DB_PATH`) với fallback in-memory.
- Control Plane Vite + React (agents / sessions / chat).
- Harness: Makefile, pre-commit, CI.
