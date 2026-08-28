# RUNBOOK — GOSO Gateway

Vận hành GOSO core (gateway + SQLite). Không bao gồm ZaloCRM.

## 1. Khởi động

Yêu cầu: Go 1.25+, `make`.

```bash
# Dev tường minh (passthrough — chỉ loopback)
GOSO_DEV_MODE=1 go run ./gateway/cmd/goso-gateway gateway --port 8080 --host 127.0.0.1

# Local bền (SQLite + admin token)
mkdir -p data
export GOSO_ADMIN_TOKEN='<token-dài-ngẫu-nhiên>'
export GOSO_DB_PATH=data/goso.db
export GOSO_VAULT_DIR=data/vault
# export GOSO_SKILLS_DIR=data/skills   # empty = use_skill / skill_search / manage fail-closed
export GOSO_ENV=production
export GOSO_RATE_LIMIT=60
go run ./gateway/cmd/goso-gateway gateway --port 8080 --host 127.0.0.1
# hoặc: make build && ./bin/goso-gateway gateway --port 8080
```

Control Plane (tùy chọn):

```bash
# terminal 1: gateway như trên
# terminal 2:
cd control-plane && npm install && VITE_GOSO_ADMIN_TOKEN="$GOSO_ADMIN_TOKEN" npm run dev
# http://localhost:3000
```

Probe:

```bash
curl -sS http://127.0.0.1:8080/healthz
# {"ok":true,"version":"0.1.0"}

# Optional application heartbeat (default off). Not a WebSocket ping.
# curl -sS -H "Authorization: Bearer $GOSO_ADMIN_TOKEN" -X POST http://127.0.0.1:8080/api/system/heartbeat
# curl -sS -H "Authorization: Bearer $GOSO_ADMIN_TOKEN" http://127.0.0.1:8080/api/stats
# last_heartbeat is omitted until POST or GOSO_HEARTBEAT=1 ticker (60s, min 30s).
```

`/healthz` không cần Bearer token. Mọi `/api/*` và `/ws` cần `Authorization: Bearer $GOSO_ADMIN_TOKEN`, trừ khi `GOSO_DEV_MODE=1`. Optional `GOSO_VIEW_TOKEN` may GET `/healthz` `/api/agents` `/api/sessions` (and a single id segment) but not POST chat or `.../messages`. Token rỗng + không dev-mode → 401.

Pairing (SPEC 077): admin `POST /api/pairing` returns a one-time code (10 minutes, shown once). `POST /api/pairing/exchange` `{code}` is unauthenticated (the code is the secret) and returns the view grant once. View-token stays GET-only — POST backup / kg write / skills write / evolution tick → 403. No OAuth/SSO (DI-19).

`GET /api/traces` returns LLM traces plus nested span trees. `GET /metrics` and JSON access logs stay as in SPEC 008.

Optional OTLP: set `GOSO_OTEL_ENDPOINT` to a collector HTTP JSON URL. Empty (default) = no export. Do not put Grafana Cloud API keys in env (DI-18).

Dừng: `SIGINT`/`SIGTERM` (Ctrl-C) — gateway shutdown 5s.

## 2. Backup SQLite

Supported path: consistent snapshot via `VACUUM INTO` (SQLite backup API). Copying the live db file (`cp`) is **not** the supported path.

`GOSO_BACKUP_DIR` defaults to `./var/backups`. Admin `POST /api/system/backup` creates a timestamped file and runs `PRAGMA integrity_check` on the snapshot (`{file, bytes, integrity:"ok"}`). `GET /api/system/backup` lists files with an integrity badge. In-memory (`GOSO_DB_PATH` empty or `:memory:`) cannot be snapshotted.

```bash
export GOSO_DB_PATH=data/goso.db
export GOSO_BACKUP_DIR=./var/backups
curl -sS -X POST -H "Authorization: Bearer $GOSO_ADMIN_TOKEN" \
  http://127.0.0.1:8080/api/system/backup
# {"file":"goso-20260828T120000Z.db","bytes":…,"integrity":"ok"}
```

Restore **drill** (temp db; live file is not overwritten):

```bash
./bin/goso-gateway restore --file goso-20260828T120000Z.db
# or: POST /api/system/restore {"file":"goso-….db"}
```

Production apply (stop the gateway first):

```bash
# dừng gateway
./bin/goso-gateway restore --file goso-20260828T120000Z.db --apply --dest "$GOSO_DB_PATH"
# khởi động lại gateway với cùng GOSO_DB_PATH
```

Prod compose sidecar calls gateway `POST /api/system/backup` (same `VACUUM INTO` + integrity_check) into volume `backup`, retain `BACKUP_RETAIN` (default 14, hourly `BACKUP_INTERVAL_SECONDS=3600`).

## 3. Xoay token / API key

Tất cả secret đi qua env, không commit. Đổi giá trị rồi **restart gateway**.

