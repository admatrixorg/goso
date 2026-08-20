# PLAN 005 — DB Persist (SQLite)

> SPEC: `specs/005-db-persist.md` (LOCKED 2026-08-20)

| # | Task | File | Verify |
|---|------|------|--------|
| T01 | SQLiteStore + migration | `gateway/internal/store/sqlite.go`, `sqlite_test.go` | `go test ./internal/store -count=1 -run SQLite` |
| T02 | Store selector (env GOSO_DB_PATH) | `gateway/internal/store/store.go` (Open), `gateway/cmd/goso-gateway/main.go` | `GOSO_DB_PATH=:memory: go test -run Selector` |
| T03 | Wire vào handlers (không đổi interface) | `gateway/internal/httpapi/*` (không đổi) | `go vet ./...` |
| T04 | QA AC 01–07 | `make verify` + restart smoke | checklist |

## Trạng thái

- [x] T01 — SQLiteStore
- [x] T02 — selector
- [x] T03 — wire
- [x] T04 — QA

## QA 2026-08-20
| AC | Kết quả | Bằng chứng |
| AC-01 | ✅ | sqlite.go SQLiteStore + modernc.org/sqlite, CRUD ok |
| AC-02 | ✅ | migrate tạo 3 bảng + index |
| AC-03 | ✅ | GOSO_DB_PATH=:memory: → memory, file → sqlite, log store: ... |
| AC-04 | ✅ | healthz + /api/* qua StoreIface |
| AC-05 | ✅ | TestSQLiteStore CRUD + PersistReopen + Memory xanh |
| AC-06 | ✅ | temp file, không panic |
| AC-07 | ✅ | header GOSO, không copy GoClaw |
| Smoke | ✅ | tạo agent/session/message, restart đọc lại đúng |
