# SPEC 008 — Observability (Logging, Tracing, Metrics)

> LOCKED: 2026-08-20 — Nhìn thấy hệ thống: log có cấu trúc, trace LLM, metrics tối thiểu.

## Goal

Gateway có thể quan sát: log JSON có `request_id`/`trace_id`, trace LLM (latency, token nếu có), và endpoint metrics tối thiểu (`/metrics` text hoặc `/api/stats`). Không phụ thuộc Prometheus/Jaeger bắt buộc.

## User stories

- **US-01** Mỗi request có `X-Request-ID` (tự sinh nếu thiếu), log một dòng JSON với method/path/status/latency.
- **US-02** Mỗi lần gọi LLM ghi trace (provider/model/latency/error) vào log và có thể truy vấn `GET /api/traces?limit=20`.
- **US-03** `GET /metrics` hoặc `GET /api/stats` trả số liệu tối thiểu (uptime, request count, LLM call count).

## Acceptance criteria

- [x] AC-01 Middleware request ID + structured JSON log (không lộ token/key)
- [x] AC-02 LLM trace: lưu in-memory ring buffer (N=200), `GET /api/traces`
- [x] AC-03 `GET /api/stats` hoặc `/metrics` — uptime + counters
- [x] AC-04 Unit test cho middleware + trace buffer
- [x] AC-05 `make verify` xanh

## Non-goals

- OpenTelemetry/Jaeger, Prometheus exporter đầy đủ — để sau (overlay)
- Alerting, dashboard Grafana — để sau
- Billing tính tiền từ trace — SPEC 010

## Ghi chú

- Log format: `{"level":"info","request_id":"...","method":"GET","path":"/api/agents","status":200,"latency_ms":3}`
- Trace lưu in-memory, không cần DB ở SPEC này.
