# SETUP — GOSO

## 1. Yêu cầu

- Go 1.25+ (`go version`) — Docker image dùng `golang:1.25-alpine`
- Node 20+ và pnpm (cho `mcp/`)
- Git, make
- Docker + Compose v2 nếu chạy stack đóng gói (`docs/DEPLOY.md`)

## 2. Clone & cài

```bash
git clone <repo> && cd goso
go mod download
```

## 3. Chạy gateway skeleton

```bash
go run ./gateway/cmd/goso-gateway --help
go run ./gateway/cmd/goso-gateway version
go run ./gateway/cmd/goso-gateway doctor
# {"name":"goso-gateway","version":"0.1.0"} / checks ok
```

## 4. Verify harness

```bash
make verify   # vet + fmt + test + mcp + gitleaks/semgrep (nếu có) + e2e
make build    # bin/goso-gateway
pnpm -C mcp install && pnpm -C mcp verify
make scan     # gitleaks + semgrep
./scripts/e2e.sh
# optional live router9 (exit 0 if GOSO_ROUTER9_BASE_URL unset or /v1/models down):
./scripts/e2e-router9.sh
```

Cài scanner (khuyến nghị local, bắt buộc trên CI):

```bash
# gitleaks: https://github.com/gitleaks/gitleaks/releases
# semgrep:  pipx install semgrep   # hoặc: uv tool install semgrep
```

MCP (Claude Code / Cursor): xem `mcp/README.md`. Env bắt buộc: `GOSO_GATEWAY_URL` (vd `http://localhost:8080`).

## 5. Desktop (SPEC 009)

```bash
make -C desktop verify    # không cần Wails/CGO
# Dev window (cần Wails CLI + WebKit):
#   go install github.com/wailsapp/wails/v2/cmd/wails@latest
#   cd desktop && wails dev
# macOS binary:
#   make -C desktop build   # → desktop/build/bin/GOSO.app
```

SQLite mặc định: `~/Library/Application Support/GOSO/goso.db` (macOS). Ghi đè: `GOSO_DB_PATH`.

Xem `desktop/README.md`.

## 6. Pre-commit (tùy chọn)

```bash
ln -sf ../../scripts/pre-commit.sh .git/hooks/pre-commit  # nếu goso là repo root
# hoặc chạy thủ công: ./scripts/pre-commit.sh
```

## Docker (tùy chọn)

```bash
docker compose up --build
# http://localhost:8080 (gateway) + http://localhost:3000 (control-plane)
# Default stays SQLite. Optional local Postgres 16 + pgvector (host 5433, not demo ports):
docker compose --profile postgres up -d postgres
export GOSO_DATABASE_URL='postgres://goso:goso@127.0.0.1:5433/goso?sslmode=disable'
# Optional local Jaeger (OTLP HTTP 4318, UI 16686 — not 8082/8091/18080/3000):
docker compose --profile otel up -d jaeger
export GOSO_OTEL_ENDPOINT=http://127.0.0.1:4318/v1/traces
# Compose-network gateway: GOSO_OTEL_ENDPOINT=http://jaeger:4318/v1/traces
```

Chi tiết overlay production: `docs/DEPLOY.md`. Do **not** switch demo `:18080` to Postgres.

## Biến môi trường

