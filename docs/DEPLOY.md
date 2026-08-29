# DEPLOY — GOSO (Docker + Compose)

Đóng gói gateway (Go 1.25, multi-stage, không CGO) và control-plane (Node 22). SQLite nằm trên volume `data`.

## Yêu cầu

- Docker Engine 24+
- Docker Compose v2 (`docker compose version`)

## Local — `docker compose up`

```bash
cp .env.example .env   # tùy chọn, điền token/LLM
docker compose up --build
```

| Dịch vụ | URL |
|---------|-----|
| Gateway | http://localhost:8080 (`GET /healthz`) |
| Control Plane | http://localhost:3000 (proxy `/api`, `/healthz`, `/ws` → gateway) |

SQLite: volume Docker tên `data` (file `/data/goso.db` trong container gateway).

Smoke:

```bash
curl -s http://localhost:8080/healthz
# {"ok":true,"version":"0.1.0"}
curl -s http://localhost:3000/healthz
# cùng JSON, qua proxy control-plane
```

Dừng: `Ctrl-C` hoặc `docker compose down`. Giữ volume: không thêm `-v`.

## Production overlay

```bash
# Bắt buộc đặt GOSO_ADMIN_TOKEN trên môi trường production
export GOSO_ADMIN_TOKEN=...          # Bearer cho /api/* và /ws
docker compose -f compose.yml -f compose.prod.yml up -d --build
```

Overlay `compose.prod.yml` thêm:

- `restart: unless-stopped` cho gateway + control-plane
- `GOSO_ENV=production`
- log rotation (`json-file`, 10m × 5)
- sidecar `backup`: gọi gateway `POST /api/system/backup` (`VACUUM INTO` + `PRAGMA integrity_check`) mỗi `BACKUP_INTERVAL_SECONDS` (mặc định 3600s) vào volume `backup`, giữ `BACKUP_RETAIN` bản (mặc định 14, tối thiểu 1). Không `cp` file đang chạy.

Token rỗng = 401 trên `/api/*` trừ khi `GOSO_DEV_MODE=1`. Production phải set `GOSO_ADMIN_TOKEN`.

## Build riêng

```bash
docker compose build                 # cả hai image
docker build -t goso-gateway .
docker build -t goso-control-plane ./control-plane
```

## Overlay ý tưởng (tối giản so với GoClaw)

GoClaw gốc có 8 overlay. GOSO chỉ ship **core + prod** ở SPEC 012; optional Postgres / Jaeger are compose profiles, not extra overlay files.

| Overlay GoClaw | GOSO |
|----------------|------|
| `docker-compose.yml` (core) | `compose.yml` — gateway + control-plane + volume `data` |
| `docker-compose.postgres.yml` | Profile `postgres` in `compose.yml` (SPEC 085) — `pgvector/pgvector:pg16` host **5433**; default `up` stays SQLite |
| selfservice (UI :3000) | Gộp vào `compose.yml` (control-plane :3000) |
| redis | Chưa — rate-limit in-memory (SPEC 006) |
| otel / jaeger | Profile `otel` in `compose.yml` (SPEC 087) — Jaeger all-in-one OTLP HTTP **4318** / UI **16686**; default `up` stays noop. No Grafana Cloud keys (DI-18). |
| tailscale | Ngoài scope |
| sandbox | Opt-in `GOSO_SANDBOX_IMAGE` + `docker` on PATH (SPEC 086). No compose overlay. Missing → `not_configured`. |
| browser | Opt-in `GOSO_BROWSER_BIN` / `CHROME_PATH` existing file (SPEC 086). No Chrome service. Missing → `not_configured`. |
| *(prod)* | `compose.prod.yml` — restart, auth, backup SQLite |

Khi thêm overlay sau này, dùng cùng pattern `-f compose.yml -f compose.<name>.yml`.

## Biến môi trường

Xem `.env.example` và `docs/SETUP.md`. Trong Docker:

| Biến | Mặc định (compose) | Ghi chú |
|------|--------------------|---------|
| `GOSO_HOST` | `0.0.0.0` | Bind mọi interface trong container |
| `GOSO_PORT` | `8080` | Cổng gateway |
| `GOSO_DB_PATH` | `/data/goso.db` | File trên volume `data` |
| `GOSO_DATABASE_URL` | rỗng | Optional pgx DSN (SPEC 085). Empty = SQLite. Profile `postgres` is host **5433**; in-network `postgres:5432`. Connect fail = no sqlite fallback. |
| `GOSO_ENV` | `development` / overlay: `production` | |
| `GOSO_ADMIN_TOKEN` | rỗng | Rỗng = dev mode |
| `GATEWAY_URL` | `http://gateway:8080` | Control-plane proxy (nội bộ Docker network) |
| `BACKUP_INTERVAL_SECONDS` | `3600` | Chỉ overlay prod |
| `GOSO_OTEL_ENDPOINT` | (unset) | Optional OTLP HTTP JSON URL. Unset = noop. Profile `otel`: compose-network `http://jaeger:4318/v1/traces`; host `http://127.0.0.1:4318/v1/traces`. No Grafana Cloud keys (DI-18). |

## Không nằm trong SPEC này

- Kubernetes / Helm
- CI tự deploy (CI hiện chỉ `make verify`)
- TLS/HTTPS reverse proxy (đặt nginx/caddy trước compose khi cần)

## Sự cố thường gặp

- **Control-plane 502 gateway unreachable** — đợi healthcheck gateway (SQLite init). `docker compose ps` phải `healthy`.
- **Permission `/data/goso.db`** — image chạy user `goso` (uid 1000); volume mới kế thừa quyền từ image.
- **Cổng đã chiếm** — đổi mapping `8080:8080` / `3000:3000` trong `compose.yml`.
- **`curl 127.0.0.1:8080` ra UI lạ (không phải GOSO)** — một process khác đang listen `127.0.0.1:8080` (bind cụ thể thắng Docker `*:8080` trên macOS). Kiểm tra `lsof -nP -iTCP:8080 -sTCP:LISTEN`. Gateway trong container vẫn healthy (`docker exec goso-gateway-1 wget -qO- http://127.0.0.1:8080/healthz`); Control Plane `:3000/healthz` proxy cùng JSON.
