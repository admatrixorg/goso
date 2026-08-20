# PLAN 002 — Gateway HTTP + Session Core

> SPEC: `specs/002-gateway-http-session.md` (LOCKED 2026-08-20)

| # | Task | File | Verify | Song song |
|---|------|------|--------|-----------|
| T01 | Domain model + Store in-memory | `gateway/internal/store/store.go`, `store_test.go` | `go test ./internal/store -count=1` | — |
| T02 | HTTP handlers (agents/sessions/messages/chat/healthz) | `gateway/internal/httpapi/handlers.go` | `go vet ./...` | sau T01 |
| T03 | WebSocket echo | `gateway/internal/httpapi/ws.go` | `go vet ./...` | ∥ T02 |
| T04 | Wire `gateway` command (flag --port, graceful shutdown) | `gateway/cmd/goso-gateway/main.go` | `go run ./gateway/cmd/goso-gateway gateway --port 0` | sau T02,T03 |
| T05 | QA verify AC 01–08 | `make verify`, curl smoke | checklist | cuối |

## Rationale

- `net/http` stdlib: không dependency, đủ cho MVP; tránh Gin/Echo.
- `gorilla/websocket`: phổ biến, nhẹ, không kéo cả framework.
- Store interface: sau này DB chỉ cần implement interface, không sửa handler.

## Trạng thái

- [x] T01 — store
- [x] T02 — handlers
- [x] T03 — websocket
- [x] T04 — wire gateway
- [x] T05 — QA

## QA 2026-08-20
| AC | Kết quả | Bằng chứng |
| AC-01 | ✅ | `gateway --port 0` → /healthz 200 {ok:true,version:0.1.0} |
| AC-02 | ✅ | POST/GET /api/agents ok, validate agent_key |
| AC-03 | ✅ | POST/GET /api/sessions ok |
| AC-04 | ✅ | POST /api/sessions/:id/messages ok |
| AC-05 | ✅ | POST /api/chat echo: ... |
| AC-06 | ✅ | GET /ws echo (101 Switching Protocols, raw frame OK) |
| AC-07 | ✅ | make verify vet+test xanh (store+httpapi) |
| AC-08 | ✅ | header Copyright MQ Global, clean-room |
