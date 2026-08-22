# GOSO Control Plane

Vite + React + TypeScript — quản trị GOSO Gateway.

## Chạy dev

```bash
npm install
npm run dev
# http://localhost:3000 — proxy /api/* và /healthz tới http://127.0.0.1:8080 (gateway)
# proxy /crm-api/* tới http://127.0.0.1:8089 (goso-crm) — tránh CORS khi dev
```

Gateway phải chạy trước:

```bash
# từ repo root
go run ./gateway/cmd/goso-gateway gateway --port 8080
# hoặc: GOSO_ADMIN_TOKEN=secret go run ./gateway/cmd/goso-gateway gateway
```

Nếu gateway có token:

```bash
VITE_GOSO_ADMIN_TOKEN=secret npm run dev
# hoặc trong browser: localStorage.setItem("goso_token", "secret")
```

## Build

```bash
npm run build   # -> dist/
npm run preview
```

## Trang

- **Agents** — list + tạo agent
- **Sessions** — list, chọn session
- **Chat** — xem message + gửi chat tới session
- **Connectors** — đăng ký connector + gán agent
- **Events** — EventStore (attempt/success/error/approval)
- **CRM metrics** — KPI live từ goso-crm HTTP (`GET /api/crm/metrics`, `GET /api/crm/advisor`)

## CRM metrics (goso-crm HTTP)

Control Plane **không** import Go của goso-crm. KPI lấy qua HTTP.

| Biến | Mặc định | Mô tả |
|------|----------|-------|
| `VITE_GOSOCRM_API_URL` | `http://127.0.0.1:8089` (upstream) | Base URL goso-crm. Để trống khi `npm run dev` → dùng Vite proxy `/crm-api`. Có thể set URL đầy đủ hoặc `/crm-api`. |
| `VITE_GOSOCRM_ORG_ID` | `01a01fe5-704c-7375-aa1f-6e50a9d0296d` (test-a) | Gửi header `X-Org-ID` trên fetch metrics/advisor. |

Mọi CRM fetch gửi `X-Org-ID`. **Không** nhúng secret trong UI, source, hay hiển thị network.

### Vite proxy `/crm-api` (dev, tránh CORS)

`vite.config.ts` map `/crm-api` → `http://127.0.0.1:8089`:

```
/crm-api/healthz             → http://127.0.0.1:8089/healthz
/crm-api/readyz              → http://127.0.0.1:8089/readyz
/crm-api/api/crm/metrics     → http://127.0.0.1:8089/api/crm/metrics
/crm-api/api/crm/advisor     → http://127.0.0.1:8089/api/crm/advisor
```

```bash
# khuyến nghị khi dev (same-origin, không CORS)
npm run dev
# hoặc tường minh:
VITE_GOSOCRM_API_URL=/crm-api npm run dev

# gọi thẳng goso-crm (trình duyệt có thể chặn CORS nếu CRM không mở origin)
VITE_GOSOCRM_API_URL=http://127.0.0.1:8089 npm run dev
```

Trạng thái **goso-crm online / offline**: `GET {base}/healthz` hoặc `/readyz`. Lỗi mạng, non-200, hoặc timeout ~3s → offline (không treo UI). Metrics = 0 / rỗng là hợp lệ, không phải lỗi.
