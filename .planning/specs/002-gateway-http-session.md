# SPEC 002 — Gateway HTTP + Session Core

> LOCKED: 2026-08-20 — Gateway HTTP chạy được + Session in-memory, nền cho 4 channel MVP.

## Goal

Biến `goso-gateway` từ skeleton thành **HTTP server chạy được**: `goso-gateway gateway` khởi động, phục vụ `GET /healthz`, CRUD Agent/Session/Message in-memory, `POST /api/chat` echo, và `GET /ws` WebSocket echo. Không DB, không LLM thật, không channel — chỉ lõi để SPEC 003 gắn channel/LLM lên.

## User stories

- **US-01** Operator chạy `goso-gateway gateway --port 8090` → server lắng nghe, `curl /healthz` trả `{"ok":true}`.
- **US-02** Client tạo Agent (`POST /api/agents`), list (`GET /api/agents`), tạo Session (`POST /api/sessions`), gửi Message (`POST /api/sessions/:id/messages`), list messages.
- **US-03** Client `POST /api/chat {session_id, message}` → trả echo `{"reply":"echo: <message>"}` (stub LLM).
- **US-04** Client kết nối `ws://localhost:8090/ws?session_id=xxx` → gửi text → nhận echo.

## Acceptance criteria

- [ ] AC-01 `goso-gateway gateway --port 0` (random port) khởi động, `GET /healthz` → 200 `{"ok":true,"version":"0.1.0"}`
- [ ] AC-02 `POST /api/agents` + `GET /api/agents` + `GET /api/agents/:id` hoạt động (in-memory, validate `agent_key` required)
- [ ] AC-03 `POST /api/sessions` (cần `agent_id`) + `GET /api/sessions` + `GET /api/sessions/:id/messages` hoạt động
- [ ] AC-04 `POST /api/sessions/:id/messages` lưu message user
- [ ] AC-05 `POST /api/chat` nhận `{session_id, message}` → trả `{"reply":"echo: ...","session_id":...}`
- [ ] AC-06 `GET /ws?session_id=xxx` WebSocket echo (dùng `nhooyr/websocket` hoặc `gorilla/websocket`)
- [ ] AC-07 `make verify` (vet+fmt+test) xanh, có unit test cho store + handler
- [ ] AC-08 Không copy code GoClaw; mọi file có header license GOSO

## Non-goals

- DB (Postgres/SQLite), LLM provider thật, channel adapter (Telegram/Zalo), auth/JWT, billing — để SPEC 003+
- Control Plane TS, Desktop Wails — track riêng
- MCP — tận dụng `goclaw-mcp` hiện có

## Ghi chú kỹ thuật

- Store: `internal/store` in-memory (map+mutex), interface để sau thay bằng DB.
- HTTP: `net/http` stdlib + `ServeMux`, không framework.
- WS: `gorilla/websocket` (nhẹ, phổ biến) hoặc `nhooyr.io/websocket`.
