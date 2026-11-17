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
| GOSO_SSRF | off | `1` blocks literal localhost/private IPs (DNS-aware) on connector HTTP, LLM HTTP, builtin `web_fetch`, builtin `browser` `{url}`, and TTS test probes. Production turns this on when the variable is unset. Default off so local fake e2e / httptest still works. `web_fetch` has no extra env; SSRF is its only network policy. |
| GOSO_SSRF_ALLOW_HOSTS | (empty) | Comma-separated hostnames that stay reachable while SSRF is on, optionally `host:port`. Match is the whole hostname (case-insensitive), not a suffix: `9router` allows `http://9router:20128` and does not allow `evil9router`, `9router.internal`, or `not.9router`. An entry without a port allows any port; `9router:20128` allows only that port. Listed names skip the private-address check. |
| GOSO_WORKSPACE | (rỗng) | Write jail. Tools/vault cannot write outside this directory. Empty = builtin filesystem tools (`read_file` `write_file` `list_files` `edit` `send_file` `search` `glob`) fail-closed (`not_configured`, no FS access). Cap read/edit 1MiB. `search` skips files over 1MiB. `send_file` returns metadata only. Each tenant gets its own folder `GOSO_WORKSPACE/<tenant>/` (default included) and file tools, `media` and `sandbox` only see the calling agent's tenant folder. |
| GOSO_MASTER_KEY | (rỗng) | 32-byte hex AES-256-GCM key for `secrets(name, nonce, ct)`. Empty → refuse persist; env provider keys still work. |
| GOSO_RATE_LIMIT | 60 | Giới hạn req/phút/IP (0 = tắt) |
| GOSO_DB_PATH | :memory: (gateway) / OS app-support (desktop) | File SQLite (vd data/goso.db; Docker: `/data/goso.db`) |
| GOSO_DATABASE_URL | (unset) | `postgres://…` opens pgx (connect fail = error, **no** SQLite fallback). Unset = SQLite `GOSO_DB_PATH`. Local: `docker compose --profile postgres up -d postgres` then `postgres://goso:goso@127.0.0.1:5433/goso?sslmode=disable`. See `docs/qa/085-postgres-local.md`. |
| GOSO_MULTI_TENANT | (off) | `1` honors `X-Goso-Tenant` (admin token required for non-default). Unset/demo = always `default`. Tenant lifecycle rows are a separate store: see below. |
| GOSO_KG_EXTRACT | (off) | `1` may insert an L2 entity from assistant lines `Name:` / `Entity:` after chat. Default off so demo chat is unchanged. Tests use `POST /api/kg/entities`. |
| GOSO_EVOLUTION_AUTO | (off) | `1` starts an in-process 1-minute ticker that calls `POST`-equivalent tick per agent. Each agent still needs `auto_adapt=true` (default false). Default off so demo is unchanged. Name/key never auto-change. |
| GOSO_HEARTBEAT | (off) | `1` starts an in-process ticker that stamps `last_heartbeat` (RFC3339 UTC) on `GET /api/stats`. Default **off**. Application-level stamp, not WebSocket ping. Does not run HEARTBEAT.md or channel delivery. |
| GOSO_HEARTBEAT_INTERVAL_SEC | 60 | Seconds between stamps when `GOSO_HEARTBEAT=1`. Values below **30** clamp to 30s. |
| GOSO_BACKUP_DIR | `./var/backups` | Directory for `VACUUM INTO` snapshots (SPEC 070). Empty `GOSO_DB_PATH` cannot be snapshotted. |
| GOSO_VAULT_DIR | `data/vault` | Thư mục markdown/text knowledge vault (`*.md` `*.txt`). Optional `TEAM.md` is prepended to the team system note (SPEC 038). |
| GOSO_EMBED_URL + GOSO_EMBED_MODEL | (rỗng = tắt) | Endpoint `/embeddings` kiểu OpenAI (base `…/v1` hoặc URL đầy đủ `…/embeddings`) và tên model. Cần cả hai **và** Postgres có extension `pgvector` thì vault mới tìm kiếm lai (từ khoá + vector). Thiếu một trong ba thì vault chỉ tìm theo từ khoá; khởi động không bao giờ lỗi vì việc này. SQLite luôn chỉ tìm theo từ khoá. Xem mục "Vault: phạm vi tài liệu và tìm kiếm vector". |
| GOSO_EMBED_API_KEY | (rỗng) | Gửi dưới dạng `Authorization: Bearer`. Không bao giờ in ra log, lỗi hay API. |
| GOSO_EMBED_DIM | (rỗng = theo model) | Số chiều vector; gửi kèm `dimensions` và bắt buộc mọi vector trả về đúng số chiều này. Giá trị sai (không phải số nguyên dương) → trạng thái `config_error`, vault vẫn chạy chỉ với từ khoá. |
| GOSO_EMBED_RPS | 2 | Giới hạn số request/giây tới endpoint embeddings (dùng chung cho ghi tài liệu và backfill). |
| GOSO_EMBED_BATCH | 16 | Số đoạn văn gửi trong một request embeddings (tối đa 2048). |
| GOSO_LITE | (off) | `1` caps **5 agents** and **1 team** (SPEC 038). Control-plane Channels page shows one-line “Lite: channels off” (SPEC 055); `GET /api/channels` still lists adapters with `"lite": true`. 6th `POST /api/agents` and 2nd team → 400. |
| GOSO_ANTHROPIC_API_KEY / GOSO_OPENAI_API_KEY | (empty) | Native LLM keys. Empty → provider absent; `echo` always remains. |
| GOSO_ANTHROPIC_CACHE_MODE / GOSO_PROMPT_CACHE | (empty) | `full` attaches Anthropic `cache_control` on stable prefix blocks (system + bootstrap + last non-user). Other values omit it. OpenAI-compat is never given a fake cache field. |
| GOSO_OPENROUTER_API_KEY, GOSO_GROQ_API_KEY, GOSO_DEEPSEEK_API_KEY, GOSO_GEMINI_API_KEY, GOSO_MISTRAL_API_KEY, GOSO_XAI_API_KEY, GOSO_MINIMAX_API_KEY, GOSO_DASHSCOPE_API_KEY | (empty) | Named OpenAI-compat providers (SPEC 039). Empty = absent. `GET /api/providers` lists configured names only. |
| GOSO_ROUTER9_BASE_URL | (unset = absent) | SPEC 044/045. Set to `http://127.0.0.1:20127/v1` to construct named provider `router9`. Key optional (`GOSO_ROUTER9_API_KEY` may be empty). Model default `ocg/deepseek-v4-flash`. Override with `GOSO_ROUTER9_MODEL` (including `cx/*`). |
| GOSO_LLM_PROVIDER | (unset) | Force Preferred() name (e.g. `router9`) when that provider is constructed. On the bridge it is the OpenAI-compatible fallback type (`custom`, `openai`, `opencode`). |
| GOSO_LLM_RESPONSE_HEADER_TIMEOUT | 180s | How long a chat call waits for upstream status and headers. Integer seconds or a Go duration (`90s`). There is no overall HTTP client timeout, so a long stream is not cut; the request context cancels it. Dial timeout 10s, TLS handshake 10s. The 20s provider-probe client is only for the models probe. |
| GOSO_LLM_FALLBACK_MODEL | (empty) | Model name sent after the primary model exhausts 3 attempts on a network error, 429, or 5xx. Empty disables fallback. A per-agent `fallback_model` in `harness_config_json` (or `params_json`) overrides this. Other 4xx responses are not retried and do not fall back. |
| GOSO_LLM_TIMEOUT | 120s | Parsed for compatibility. Chat calls do not use it as `http.Client.Timeout`. |
| GOSO_KNOWLEDGE_MAX_BYTES | 32768 | Max bytes of synced agent knowledge appended to the bridge system prompt. A per-agent `knowledge_max_bytes` in `harness_config_json` (or `params_json`) overrides this. Non-positive values keep 32768. |
| GOSO_WEB_SEARCH | (off) | Enables builtin `web_search` (also needs the tool switched on in the UI). `1`/`on` = provider order `brave,exa,tavily,ddg`; a comma list sets your own order (e.g. `tavily,ddg`); `ddg` = DuckDuckGo Instant Answer only. A provider without its key is skipped; an error or empty answer falls through to the next one; DDG needs no key and is the usual last resort. Result: `{provider, results:[{title,url,snippet}], fallback_from?}`. Empty env, empty query, or no ready provider = fail-closed `not_configured` (no network). Keys alone never enable it. Builtin `web_fetch` `{url}` is separate: always advertised, HTTP GET, body cap 1MiB, gated only by `GOSO_SSRF`. |
| GOSO_BRAVE_API_KEY | (empty) | Brave Search API key for `web_search` (sent as `X-Subscription-Token`). Secret: never logged or returned. |
| GOSO_EXA_API_KEY | (empty) | Exa search API key for `web_search` (sent as `x-api-key`). Secret. |
| GOSO_TAVILY_API_KEY | (empty) | Tavily search API key for `web_search` (sent as `Authorization: Bearer`). Secret. |
| GOSO_MEDIA / GOSO_MEDIA_* | (off) | Test-only: with `1` **and** a process-injected double, `image_gen`/`tts` call the double instead of the providers below. `media` may run local ffmpeg when `GOSO_FFMPEG` is an existing file, or `ffmpeg` is on PATH **and** `GOSO_MEDIA=1`. Transcode `{in,out}` is jailed to `GOSO_WORKSPACE`. Missing ffmpeg → `not_configured`. |
| GOSO_TTS_PROVIDER / GOSO_TTS_API_KEY / GOSO_TTS_VOICE / GOSO_TTS_MODEL / GOSO_TTS_ENDPOINT / GOSO_TTS_REGION / GOSO_TTS_LANGUAGE / GOSO_TTS_AUTO_APPLY / GOSO_TTS_MAX_CHARS / GOSO_TTS_TIMEOUT_MS / GOSO_TTS_ENABLED | (empty) | SPEC 118 TTS operator config overlay. `GET /api/tts` returns `key_set`/`configured` only — never the key. Env-owned key refuses PUT secrets (409). Builtin `tts` tool synthesizes with `openai` (also any OpenAI-compatible `/v1/audio/speech` via `GOSO_TTS_ENDPOINT`) or `elevenlabs` (needs a voice id), saves `generated/audio/*.mp3` in the tenant workspace and returns the path for `send_file`. `google`/`azure`/`edge` → `not_configured`. Needs `GOSO_WORKSPACE` and the tool switched on in Tools. |
| GOSO_VISION_PROVIDER | (empty) | Builtin `read_image`: name of an env or saved LLM provider that accepts images (OpenAI-compatible or Anthropic). Ignored when `GOSO_VISION_BASE_URL` is set. Empty both → `not_configured`. |
| GOSO_VISION_MODEL | (provider default) | Vision model id for `read_image` (e.g. `gpt-4o-mini`, `claude-sonnet-4-5`). |
| GOSO_VISION_BASE_URL | (empty) | Direct vision endpoint for `read_image` (e.g. `https://api.openai.com/v1`). SSRF-checked. |
| GOSO_VISION_API_KEY | (empty) | Key for `GOSO_VISION_BASE_URL`. Empty is allowed for self-hosted OpenAI-compatible servers. Never logged or returned. |
| GOSO_VISION_TYPE | `openai-compat` | `openai-compat` or `anthropic` for `GOSO_VISION_BASE_URL`. |
| GOSO_STT_URL | (empty) | Builtin `read_audio` / `stt`: OpenAI-compatible base (`https://api.openai.com`, `.../v1`, or the full `.../audio/transcriptions`). Empty → `not_configured`. SSRF-checked. |
| GOSO_STT_API_KEY | (empty) | Bearer key for `GOSO_STT_URL`. Never logged or returned. |
| GOSO_STT_MODEL | `whisper-1` | Transcription model (e.g. `whisper-large-v3` on Groq). |
| GOSO_STT_LANGUAGE | `vi` | Default ISO-639-1 language hint when the call gives none. |
| GOSO_IMAGE_URL | (empty) | Builtin `create_image` (alias `image_gen`): OpenAI-compatible base for `/v1/images/generations`. Empty → `not_configured`. Output saved to `generated/images/` in the tenant workspace. |
| GOSO_IMAGE_API_KEY | (empty) | Bearer key for `GOSO_IMAGE_URL`. Never logged or returned. |
| GOSO_IMAGE_MODEL | `gpt-image-1` | Image model id. |
| GOSO_IMAGE_SIZE | `1024x1024` | Default `WxH` when the call gives none. |
| GOSO_VAULT_DOCLING / GOSO_VAULT_DOCLING_URL | (off) | Builtin `read_document` reads `.txt`/`.md` in process with only `GOSO_WORKSPACE`. PDF, DOCX, XLSX, PPTX and scanned images go to the vault Docling sidecar and need `GOSO_VAULT_DOCLING=1` plus the sidecar URL; otherwise `not_configured`. The bytes are staged under `t/<tenant>/raw/.read-*` in `GOSO_VAULT_DIR` for the call and removed after. Input cap = `GOSO_VAULT_INGEST_MAX_BYTES`. |
| GOSO_FFMPEG | (empty) | Absolute path to an ffmpeg binary for builtin `media`. Existing file configures `media` without needing PATH. Missing path → `not_configured` (does not fall through to PATH). |
| GOSO_SANDBOX_IMAGE | (empty) | Docker image name for builtin `sandbox`. Non-empty **and** `docker` on PATH → `docker run --rm --network=none --memory=256m` timeout 15s, `{cmd}` as argv (never `sh -c`). The calling tenant's folder `GOSO_WORKSPACE/<tenant>` is mounted `-v abs:abs:ro`. Empty or missing docker → `not_configured`, no spawn. Unit tests must not pull images. |
| GOSO_BROWSER_BIN / CHROME_PATH | (empty) | Existing file path for builtin `browser`. Spawns `--headless --disable-gpu --no-sandbox` timeout 20s with `{url}` after `security.CheckURL`. Stdout cap 64KiB. Missing file → `not_configured`, never launch. |
| GOSO_SKILLS_DIR | (empty) | Skill folders. Shared root `<dir>/<name>/SKILL.md` is written only by the `default` tenant. Other tenants read it and write `<dir>/t/<tenant>/<name>/SKILL.md`; the same name overrides the shared file. Empty = fail-closed: `use_skill` / `skill_search` / manage return `not_configured` and `GET /api/skills` is `{skills:[]}`. `GET ?q=` is that tenant's BM25 top 5. Name `[a-z0-9_-]{1,64}` except reserved `t`. No script exec. Cap 64KiB. |
| GOSO_CONTEXT_DIR | (empty) | Bootstrap markdown dir (SPEC 051). Empty = no inject. When set: read only direct children `SOUL.md`, `IDENTITY.md`, `AGENTS.md` (optional `USER.md`), 32KiB each, labeled system text in the pipeline prompt stage. Does not change `display_name` / `agent_key`. Missing dir and `..` are no-ops. |
| GOSO_CONTEXT_FILES | (off) | `1` enables durable agent and user context files (`PUT/GET/DELETE /api/context-files`) and injects them after bootstrap in the API chat prompt. Default off: routes answer 404 and chat does not read the table. Scoped by `tenant_id`; a user file is also scoped by `user_key` (`X-Goso-User` or chat `user_key`). A user file with `agent_id` (scope `user` + `agent_id`, or scope `agent_user`) belongs to that agent only; a turn reads it first and falls back to the tenant-wide user file of the same name. Caps match GoClaw: 512 KiB per file (`GOSO_CONTEXT_FILE_MAX_BYTES`), no file-count or total limit per owner unless `GOSO_CONTEXT_MAX_FILES` / `GOSO_CONTEXT_MAX_BYTES` are set. A larger write is HTTP 400 `too_large` and is not stored. Does not change `agents.instructions`, memory, vault, or `GOSO_CONTEXT_DIR`. |
| GOSO_TELEGRAM_BOT_TOKEN, GOSO_TELEGRAM_MODE, GOSO_TELEGRAM_WEBHOOK_SECRET, GOSO_PUBLIC_URL | (empty; mode default poll) | SPEC 084 live Telegram. Webhook mode requires public URL else health=failed. In webhook mode each bot gives Telegram its own `secret_token` (derived from its bot token); a call carrying it skips the admin token. `GOSO_TELEGRAM_WEBHOOK_SECRET` only applies to calls without it. Header `X-Goso-Telegram-Secret` if webhook secret set. |
| GOSO_ZALO_OA_ACCESS_TOKEN + GOSO_ZALO_OA_SECRET | (empty) | Both required for OA `configured`. Optional `GOSO_ZALO_OA_APP_ID`. Webhook only. Missing secret: demo accept+warn, production fail-closed. |
| GOSO_ZALO_PERSONAL_TOKEN | (empty) | Env wins over secrets-box QR session (`channel:zalo-personal:session`). Logout clears box only. Sidecar injects `POST /api/channels/zalo-personal/webhook`. Unofficial/ban risk. |
| GOSO_ZALO_BOT_MODE | poll | A tenant's `<tenant>/zalo-oa/<name>` channel is a Zalo Bot (bot.zaloplatforms.com), as in GoClaw. Set its token per channel: `PUT /api/channels/{name}/secrets` `{"bot_token":"…"}`. Default `poll` reads `getUpdates` (and removes any webhook first). `webhook` needs `GOSO_PUBLIC_URL`; goso calls `setWebhook` with `<GOSO_PUBLIC_URL>/api/channels/zalo-oa/bot/<tenant>/<name>` and a per-bot `secret_token` derived from the bot token; Zalo sends it as `X-Bot-Api-Secret-Token`, and the call skips the admin token. Only one system may read a bot: stop GoClaw's channel before enabling it here. Text only; replies split at 2000 characters; groups off unless the channel's group policy allows them. |
| (no env) Lark / Feishu app | — | A tenant's `<tenant>/feishu/<name>` channel is a Lark (or Feishu) app bot, as in GoClaw. goso holds Lark's long connection (websocket), so it needs no public URL and no change in the Lark developer console. Set per channel: `PATCH /api/channels/{name}` `{"settings":{"app_id":"cli_…","domain":"lark"}}` (`domain` is `lark`, `feishu` or an https URL; empty tries Lark, then Feishu) and `PUT /api/channels/{name}/secrets` `{"app_secret":"…"}`. The app needs the `im.message.receive_v1` event and the long-connection subscription mode. Text only; group messages need an @mention of the bot when `require_mention` is on; replies answer the message in place and split at 4000 characters. Lark shares a long connection's events between every client connected for the app, so stop GoClaw's channel before enabling it here. |
| (no env) ZaloCRM bridge | — | A tenant's `<tenant>/zalo-personal/<name>` channel answers for one ZaloCRM Zalo account. ZaloCRM posts signed `message.received` events to `POST /api/channels/zalo-personal/bridge` (set its `GOCLAW_BRIDGE_URL` to `<GOSO_PUBLIC_URL>/api/channels/zalo-personal/bridge`). The call skips the admin token; goso checks `X-Goclaw-Signature` (HMAC-SHA256 of `ts.nonce.body`) against the channel's `bridge_secret`, a 5-minute window and nonce replay. Set per channel: `PATCH /api/channels/{name}` `{"settings":{"account_id":"…","crm_base_url":"https://…","group_enabled":false}}` and `PUT /api/channels/{name}/secrets` `{"bridge_secret":"…"}` (the org's bridge secret in ZaloCRM). goso replies to `<crm_base_url>/api/internal/goclaw/reply`. Disabled channel → 503 so ZaloCRM keeps the message. Groups need `group_enabled=true`. |
| GOSO_DISCORD_BOT_TOKEN, GOSO_SLACK_BOT_TOKEN, GOSO_SLACK_APP_TOKEN, GOSO_FEISHU_APP_SECRET, GOSO_WHATSAPP_ACCESS_TOKEN | (empty) | Phase-2 parked (no live Start). Slack `env_names` is bot+app. WhatsApp Cloud-API shape; native = DI-01. PATCH non-secret only. |
| GOSO_WS_ORIGINS | (empty = allow all) | Comma-separated WebSocket Origin allowlist. Empty keeps previous allow-all behaviour. |
| GOSO_ENV | development | Môi trường |
| GOSO_GATEWAY_URL | — | URL gateway cho `goso-mcp` (bắt buộc khi chạy MCP) |
| GOSO_TOKEN | (rỗng) | Bearer token MCP → gateway |
| GOSO_MCP_PORT | 3100 | Cổng Streamable HTTP của MCP |
| GOSO_OTEL_ENDPOINT | (rỗng = noop) | Optional OTLP HTTP JSON collector URL (SPEC 042/087). Empty = no export. Local Jaeger: `docker compose --profile otel up -d jaeger` then host `http://127.0.0.1:4318/v1/traces` or compose-network `http://jaeger:4318/v1/traces`. Do **not** put Grafana Cloud keys here (DI-18). |

Tenant lifecycle (SPEC 151) uses the same database as the rest of the gateway. A SQLite file (`GOSO_DB_PATH`) and Postgres (`GOSO_DATABASE_URL`) create `tenant_registry` and `tenant_registry_members` on open. Those statements only add tables. They do not alter memories, vault, contacts, or any older table. An empty `tenant_registry` loads the master tenant and nothing else, which is the previous startup, so there is no on/off flag. Create, status, and member changes write through in the same call. If that write fails, the API returns an error and the in-memory rows stay as they were. With `GOSO_DB_PATH` unset or `:memory:`, the registry stays in the process, because that mode has no database to reopen.

Scheduled chat (SPEC 054) lives in SQLite `cron_jobs` and an in-process 1-minute ticker. `GET/POST/DELETE /api/cron` (aliased at `/v1/cron`). Specs: `every:Nm|Nh` or optional 5-field (`*`, decimal, `*/n`) evaluated in **UTC**. Cap 20 jobs. Empty list is a no-op. Failed fires are retried next tick. No OS crontab and no extra env.

Run a job now: `POST /api/cron/{id}/run` (also `/v1/cron/{id}/run`). It runs the job of the caller's tenant at once, even when the job is disabled or not due; another tenant's job is 404. The default answer is `202 {"ok":true,"ran":true,"id":…}` and the job runs in the background. `?wait=1` waits and answers `200` with `ok`, the job's fresh `last_run`, and `error: "job run failed"` when it failed. A job already running (by hand or by the ticker) is `409`. Like a scheduled run, success stamps `last_run` (so the next scheduled run counts from now) and failure sets `last_error`. The agent's `cron` tool has the same thing as `action=run {job_id}`: it fires one of the agent's own jobs in the background. It is refused in a customer channel conversation and in a turn that a cron job started.

Optional router9 e2e (`scripts/e2e-router9.sh`, SPEC 055): if `GOSO_ROUTER9_BASE_URL` is unset or `GET /v1/models` is not HTTP 200, the script **skips with exit 0**. If the router is up, it starts an **ephemeral** gateway (`--port 0`; does not bind `:8082` `:8091` `:3000` `:18080` `:18088`), then `POST /api/chat` with the admin Bearer (`GOSO_ADMIN_TOKEN` or `GOSO_E2E_TOKEN`) and catalog default model `ocg/deepseek-v4-flash` (`GOSO_ROUTER9_MODEL` overrides). Asserts HTTP 200 and a non-empty `reply`. No keys are hardcoded. Not part of `make verify`.

Channel turns (Telegram, Zalo OA, Zalo Bot, Zalo personal and the ZaloCRM bridge, Lark/Feishu, WhatsApp, Discord, Slack) run on the full agent runtime, the same pipeline as `POST /api/chat`: tools and connectors, context files, hooks, approvals, history summary and customer-scoped memory. The channel guardrail closes the system prompt. The customer never sees an approval id or a loop-limit marker: a turn that waits for an operator approval with nothing else to say answers "Mình đã ghi nhận yêu cầu, nhân viên sẽ kiểm tra và phản hồi bạn sớm." An archived channel session and a disabled agent still answer, as before. A model error is still not sent to the customer.

With `GOSO_CONTEXT_FILES=1`, a channel turn reads the user context files of the person or group that wrote. The `user_key` uses GoClaw's `user_context_files.user_id` format, so files imported from GoClaw match:

| Chat | `user_key` | Example |
|---|---|---|
| Direct message | sender id (Telegram user id, Zalo uid, Lark `open_id`, Discord/Slack user id) | `123456789`, `ou_abc123` |
| Group | `group:<instance>:<chat id>`, where `<instance>` is the last part of the channel name `<tenant>/<catalog>/<instance>`, or the catalog name for an older single bot | `group:minh-an-bot:-1001234567` |
| Discord server channel | `guild:<guild id>:user:<author id>` (one key per member per server) | `guild:9001:user:4242` |

Files a customer sends on a channel reach the turn too:

| Kind | What the agent gets |
|---|---|
| Image | The picture goes to the model as image input when the turn's provider accepts images (OpenAI-compatible and Anthropic); up to 5 per message, pictures larger than 1600 px are scaled down for the model. The original is saved in the tenant's workspace (`GOSO_WORKSPACE/<tenant>/.uploads/`) and the text says `<media:image path=".uploads/…">`, so `read_image` can read it when the model cannot. |
| Voice / audio | Saved, then transcribed before the turn when `GOSO_STT_URL` is set: `<media:voice …>` followed by `<transcript>…</transcript>`. Without STT the text says `[Voice message not transcribed: speech-to-text is not configured]`. |
| Document | Saved; the text gives the path and says to use `read_document`. A small text file (txt, md, csv, json, …) is also put inline in `<file name="…">…</file>`, up to 50,000 characters. |
| Video | Saved; the agent is told it cannot watch it and should ask what it shows. |
| Sticker | `[sticker]` (Telegram stickers are ignored). |

Limits: 10 MB for an image, 20 MB for any other file, 10 attachments per message, 60 s per download. A file that is too large or fails to download becomes `[sent media (kind) — skipped: reason]`, and the turn still runs. Without `GOSO_WORKSPACE` nothing is saved, but images still reach the model. Download links that carry a token (Telegram's file URL, Slack's private URL, WhatsApp's media URL) are never logged, saved or shown to the model; the Slack token is only sent to Slack's file hosts, and an auth header is dropped on any redirect. What each channel sends: Telegram (photo, document, voice, audio, video, round video, animation), Zalo OA (image, gif, file, audio, video, sticker), Zalo Bot (image with caption only; the platform sends no other media), Zalo personal through the ZaloCRM bridge (photo, file, voice, video, sticker), Lark/Feishu (image, file, audio, video, sticker, pictures inside a rich-text post), Discord, Slack and WhatsApp Cloud (image, document, audio/voice, video; WhatsApp stickers too). The legacy Feishu webhook and the legacy Zalo personal webhook stay text-only.

Chat ids and sessions follow GoClaw too, so a session imported from GoClaw (labelled with its GoClaw session key `agent:<agent>:<instance>:<peer>:<chat>`) is continued:

| Channel | Chat id | Peer | Own session per |
|---|---|---|---|
| Telegram | chat id | group for `group`/`supergroup` | forum topic (`…:topic:<n>`, General is 1), private-chat thread (`…:thread:<n>`) |
| Lark/Feishu | `chat_id`; `<chat_id>:topic:<root_id>` when the channel's `topic_session_mode` setting is `enabled` | group only for `chat_type` `group` | topic (with `topic_session_mode`) |
| Zalo Bot (`zalo-oa` tenant channel) | chat id, else the sender | always direct | — |
| Zalo via ZaloCRM bridge | ZaloCRM `conversationId` | group for group threads | — |
| Discord | channel id | group in a server | — |
| Slack | channel id | direct only for a message in an `im` channel | thread (`…:thread:<thread_ts>`) |
| WhatsApp | `<number>@s.whatsapp.net` | direct | — |

A WhatsApp id contains `@`, which a `user_key` may not, so WhatsApp turns read no user file. `goso-context-import` stores GoClaw's `user_id` verbatim (including `group:` ids) unless `--customer-map` maps it.

The `message` tool sends on a tenant's Telegram, Zalo Bot, Lark, bridged Zalo (ZaloCRM), Discord, Slack and WhatsApp channels: only channels of the agent's own tenant that are bound to it, and only chats that already have a session with it. Discord and Slack need the channel's `bot_token` secret; WhatsApp needs `access_token` and the `phone_number_id` setting. On a bridged Zalo channel the agent can write only after ZaloCRM handed it the conversation (the lease of a customer message) since the gateway started.

Xem thêm: `docs/RUNBOOK.md` (vận hành), `docs/RELEASE.md` (phát hành), `.env.example` (mẫu env).

## Vault: phạm vi tài liệu và tìm kiếm vector

Mỗi tài liệu vault có `scope`, `agent_id`, `team_id` và `customer_key`. Tài liệu cũ (trước bản này) được chuyển thành `scope=tenant`, tức là vẫn hiện như trước.

| scope | Ai thấy |
|---|---|
| `tenant` (mặc định) | Mọi agent của tenant |
| `agent` | Chỉ agent có `agent_id` |
| `team` | Chỉ agent thuộc team `team_id` (trưởng nhóm hoặc thành viên) |
| `personal` | Chỉ agent có `agent_id`, và **không bao giờ** trong hội thoại kênh |

- Một agent thấy tài liệu tenant, tài liệu của chính nó và tài liệu các team của nó; không bao giờ thấy tài liệu `agent`/`personal` của agent khác. Luật này áp cho tool `vault_search`, `vault_read` và HTTP API.
- Hội thoại kênh (Zalo, Telegram, Discord, Slack, Lark, WhatsApp…) được dùng vault, nhưng chỉ thấy tài liệu có `customer_key` rỗng hoặc trùng `customer_key` của khách trong thread đó, và không thấy tài liệu `personal`.
- Ghi: `PUT /api/vault/docs` và upload (`GOSO_VAULT_UPLOAD=1`) nhận thêm `scope`, `agent_id`, `team_id`, `customer_key`. Để trống `scope` mà có `agent_id` → `agent`; có `team_id` → `team`. Trả **400** khi: `scope` lạ; `agent`/`personal` thiếu `agent_id`; `team` thiếu `team_id`; `agent_id` hoặc `team_id` không thuộc tenant của request.
- Đọc qua API: mặc định là góc nhìn quản trị (thấy mọi tài liệu của tenant). Thêm `?agent_id=<id hoặc agent_key>` vào search/get/list để xem đúng như agent đó thấy; agent của tenant khác → 400.
- Import từ GoClaw (`goso-vault-import`) mang theo phạm vi: `agent_id`/`team_id` của GoClaw được đổi sang id goso qua danh bạ đã import (agent, team). Tài liệu có chủ chưa được import thì bị bỏ qua (`agent-unmapped`, `team-unmapped`), không bị nới thành `tenant`.

Tìm kiếm vector (tuỳ chọn):

- Cần Postgres có extension `vector` (pgvector). Gateway thử `CREATE EXTENSION IF NOT EXISTS vector` khi mở DB; không có quyền hoặc không có extension thì bỏ qua, chạy tiếp chỉ với từ khoá. Bảng `vault_chunks` chỉ được tạo khi có pgvector.
- Khi bật (`GOSO_EMBED_URL` + `GOSO_EMBED_MODEL`), mỗi lần ghi tài liệu sẽ cắt đoạn và embed ở nền. Tìm kiếm trộn kết quả từ khoá và vector (reciprocal rank fusion); mỗi kết quả có `match` = `keyword` / `vector` / `both`. Endpoint embeddings lỗi thì tự quay về từ khoá.
- `GET /api/vault/embeddings`: trạng thái của tenant (`not_configured`, `config_error`, `store_unsupported`, `ready`, kèm lỗi gần nhất) và lần backfill gần nhất. `GET /api/memory/index`, `GET /api/kg/index` và `GET /api/kg/graph` báo trạng thái embedding thật (`embedding`, `embedding_configured`, `embedding_used_for: ["vault"]`); memory và KG vẫn tìm theo từ khoá.
- Backfill tài liệu cũ hoặc tài liệu đã đổi: `POST /api/vault/embeddings/backfill?limit=200` (mặc định 200, tối đa 1000 tài liệu mỗi lần; chạy nền, trả 202; đang chạy thì 409). Thêm `&wait=1` để chạy trong request và nhận kết quả (200, lỗi endpoint → 502). Chỉ chạy trong tenant của request, theo giới hạn `GOSO_EMBED_RPS`. Kết quả có `embedded`, `failed`, `remain`; gọi lặp lại tới khi `remain` = false.

## Khoá API theo tenant

- `POST /api/api-keys` (token admin) nhận `tenant_id`. Tenant phải có trong danh sách tenant; để trống là tenant `default`.
- Khoá được lưu vào bảng `api_keys` (chỉ giữ hash và tiền tố), khởi động lại vẫn còn. Số lần dùng và lần dùng cuối chỉ đếm trong bộ nhớ.
- Mọi request dùng khoá chạy trong tenant của khoá. Gửi kèm `X-Goso-Tenant` khác tenant của khoá thì nhận 403.
- Khoá của tenant khác `default`, kể cả scope `admin`, không vào được các đường dẫn toàn hệ thống: `/api/api-keys`, `/api/tenants`, `/api/packages`, `/api/system/backup|restore`, `/api/pairing`.
- Token admin (`GOSO_ADMIN_TOKEN`) giữ cách cũ: chọn tenant bằng header.

## Agent key theo tenant

- `agent_key` là duy nhất trong một tenant, không còn duy nhất toàn máy: ràng buộc là `UNIQUE(tenant_id, agent_key)` (index `idx_agents_tenant_key`). Hai tenant có thể cùng có agent `tro-ly-demo`.
- Mọi chỗ tra agent theo key (kênh Telegram/Zalo, bridge, import agent, import MCP, delegate) chỉ tìm trong tenant của mình. Import agent không còn đổi tên key khi tenant khác đã dùng key đó.
- Nâng cấp tự chạy khi mở database:
  - SQLite: bảng `agents` có `UNIQUE` trên cột `agent_key` được dựng lại trong một transaction (dữ liệu giữ nguyên), rồi tạo `idx_agents_tenant_key`.
  - Postgres: tìm tên thật của ràng buộc unique hoặc index unique chỉ trên `agent_key` qua catalog (bản cũ là `agents_agent_key_key`), xoá, rồi tạo `idx_agents_tenant_key`, tất cả trong một transaction.

## Liên kết agent (agent links)

- `GET /api/agent-links`: mọi liên kết trong tenant của request.
- `GET/POST /api/agents/{id}/links`, `DELETE /api/agents/{id}/links/{to_id}` (alias `/v1/...`). Liên kết giữa hai tenant bị từ chối (400). Bảng `agent_links` có thêm cột `tenant_id` (lấy từ agent nguồn, tự điền cho dòng cũ).
- Quy tắc `delegate`: agent có ít nhất một liên kết đi thì chỉ được giao việc cho agent nó liên kết tới. Agent chưa có liên kết giao được cho mọi agent cùng tenant như trước. `to_agent_id` nhận id hoặc `agent_key` (tra trong tenant của agent gọi). Delegate không còn tự tạo liên kết; tham số `bidirectional` của tool bị bỏ.

## Hook cấu hình theo tenant

- `GET/POST /api/hooks`, `GET/PATCH/DELETE /api/hooks/{id}` (alias `/v1/hooks`). Chỉ thấy và sửa hook của tenant của request; hook tenant khác trả 404. Tạo, sửa, xoá đều ghi audit log (entity `hook`, không ghi secret, chỉ ghi host của URL).
- Trường: `event` (`SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`), `agent_id` (bỏ trống = mọi agent của tenant), `matcher` (glob tên tool, chỉ cho `PreToolUse`/`PostToolUse`; khớp `contact_search` hoặc `zalocrm__contact_search`; trống hoặc `*` = mọi tool), `handler_type`, `config` (JSON), `enabled`, `secret` (chỉ ghi).
- `handler_type: "http"`: POST JSON sự kiện tới `config.url` (http/https, qua SSRF guard khi tạo và khi gửi), timeout `config.timeout_ms` (mặc định 3000, tối đa 10000). Header `X-Goso-Event`, `X-Goso-Hook-Id`, `X-Goso-Timestamp`; có `secret` thì thêm `X-Goso-Signature: sha256=<hex HMAC-SHA256(secret, timestamp + "." + body)>`. Gửi nền, lỗi chỉ ghi log, không làm hỏng lượt chạy.
- `handler_type: "block"` (chỉ `PreToolUse`): chặn tool khớp; model nhận kết quả `blocked by hook: <config.message>`.
- Secret được mã hoá bằng `GOSO_MASTER_KEY` (thiếu key mà gửi secret thì 400). GET chỉ trả `has_secret`; `"secret": ""` khi PATCH để xoá.
- Hook được nạp theo tenant lúc chạy, cache 30 giây, ghi qua API thì cache trong process bị xoá ngay.