| Biến | Mặc định | Mô tả |
|------|----------|-------|
| GOSO_PORT | 8080 | Cổng gateway |
| GOSO_HOST | 127.0.0.1 | Bind host (Docker: `0.0.0.0`) |
| GOSO_LOG_LEVEL | info | Mức log |
| GOSO_ADMIN_TOKEN | (rỗng → 401) | Bearer `/api/*` và `/ws`. Passthrough chỉ khi `GOSO_DEV_MODE=1`. |
| GOSO_VIEW_TOKEN | (rỗng) | Optional GET-only Bearer for `/healthz` `/api/agents` `/api/sessions`. POST `/api/chat` → 403. Admin `POST /api/pairing` issues a 10-minute one-time code that `POST /api/pairing/exchange` swaps once for this token (or a minted `gv_` grant if unset). |
| GOSO_INJECTION | log | `log` (default) or `block`. Scan user chat for documented injection patterns; block → 400 on `/api/chat`. |
| GOSO_SSRF | off | `1` blocks literal localhost/private IPs (DNS-aware) on connector HTTP, LLM HTTP, builtin `web_fetch`, and builtin `browser` `{url}`. Default off so local fake e2e / httptest still works. `web_fetch` has no extra env; SSRF is its only network policy. |
| GOSO_WORKSPACE | (rỗng) | Write jail. Tools/vault cannot write outside this directory. Empty = builtin filesystem tools (`read_file` `write_file` `list_files` `edit` `send_file` `search` `glob`) fail-closed (`not_configured`, no FS access). Cap read/edit 1MiB. `search` skips files over 1MiB. `send_file` returns metadata only. |
| GOSO_MASTER_KEY | (rỗng) | 32-byte hex AES-256-GCM key for `secrets(name, nonce, ct)`. Empty → refuse persist; env provider keys still work. |
| GOSO_RATE_LIMIT | 60 | Giới hạn req/phút/IP (0 = tắt) |
| GOSO_DB_PATH | :memory: (gateway) / OS app-support (desktop) | File SQLite (vd data/goso.db; Docker: `/data/goso.db`) |
| GOSO_DATABASE_URL | (unset) | `postgres://…` opens pgx (connect fail = error, **no** SQLite fallback). Unset = SQLite `GOSO_DB_PATH`. Local: `docker compose --profile postgres up -d postgres` then `postgres://goso:goso@127.0.0.1:5433/goso?sslmode=disable`. See `docs/qa/085-postgres-local.md`. |
| GOSO_MULTI_TENANT | (off) | `1` honors `X-Goso-Tenant` (admin token required for non-default). Unset/demo = always `default`. |
| GOSO_KG_EXTRACT | (off) | `1` may insert an L2 entity from assistant lines `Name:` / `Entity:` after chat. Default off so demo chat is unchanged. Tests use `POST /api/kg/entities`. |
| GOSO_EVOLUTION_AUTO | (off) | `1` starts an in-process 1-minute ticker that calls `POST`-equivalent tick per agent. Each agent still needs `auto_adapt=true` (default false). Default off so demo is unchanged. Name/key never auto-change. |
| GOSO_HEARTBEAT | (off) | `1` starts an in-process ticker that stamps `last_heartbeat` (RFC3339 UTC) on `GET /api/stats`. Default **off**. Application-level stamp, not WebSocket ping. Does not run HEARTBEAT.md or channel delivery. |
| GOSO_HEARTBEAT_INTERVAL_SEC | 60 | Seconds between stamps when `GOSO_HEARTBEAT=1`. Values below **30** clamp to 30s. |
| GOSO_BACKUP_DIR | `./var/backups` | Directory for `VACUUM INTO` snapshots (SPEC 070). Empty `GOSO_DB_PATH` cannot be snapshotted. |
| GOSO_VAULT_DIR | `data/vault` | Thư mục markdown/text knowledge vault (`*.md` `*.txt`). Optional `TEAM.md` is prepended to the team system note (SPEC 038). |
| GOSO_LITE | (off) | `1` caps **5 agents** and **1 team** (SPEC 038). Control-plane Channels page shows one-line “Lite: channels off” (SPEC 055); `GET /api/channels` still lists adapters with `"lite": true`. 6th `POST /api/agents` and 2nd team → 400. |
| GOSO_ANTHROPIC_API_KEY / GOSO_OPENAI_API_KEY | (empty) | Native LLM keys. Empty → provider absent; `echo` always remains. |
| GOSO_ANTHROPIC_CACHE_MODE / GOSO_PROMPT_CACHE | (empty) | `full` attaches Anthropic `cache_control` on stable prefix blocks (system + bootstrap + last non-user). Other values omit it. OpenAI-compat is never given a fake cache field. |
| GOSO_OPENROUTER_API_KEY, GOSO_GROQ_API_KEY, GOSO_DEEPSEEK_API_KEY, GOSO_GEMINI_API_KEY, GOSO_MISTRAL_API_KEY, GOSO_XAI_API_KEY, GOSO_MINIMAX_API_KEY, GOSO_DASHSCOPE_API_KEY | (empty) | Named OpenAI-compat providers (SPEC 039). Empty = absent. `GET /api/providers` lists configured names only. |
| GOSO_ROUTER9_BASE_URL | (unset = absent) | SPEC 044/045. Set to `http://127.0.0.1:20127/v1` to construct named provider `router9`. Key optional (`GOSO_ROUTER9_API_KEY` may be empty). Model default `ocg/deepseek-v4-flash`. Override with `GOSO_ROUTER9_MODEL` (including `cx/*`). |
| GOSO_LLM_PROVIDER | (unset) | Force Preferred() name (e.g. `router9`) when that provider is constructed. |
| GOSO_WEB_SEARCH | (off) | `ddg` or `1` enables builtin `web_search` (DuckDuckGo Instant Answer → `{title,url,snippet}[]`). Empty env, empty query, or empty provider base = fail-closed `not_configured` (no network). Builtin `web_fetch` `{url}` is separate: always advertised, HTTP GET, body cap 1MiB, gated only by `GOSO_SSRF`. |
| GOSO_MEDIA / GOSO_MEDIA_* | (off) | `image_gen`/`tts` stay `not_configured` unless `1` **and** a process-injected test double. Never a paid API. `media` may run local ffmpeg when `GOSO_FFMPEG` is an existing file, or `ffmpeg` is on PATH **and** `GOSO_MEDIA=1`. Transcode `{in,out}` is jailed to `GOSO_WORKSPACE`. Missing ffmpeg → `not_configured`. |
| GOSO_FFMPEG | (empty) | Absolute path to an ffmpeg binary for builtin `media`. Existing file configures `media` without needing PATH. Missing path → `not_configured` (does not fall through to PATH). |
| GOSO_SANDBOX_IMAGE | (empty) | Docker image name for builtin `sandbox`. Non-empty **and** `docker` on PATH → `docker run --rm --network=none --memory=256m` timeout 15s, `{cmd}` as argv (never `sh -c`). `GOSO_WORKSPACE` if set is mounted `-v abs:abs:ro`. Empty or missing docker → `not_configured`, no spawn. Unit tests must not pull images. |
| GOSO_BROWSER_BIN / CHROME_PATH | (empty) | Existing file path for builtin `browser`. Spawns `--headless --disable-gpu --no-sandbox` timeout 20s with `{url}` after `security.CheckURL`. Stdout cap 64KiB. Missing file → `not_configured`, never launch. |
| GOSO_SKILLS_DIR | (empty) | One-level skill folders (`<dir>/<name>/SKILL.md`). Empty = fail-closed: `use_skill` / `skill_search` / manage return `not_configured` and `GET /api/skills` is `{skills:[]}`. `GET ?q=` BM25 top 5. `POST`/`DELETE` jailed. Name `[a-z0-9_-]{1,64}`. No script exec. Cap 64KiB. |
| GOSO_CONTEXT_DIR | (empty) | Bootstrap markdown dir (SPEC 051). Empty = no inject. When set: read only direct children `SOUL.md`, `IDENTITY.md`, `AGENTS.md` (optional `USER.md`), 32KiB each, labeled system text in the pipeline prompt stage. Does not change `display_name` / `agent_key`. Missing dir and `..` are no-ops. |
| GOSO_TELEGRAM_BOT_TOKEN, GOSO_TELEGRAM_MODE, GOSO_TELEGRAM_WEBHOOK_SECRET, GOSO_PUBLIC_URL | (empty; mode default poll) | SPEC 084 live Telegram. Webhook mode requires public URL else health=failed. Header `X-Goso-Telegram-Secret` if webhook secret set. |
| GOSO_ZALO_OA_ACCESS_TOKEN + GOSO_ZALO_OA_SECRET | (empty) | Both required for OA `configured`. Optional `GOSO_ZALO_OA_APP_ID`. Webhook only. Missing secret: demo accept+warn, production fail-closed. |
| GOSO_ZALO_PERSONAL_TOKEN | (empty) | Env wins over secrets-box QR session (`channel:zalo-personal:session`). Logout clears box only. Sidecar injects `POST /api/channels/zalo-personal/webhook`. Unofficial/ban risk. |
| GOSO_DISCORD_BOT_TOKEN, GOSO_SLACK_BOT_TOKEN, GOSO_SLACK_APP_TOKEN, GOSO_FEISHU_APP_SECRET, GOSO_WHATSAPP_ACCESS_TOKEN | (empty) | Phase-2 parked (no live Start). Slack `env_names` is bot+app. WhatsApp Cloud-API shape; native = DI-01. PATCH non-secret only. |
| GOSO_WS_ORIGINS | (empty = allow all) | Comma-separated WebSocket Origin allowlist. Empty keeps previous allow-all behaviour. |
| GOSO_ENV | development | Môi trường |
| GOSO_GATEWAY_URL | — | URL gateway cho `goso-mcp` (bắt buộc khi chạy MCP) |
| GOSO_TOKEN | (rỗng) | Bearer token MCP → gateway |
| GOSO_MCP_PORT | 3100 | Cổng Streamable HTTP của MCP |
| GOSO_OTEL_ENDPOINT | (rỗng = noop) | Optional OTLP HTTP JSON collector URL (SPEC 042/087). Empty = no export. Local Jaeger: `docker compose --profile otel up -d jaeger` then host `http://127.0.0.1:4318/v1/traces` or compose-network `http://jaeger:4318/v1/traces`. Do **not** put Grafana Cloud keys here (DI-18). |

