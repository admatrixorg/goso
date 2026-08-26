# SPEC 016 — Gateway: đóng default-open (auth bắt buộc)

> LOCKED: 2026-08-26. Nửa goso của SPEC 016 (canonical CRM: `goso-crm/.planning/specs/016-auth-security.md`).
> Sửa SPEC 006: token rỗng không còn là dev-mode mặc định.

## Goal

`GOSO_ADMIN_TOKEN` rỗng → `/api/*` và `/ws` trả **401**, trừ khi `GOSO_DEV_MODE=1` (tường minh). `/healthz` bypass. Rate limit giữ.

## Acceptance criteria

- [ ] AC-01 Empty token, no `GOSO_DEV_MODE` → 401 JSON trên `/api/agents`.
- [ ] AC-02 Bearer đúng → đi tiếp (201/200 như cũ).
- [ ] AC-03 `GOSO_DEV_MODE=1` + token rỗng → passthrough (desktop/local).
- [ ] AC-04 `/healthz` 200 không token.
- [ ] AC-05 `make verify` xanh. Docs RUNBOOK / `.env.example` không còn “rỗng = dev mode mặc định”.
