# SPEC 004 — Zalo Personal + Zalo OA Channels

> LOCKED: 2026-08-20 — Hoàn thiện MVP 4 channel: Telegram (003) + WebSocket (002) + Zalo Personal + Zalo OA.

## Goal

Thêm 2 Zalo adapter vào `gateway/internal/channel`: Zalo Personal (ca nhân, reverse-engineered) và Zalo OA (Official Account webhook theo tài liệu Zalo OA). Cả hai dùng chung Channel interface với Telegram, gọi cùng LLM provider. Xong SPEC này, gateway đủ 4 channel MVP để demo đầu-cuối.

## User stories

- **US-01** Operator cấu hình `GOSO_ZALO_OA_ACCESS_TOKEN` + `GOSO_ZALO_OA_SECRET`, Zalo OA gửi webhook `POST /api/channels/zalo-oa/webhook` với `message.text` → GOSO tạo session `zalo-oa:<user_id>`, gọi LLM, trả lời qua OA send API (stub HTTP trong test).
- **US-02** Operator cấu hình Zalo Personal (cookie/token), Zalo Personal webhook `POST /api/channels/zalo-personal/webhook` → session `zalo-personal:<thread_id>`, gọi LLM và trả lời.
- **US-03** `GET /api/channels` trả `["telegram","zalo-personal","zalo-oa"]` khi cả 3 đã wire.

## Acceptance criteria

- [ ] AC-01 `internal/channel/zalo_oa.go` — `ZaloOA` struct, `HandleUpdate`, parse payload Zalo OA (user_id + message.text), session `zalo-oa:<user_id>`, gọi `llm.Provider`, `Sender` injectable, test mock
- [ ] AC-02 `internal/channel/zalo_personal.go` — `ZaloPersonal` tương tự, session `zalo-personal:<thread_id>`
- [ ] AC-03 `GET /api/channels` trả đủ 3 channel khi wire (telegram + zalopersonal + zalo oa)
- [ ] AC-04 `make verify` xanh, unit test cho cả 2 adapter + e2e webhook
- [ ] AC-05 Không copy code ZaloCRM hiện có; viết clean-room; mọi file có header GOSO

## Non-goals

- Đăng nhập Zalo Personal thật (QR/cookie refresh), polling loop — stub webhook, để SPEC riêng
- Rich message (image, sticker, button), template OA — chỉ text
- Persist token vào DB — env/SPEC 00X
- Discord — để sau SPEC 004

## Ghi chú

- Payload Zalo OA tham khảo `zalocrm-mcp` và doc Zalo: `{event_name:"user_send_text", sender:{id}, message:{text}}` — chấp nhận cả format rút gọn `{user_id, message:{text}}` để linh hoạt test.
- Payload Zalo Personal: `{thread_id, message:{text}, from_id}` — đơn giản, không ràng buộc API chính thức.
