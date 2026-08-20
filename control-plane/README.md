# GOSO Control Plane

Vite + React + TypeScript — quản trị GOSO Gateway.

## Chạy dev

```bash
npm install
npm run dev
# http://localhost:3000 — proxy /api/* và /healthz tới http://127.0.0.1:8080 (gateway)
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
