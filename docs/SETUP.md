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
```

Chi tiết overlay production: `docs/DEPLOY.md`.

## Biến môi trường

| Biến | Mặc định | Mô tả |
|------|----------|-------|
| GOSO_PORT | 8080 | Cổng gateway |
| GOSO_HOST | 127.0.0.1 | Bind host (Docker: `0.0.0.0`) |
| GOSO_LOG_LEVEL | info | Mức log |
| GOSO_ADMIN_TOKEN | (rỗng → 401) | Bearer `/api/*` và `/ws`. Passthrough chỉ khi `GOSO_DEV_MODE=1`. |
| GOSO_VIEW_TOKEN | (rỗng) | Optional GET-only Bearer for `/healthz` `/api/agents` `/api/sessions`. POST `/api/chat` → 403. |
| GOSO_INJECTION | log | `log` (default) or `block`. Scan user chat for documented injection patterns; block → 400 on `/api/chat`. |
| GOSO_SSRF | off | `1` blocks literal localhost/private IPs on connector HTTP. Default off so local fake e2e works. |
| GOSO_WORKSPACE | (rỗng) | Optional write jail. Tools/vault cannot write outside this directory. |
| GOSO_MASTER_KEY | (rỗng) | 32-byte hex AES-256-GCM key for `secrets(name, nonce, ct)`. Empty → refuse persist; env provider keys still work. |
| GOSO_RATE_LIMIT | 60 | Giới hạn req/phút/IP (0 = tắt) |
| GOSO_DB_PATH | :memory: (gateway) / OS app-support (desktop) | File SQLite (vd data/goso.db; Docker: `/data/goso.db`) |
| GOSO_VAULT_DIR | `data/vault` | Thư mục markdown/text knowledge vault (`*.md` `*.txt`). Optional `TEAM.md` is prepended to the team system note (SPEC 038). |
| GOSO_LITE | (off) | `1` caps **5 agents** and **1 team** (SPEC 038). 6th `POST /api/agents` and 2nd team → 400. |
| GOSO_ANTHROPIC_API_KEY / GOSO_OPENAI_API_KEY | (empty) | Native LLM keys. Empty → provider absent; `echo` always remains. |
| GOSO_OPENROUTER_API_KEY, GOSO_GROQ_API_KEY, GOSO_DEEPSEEK_API_KEY, GOSO_GEMINI_API_KEY, GOSO_MISTRAL_API_KEY, GOSO_XAI_API_KEY, GOSO_MINIMAX_API_KEY, GOSO_DASHSCOPE_API_KEY | (empty) | Named OpenAI-compat providers (SPEC 039). Empty = absent. `GET /api/providers` lists configured names only. |
| GOSO_TELEGRAM_BOT_TOKEN, GOSO_ZALO_OA_ACCESS_TOKEN, GOSO_ZALO_PERSONAL_TOKEN, GOSO_DISCORD_BOT_TOKEN, GOSO_SLACK_BOT_TOKEN, GOSO_FEISHU_APP_SECRET, GOSO_WHATSAPP_ACCESS_TOKEN | (empty) | Channel tokens (SPEC 040). Empty = still listed on `GET /api/channels` with `configured: false`. Live values = DI-01..07, not in git. WhatsApp adapter is Cloud API shaped (native vs Business = DI-01). |
| GOSO_WS_ORIGINS | (empty = allow all) | Comma-separated WebSocket Origin allowlist. Empty keeps previous allow-all behaviour. |
| GOSO_ENV | development | Môi trường |
| GOSO_GATEWAY_URL | — | URL gateway cho `goso-mcp` (bắt buộc khi chạy MCP) |
| GOSO_TOKEN | (rỗng) | Bearer token MCP → gateway |
| GOSO_MCP_PORT | 3100 | Cổng Streamable HTTP của MCP |
| GOSO_OTEL_ENDPOINT | (rỗng = noop) | Optional OTLP HTTP JSON collector URL (SPEC 042). Empty = no export. Do **not** put Grafana Cloud keys here (DI-18). |

Xem thêm: `docs/RUNBOOK.md` (vận hành), `docs/RELEASE.md` (phát hành), `.env.example` (mẫu env).
