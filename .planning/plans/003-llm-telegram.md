# PLAN 003 — LLM Provider + Telegram

> SPEC: `specs/003-llm-telegram.md` (LOCKED 2026-08-20)

| # | Task | File | Verify |
|---|------|------|--------|
| T01 | LLM interface + Anthropic/OpenAI impl + echo fallback | `gateway/internal/llm/provider.go`, `anthropic.go`, `openai.go`, `provider_test.go` | `go test ./internal/llm -count=1` |
| T02 | Provider registry (env) + GET /api/providers | `gateway/internal/llm/registry.go`, handler | `curl /api/providers` |
| T03 | Cập nhật POST /api/chat dùng LLM | `gateway/internal/httpapi/handlers.go` | `go test ./internal/httpapi -count=1` |
| T04 | Telegram Channel adapter (webhook) | `gateway/internal/channel/telegram.go`, `channel_test.go` | `go test ./internal/channel -count=1` |
| T05 | GET /api/channels + wire router | `gateway/internal/httpapi/handlers.go`, `main.go` | `go vet ./...` |
| T06 | QA AC 01–07 | `make verify` + smoke | checklist |

## Trạng thái

- [x] T01 — llm
- [x] T02 — registry/providers
- [x] T03 — chat via llm
- [x] T04 — telegram
- [x] T05 — wire
- [x] T06 — QA

## QA 2026-08-20
| AC | Kết quả | Bằng chứng |
| AC-01 | ✅ | llm/provider.go + anthropic/openai + echo, mock test xanh |
| AC-02 | ✅ | POST /api/chat gọi LLM (echo fallback khi không key), lưu history |
| AC-03 | ✅ | GET /api/providers → [echo]/[anthropic] (không lộ key) |
| AC-04 | ✅ | POST /api/channels/telegram/webhook → session telegram:chat_id, gọi LLM, Sender mock |
| AC-05 | ✅ | GET /api/channels → [telegram] |
| AC-06 | ✅ | go test: llm (anthropic/openai mock) + channel + httpapi xanh |
| AC-07 | ✅ | make verify + gofmt xanh, header clean-room |
| Smoke | ✅ | healthz/providers/channels/chat/telegram webhook OK |
| WS | ✅ | SPEC 002 WS echo vẫn xanh (101 Switching Protocols) |
