# SPEC 010 — Billing & Quota (Token Metering Stub)

> LOCKED: 2026-08-20 — Đếm token/chi phí tối thiểu, làm nền cho thu phí.

## Goal

Có **metering stub**: đếm token ước tính (hoặc thực tế nếu provider trả về) cho mỗi LLM call, lưu vào trace, và `GET /api/usage` tổng hợp theo agent/ngày. Chưa tính tiền thật, chỉ metering.

## User stories

- **US-01** Mỗi LLM call ghi `prompt_tokens`/`completion_tokens` (ước tính nếu provider không trả) vào trace.
- **US-02** `GET /api/usage?agent_id=xxx&from=YYYY-MM-DD` trả tổng token và số call.
- **US-03** `GET /api/usage` có thể lọc theo `provider`.

## Acceptance criteria

- [ ] AC-01 Token estimator (rè `len(text)/4` hoặc word count) khi provider không trả token count
- [ ] AC-02 Lưu usage vào store (SQLite) hoặc trace buffer
- [ ] AC-03 `GET /api/usage` — query params `agent_id`, `from`, `to`, `provider`
- [ ] AC-04 Unit test cho estimator + usage query
- [ ] AC-05 `make verify` xanh

## Non-goals

- Tính tiền VND, Stripe, quota chặn request — SPEC riêng
- Giá model thực tế (cập nhật bảng giá) — có thể hardcode stub

## Ghi chú

- Provider Anthropic/OpenAI có thể trả usage; nếu không, dùng ước tính để demo.
- Lưu usage cùng trace hoặc bảng `usage` riêng — chọn đơn giản nhất (trace buffer + SQLite nếu cần).
