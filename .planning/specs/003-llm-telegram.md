# SPEC 003 — LLM Provider + Telegram Channel

> LOCKED: 2026-08-20 — Gắn LLM thật (Anthropic/OpenAI) + Telegram webhook/polling, thay echo bằng LLM.

## Goal

`POST /api/chat` gọi LLM thật qua Provider đã cấu hình; Telegram adapter nhận message từ Telegram (webhook stub + polling skeleton) và trả lời qua LLM. Các channel sau (Zalo Personal/OA, Discord) chỉ cần implement cùng Channel interface.

## User stories

- **US-01** Operator cấu hình `GOSO_ANTHROPIC_API_KEY` / `GOSO_OPENAI_API_KEY`, tạo Agent với `provider=anthropic + model=claude-sonnet-4`, gọi `POST /api/chat` → trả lời từ LLM (không còn echo).
- **US-02** Operator gắn Telegram bot token (`GOSO_TELEGRAM_BOT_TOKEN`), đặt webhook `POST /api/channels/telegram/webhook` → gửi tin nhắn cho bot → bot trả lời bằng LLM.
- **US-03** `GET /api/providers` list provider đã cấu hình, `GET /api/channels` list channel.

## Acceptance criteria

- [ ] AC-01 `internal/llm` — Provider interface (`Chat(ctx, messages) -> reply`), impl `anthropic` + `openai` (HTTP trực tiếp, không SDK nặng), stub `echo` khi không có key
- [ ] AC-02 `POST /api/chat` dùng LLM: lấy Session → Agent → Provider → gọi Chat, lưu cả user+assistant message
- [ ] AC-03 `GET /api/providers` trả provider đã cấu hình (không lộ key)
- [ ] AC-04 `POST /api/channels/telegram/webhook` nhận Telegram Update JSON, trích `message.text`, tạo/tiếp Session theo `chat_id`, gọi LLM, trả `sendMessage` (stub HTTP, không gọi Telegram thật trong test)
- [ ] AC-05 `GET /api/channels` list channel (telegram)
- [ ] AC-06 Unit test cho llm (mock HTTP) + channel telegram (mock) + e2e chat
- [ ] AC-07 `make verify` xanh, không copy GoClaw

## Non-goals

- Zalo Personal / Zalo OA / Discord — SPEC 004+
- Streaming SSE, tool-calling, image — để sau
- DB persist Provider/Channel — config qua env, SPEC 004 mới persist
- Polling loop Telegram long-polling — skeleton, webhook là chính

## Ghi chú

- Provider config qua env: `GOSO_ANTHROPIC_API_KEY`, `GOSO_OPENAI_API_KEY`, `GOSO_TELEGRAM_BOT_TOKEN`.
- Channel interface: `HandleUpdate(w,r)` + `Name()`.
- LLM HTTP: dùng `net/http` stdlib, timeout 30s.
