# SPEC 007 — Control Plane (Admin API + Dashboard Stub)

> LOCKED: 2026-08-20 — Lớp quản trị GOSO theo lộ trình C→B: Admin API + Dashboard stub, gọi gateway.

## Goal

Dựng **Control Plane** tối thiểu để quản trị gateway không cần curl thủ công: Admin API (proxy tới gateway) + Dashboard web stub (hiển thị agents/sessions/messages, gửi chat). Control Plane chạy riêng (port khác), đọc `GOSO_GATEWAY_URL` + `GOSO_ADMIN_TOKEN` để gọi gateway.

## User stories

- **US-01** Operator chạy `npm run dev` trong `control-plane/` → Dashboard mở ở `http://localhost:3000`, thấy list Agents, tạo Agent, thấy Sessions/Messages.
- **US-02** Từ Dashboard gửi `chat` tới session → thấy reply echo/LLM.
- **US-03** `GET /api/health` của Control Plane trả gateway status.

## Acceptance criteria

- [ ] AC-01 `control-plane/` — Vite + React + TypeScript, `src/api/client.ts` gọi gateway (`GOSO_GATEWAY_URL`), proxy qua `/api/*` của gateway (Bearer token)
- [ ] AC-02 Pages: Agents list + create, Sessions list, Messages list, Chat box (text)
- [ ] AC-03 `control-plane/src/server.ts` (hoặc Vite proxy) — optional Node server cho production build, không bắt buộc cho dev
- [ ] AC-04 `pnpm -C control-plane verify` hoặc `npm run verify` xanh (typecheck + lint + build)
- [ ] AC-05 Không lộ token ra client bundle (token chỉ ở gateway, dashboard gọi qua Control Plane proxy hoặc gateway trực tiếp với token từ env)
- [ ] AC-06 `make verify` ở repo root vẫn xanh (control-plane không break gateway verify)

## Non-goals

- Auth UI, RBAC dashboard — để sau
- Billing/quota — SPEC riêng
- WebSocket realtime dashboard — stub poll
- Wails Desktop — track riêng

## Ghi chú

- Stack: Vite 5 + React 18 + TypeScript 5, giữ đơn giản, không UI framework nặng (Tailwind optional, chưa cần).
- Gateway vẫn là nguồn chân lý; Control Plane chỉ là view + proxy.