| Biến | Dùng cho | Cách xoay |
|------|----------|-----------|
| `GOSO_ADMIN_TOKEN` | Bearer `/api/*`, `/ws` | Sinh token mới (`openssl rand -hex 32`), set env, restart, cập nhật client/control-plane (`VITE_GOSO_ADMIN_TOKEN` / `localStorage.goso_token`). Token cũ hết hiệu lực ngay. |
| `GOSO_VIEW_TOKEN` | GET `/healthz` `/api/agents` `/api/sessions` | Optional viewer Bearer. POST chat → 403. Rotate like admin token. |
| `GOSO_MASTER_KEY` | AES-256-GCM `secrets` table | `openssl rand -hex 32`. Empty = refuse store (env LLM keys still work). Losing the key loses stored blobs. |
| `GOSO_ANTHROPIC_API_KEY` | LLM Anthropic | Tạo key mới trên console Anthropic, set env, restart. Key cũ revoke trên console. |
| `GOSO_OPENAI_API_KEY` | LLM OpenAI | Tương tự. |
| `GOSO_OPENROUTER_API_KEY` / `GOSO_GROQ_API_KEY` / `GOSO_DEEPSEEK_API_KEY` / `GOSO_GEMINI_API_KEY` / `GOSO_MISTRAL_API_KEY` / `GOSO_XAI_API_KEY` / `GOSO_MINIMAX_API_KEY` / `GOSO_DASHSCOPE_API_KEY` | Named OpenAI-compat (SPEC 039) | Tạo key trên console vendor, set env, restart. Empty = provider absent. |
| `GOSO_TELEGRAM_BOT_TOKEN` | Telegram `sendMessage` | `@BotFather` /revoke rồi token mới, cập nhật webhook nếu có. |
| `GOSO_ZALO_OA_ACCESS_TOKEN` | Zalo OA | Làm mới access token OA, restart. |
| `GOSO_ZALO_PERSONAL_TOKEN` | Zalo Personal | Làm mới token, restart. |
| `GOSO_DISCORD_BOT_TOKEN` / `GOSO_SLACK_BOT_TOKEN` / `GOSO_FEISHU_APP_SECRET` / `GOSO_WHATSAPP_ACCESS_TOKEN` | Channel adapters (SPEC 040/078) | Placeholders only in git. Live tokens = DI-01..07. Empty = `configured: false`, `missing: true` on `GET /api/channels`. Names listed in `env_names[]`. PATCH does not store tokens. |

Không log token (auth middleware chỉ trả `{"error":"unauthorized"}`). Không dán key vào issue/chat.

Nếu `GOSO_ADMIN_TOKEN` rỗng và không có `GOSO_DEV_MODE=1`: `/api/*` trả **401**. Passthrough chỉ khi `GOSO_DEV_MODE=1`. Không dùng passthrough trên host public.

## 4. Sự cố

### Gateway không listen

- `go run ./gateway/cmd/goso-gateway doctor` — kiểm tra Go / port / env.
- Port bận: đổi `GOSO_PORT` hoặc `--port`.
- Log lúc start: `listen host:port: ...`.

### `/healthz` không 200

- Process chết / OOM — xem log stdout.
- Sai host (gateway bind `127.0.0.1`, probe từ máy khác sẽ fail) — bind `0.0.0.0` khi cần.

### `/api/*` trả 401

- Thiếu/sai `Authorization: Bearer ...`.
- Token đã xoay, client còn token cũ.
- Control Plane: set `VITE_GOSO_ADMIN_TOKEN` hoặc `localStorage.setItem("goso_token", "...")`.

### `/api/*` trả 429

- Vượt `GOSO_RATE_LIMIT` (mặc định 60 req/phút/IP). Đợi `Retry-After` hoặc tăng/tắt (`GOSO_RATE_LIMIT=0`) cho load test.

### Chat trả `echo: ...` thay vì model thật

- Chưa set native / named LLM keys (`GOSO_ANTHROPIC_API_KEY`, `GOSO_OPENAI_API_KEY`, `GOSO_GROQ_API_KEY`, …) → provider `echo`. `GET /api/providers` lists configured names only (never secrets).
- Key sai → LLM 502 (`Bad Gateway`). Kiểm tra key, không commit key.

### SQLite lỗi / dữ liệu mất

- `GOSO_DB_PATH` chưa set → in-memory.
- File không ghi được (quyền, disk đầy) — `df -h`, `ls -l data/`.
- Restore drill: `goso-gateway restore --file <snapshot>` (temp). Live apply: stop gateway, then `--apply`.

### Telegram/Zalo webhook 200 nhưng không nhắn ra channel

- Thiếu bot/OA token: webhook vẫn 200 (tránh retry storm) kèm `warning` trong body. Set token rồi restart.
- Payload rỗng / không có text → 200 no-op.

### `make verify` đỏ

- `gofmt` — `make fmt-fix`
- test fail — `go test ./... -count=1`
- gitleaks — secret trong working tree, xoá/rotate, không commit
- semgrep — xem rule id, sửa source
- e2e — đọc log script; cần `curl` + `python3`; cổng local tự chọn (`--port 0`)

## 5. RPO / RTO, TLS, cảnh báo

Không có SLA vendor. Con số dưới đây là vận hành GOSO SQLite, không phải cam kết hợp đồng.

| | Ý nghĩa |
|---|---------|
| **RPO** | Worst case mất dữ liệu từ lần snapshot toàn vẹn gần nhất. Prod sidecar mặc định mỗi `BACKUP_INTERVAL_SECONDS` (3600s) — tối đa ~1 giờ nếu interval giữ nguyên. Snapshot thủ công (`POST /api/system/backup`) rút RPO xuống lần bấm gần nhất. |
| **RTO** | Dừng gateway, `goso-gateway restore --file … --apply`, khởi động lại. Thường vài phút trên host local; không đo fake. |

TLS: compose không terminate TLS. Khi public, đặt reverse proxy (Caddy/nginx) trước `:8080` / `:3000`. Không bật Grafana. **DI-10** (OTEL collector / Jaeger vendor) và **DI-18** (Grafana SaaS) vẫn parked — `GOSO_OTEL_ENDPOINT` rỗng = noop; không nhét API key Grafana Cloud.

Cảnh báo: compose `healthcheck` + `GET /healthz`. Không có pager / alert vendor trong SPEC 070.

## 6. Kiểm tra nhanh sau sự cố

```bash
curl -sS http://127.0.0.1:8080/healthz
curl -sS -H "Authorization: Bearer $GOSO_ADMIN_TOKEN" http://127.0.0.1:8080/api/agents
make verify
```