Scheduled chat (SPEC 054) lives in SQLite `cron_jobs` and an in-process 1-minute ticker. `GET/POST/DELETE /api/cron` (aliased at `/v1/cron`). Specs: `every:Nm|Nh` or optional 5-field (`*`, decimal, `*/n`) evaluated in **UTC**. Cap 20 jobs. Empty list is a no-op. Failed fires are retried next tick. No OS crontab and no extra env.

Optional router9 e2e (`scripts/e2e-router9.sh`, SPEC 055): if `GOSO_ROUTER9_BASE_URL` is unset or `GET /v1/models` is not HTTP 200, the script **skips with exit 0**. If the router is up, it starts an **ephemeral** gateway (`--port 0`; does not bind `:8082` `:8091` `:3000` `:18080` `:18088`), then `POST /api/chat` with the admin Bearer (`GOSO_ADMIN_TOKEN` or `GOSO_E2E_TOKEN`) and catalog default model `ocg/deepseek-v4-flash` (`GOSO_ROUTER9_MODEL` overrides). Asserts HTTP 200 and a non-empty `reply`. No keys are hardcoded. Not part of `make verify`.

Xem thêm: `docs/RUNBOOK.md` (vận hành), `docs/RELEASE.md` (phát hành), `.env.example` (mẫu env).
