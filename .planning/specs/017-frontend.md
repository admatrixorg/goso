# SPEC 017 — Control-plane DEMO gate

> LOCKED: 2026-08-26. Pair of goso-crm SPEC 017 (Go UI restyle).

## Flag

`VITE_DEMO_MODE=true|1` → hiện tab DEMO (Home, Việc, Họp, Bạn bè, Lịch, Kho ảnh, Marketing) + badge.
Unset hoặc `false` → **ẩn** các tab đó; default tab = Tổng quan (CRM metrics). Settings / Agent / Phiên / Chat / Kết nối / Nhật ký giữ.

Không xây 13 settings / 7 marketing / heatmap.

## AC

- [ ] Build hai mode. Preview/curl-or-grep JS: demo có `DEMO` fixtures; non-demo không có nav labels mock trong bundle (or hidden).
- [ ] `docs/qa/017-demo-gate.md`.
