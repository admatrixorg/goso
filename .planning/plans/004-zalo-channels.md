# PLAN 004 — Zalo Personal + Zalo OA

> SPEC: `specs/004-zalo-channels.md` (LOCKED 2026-08-20)

| # | Task | File | Verify |
|---|------|------|--------|
| T01 | Zalo OA adapter + test | `gateway/internal/channel/zalo_oa.go`, `zalo_oa_test.go` | `go test ./internal/channel -count=1 -run ZaloOA` |
| T02 | Zalo Personal adapter + test | `gateway/internal/channel/zalo_personal.go`, `zalo_personal_test.go` | `go test ./internal/channel -count=1 -run ZaloPersonal` |
| T03 | Wire 2 channel vào router + main | `gateway/internal/httpapi/handlers.go`, `gateway/cmd/goso-gateway/main.go` | `go vet ./...` |
| T04 | QA AC 01–05 | `make verify` + smoke | checklist |

## Trạng thái

- [x] T01 — Zalo OA
- [x] T02 — Zalo Personal
- [x] T03 — wire
- [x] T04 — QA

## QA 2026-08-20
| AC | Kết quả | Bằng chứng |
| AC-01 | ✅ | zalo_oa.go HandleUpdate user_id/sender.id + session zalo-oa:<id>, Sender mock |
| AC-02 | ✅ | zalo_personal.go thread_id/from_id + session zalo-personal:<id> |
| AC-03 | ✅ | GET /api/channels → [telegram,zalo-personal,zalo-oa] |
| AC-04 | ✅ | go test channel (6 case) + httpapi + llm + store xanh |
| AC-05 | ✅ | clean-room, không copy ZaloCRM, header GOSO |
| Smoke | ✅ | healthz/chat + 3 webhook tạo 3 session đúng label |
