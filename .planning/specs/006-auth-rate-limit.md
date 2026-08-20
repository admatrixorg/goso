# SPEC 006 — Auth (Admin Token) + Rate Limit

> LOCKED: 2026-08-20 — Bảo vệ gateway bằng admin token + rate limit đơn giản.

## Goal

Thêm lớp bảo vệ tối thiểu trước khi mở gateway ra ngoài: **admin token** cho mọi `/api/*` và `WebSocket`, và **rate limit** theo IP.

## User stories

- **US-01** Operator đặt `GOSO_ADMIN_TOKEN=secret123`, mọi `GET/POST /api/*` không có `Authorization: Bearer secret123` → 401. Có header đúng → cho qua.
- **US-02** `GET /healthz` luôn cho qua (không cần auth) để probe.
- **US-03** `GET /ws` yêu cầu token (query `?token=secret123` hoặc header `Authorization`).
- **US-04** Rate limit: 60 request/phút/IP cho `/api/*` → 429 khi vượt.

## Acceptance criteria

- [ ] AC-01 `gateway/internal/auth` — middleware `RequireToken(token)` kiểm tra `Authorization: Bearer <token>` hoặc `?token=`, trả 401 JSON nếu sai; bypass `/healthz`
- [ ] AC-02 `gateway/internal/ratelimit` — token bucket / sliding window đơn giản (map+mutex, 60/min/IP), trả 429 + `Retry-After`
- [ ] AC-03 Wire vào `gateway/cmd/goso-gateway`: đọc `GOSO_ADMIN_TOKEN`, nếu rỗng → cảnh báo và cho qua (dev mode), nếu có → gắn middleware cho `/api` và `/ws`
- [ ] AC-04 Unit test cho auth + ratelimit + e2e 401/429
- [ ] AC-05 `make verify` xanh; `GET /healthz` vẫn 200 không cần token
- [ ] AC-06 Không lộ token vào log/response

## Non-goals

- JWT/OAuth, RBAC, multi-user — để SPEC riêng
- Redis-backed rate limit — in-memory đủ MVP
- TLS — để deployment

## Ghi chú

- Token lấy từ `GOSO_ADMIN_TOKEN` env. Khi rỗng, gateway ở dev mode (log cảnh báo, không chặn).
- Rate limit config: `GOSO_RATE_LIMIT=60` (req/min/IP), 0 = tắt.
