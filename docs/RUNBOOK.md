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
```

`/healthz` không cần Bearer token. Mọi `/api/*` và `/ws` cần `Authorization: Bearer $GOSO_ADMIN_TOKEN`, trừ khi `GOSO_DEV_MODE=1`. Token rỗng + không dev-mode → 401.

Dừng: `SIGINT`/`SIGTERM` (Ctrl-C) — gateway shutdown 5s.

## 2. Backup SQLite

Khi `GOSO_DB_PATH` trỏ file (vd `data/goso.db`):

```bash
# Snapshot (gateway có thể đang chạy; copy file là đủ vì chưa bật WAL)
ts=$(date +%Y%m%d-%H%M%S)
mkdir -p backups
cp -p "$GOSO_DB_PATH" "backups/goso-${ts}.db"
# nếu có sidecar:
cp -p "${GOSO_DB_PATH}-wal" "backups/goso-${ts}.db-wal" 2>/dev/null || true
cp -p "${GOSO_DB_PATH}-shm" "backups/goso-${ts}.db-shm" 2>/dev/null || true
```

Restore:

```bash
# dừng gateway trước
cp -p backups/goso-<ts>.db "$GOSO_DB_PATH"
# khởi động lại gateway với cùng GOSO_DB_PATH
```

In-memory (`GOSO_DB_PATH` rỗng hoặc `:memory:`) — không có backup; dữ liệu mất khi process thoát.

Khuyến nghị: cron copy `data/goso.db` mỗi giờ, giữ 7 ngày.

## 3. Xoay token / API key

Tất cả secret đi qua env, không commit. Đổi giá trị rồi **restart gateway**.

| Biến | Dùng cho | Cách xoay |
|------|----------|-----------|
| `GOSO_ADMIN_TOKEN` | Bearer `/api/*`, `/ws` | Sinh token mới (`openssl rand -hex 32`), set env, restart, cập nhật client/control-plane (`VITE_GOSO_ADMIN_TOKEN` / `localStorage.goso_token`). Token cũ hết hiệu lực ngay. |
| `GOSO_ANTHROPIC_API_KEY` | LLM Anthropic | Tạo key mới trên console Anthropic, set env, restart. Key cũ revoke trên console. |
| `GOSO_OPENAI_API_KEY` | LLM OpenAI | Tương tự. |
| `GOSO_OPENROUTER_API_KEY` / `GOSO_GROQ_API_KEY` / `GOSO_DEEPSEEK_API_KEY` / `GOSO_GEMINI_API_KEY` / `GOSO_MISTRAL_API_KEY` / `GOSO_XAI_API_KEY` / `GOSO_MINIMAX_API_KEY` / `GOSO_DASHSCOPE_API_KEY` | Named OpenAI-compat (SPEC 039) | Tạo key trên console vendor, set env, restart. Empty = provider absent. |
| `GOSO_TELEGRAM_BOT_TOKEN` | Telegram `sendMessage` | `@BotFather` /revoke rồi token mới, cập nhật webhook nếu có. |
| `GOSO_ZALO_OA_ACCESS_TOKEN` | Zalo OA | Làm mới access token OA, restart. |
| `GOSO_ZALO_PERSONAL_TOKEN` | Zalo Personal | Làm mới token, restart. |

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
- Restore từ `backups/`.

### Telegram/Zalo webhook 200 nhưng không nhắn ra channel

- Thiếu bot/OA token: webhook vẫn 200 (tránh retry storm) kèm `warning` trong body. Set token rồi restart.
- Payload rỗng / không có text → 200 no-op.

### `make verify` đỏ

- `gofmt` — `make fmt-fix`
- test fail — `go test ./... -count=1`
- gitleaks — secret trong working tree, xoá/rotate, không commit
- semgrep — xem rule id, sửa source
- e2e — đọc log script; cần `curl` + `python3`; cổng local tự chọn (`--port 0`)

## 5. Kiểm tra nhanh sau sự cố

```bash
curl -sS http://127.0.0.1:8080/healthz
curl -sS -H "Authorization: Bearer $GOSO_ADMIN_TOKEN" http://127.0.0.1:8080/api/agents
make verify
```
