# SPEC 005 — DB Persist (SQLite)

> LOCKED: 2026-08-20 — Lưu Agent/Session/Message xuống SQLite, giữ in-memory như fallback.

## Goal

Gateway giữ lại dữ liệu sau restart: thay `Store` in-memory thuần bằng **SQLite** ( `modernc.org/sqlite` — pure Go, không cần CGO), file `data/goso.db` (config qua `GOSO_DB_PATH`). Khi không có DB (hoặc `GOSO_DB_PATH=:memory:`), fallback về in-memory. Migration tạo bảng `agents`, `sessions`, `messages`.

## User stories

- **US-01** Operator chạy `GOSO_DB_PATH=data/goso.db goso-gateway gateway --port 8090`, tạo Agent/Session/Message, restart gateway → dữ liệu vẫn còn (`GET /api/agents`/`/api/sessions` trả lại).
- **US-02** Không đặt `GOSO_DB_PATH` hoặc đặt `:memory:` → hành vi như cũ (in-memory).

## Acceptance criteria

- [ ] AC-01 `gateway/internal/store/sqlite.go` — `SQLiteStore` implement cùng interface với `Store` (CreateAgent/ListAgents/GetAgent/CreateSession/ListSessions/GetSession/AddMessage/ListMessages), dùng `database/sql` + `modernc.org/sqlite`
- [ ] AC-02 Migration tự chạy khi mở DB: tạo 3 bảng nếu chưa có
- [ ] AC-03 `gateway/cmd/goso-gateway` chọn Store: nếu `GOSO_DB_PATH` khác rỗng và khác `:memory:` → SQLite file, ngược lại in-memory; log `store: sqlite <path>` hoặc `store: memory`
- [ ] AC-04 `GET /healthz` giữ nguyên; `GET /api/*` hoạt động với cả 2 Store
- [ ] AC-05 Unit test cho SQLiteStore (temp file), và smoke restart (tạo data, mở lại DB, đọc lại)
- [ ] AC-06 `make verify` xanh, không panic khi DB file chưa tồn tại
- [ ] AC-07 Không copy code GoClaw; header GOSO

## Non-goals

- Postgres/pgvector — để SPEC riêng (server profile)
- FTS5/vector search — sau
- Backup/restore, WAL tuning — để sau
- Control Plane TS — track riêng

## Ghi chú

- Driver: `modernc.org/sqlite` (pure Go, không CGO) thay vì `mattn/go-sqlite3` (cần CGO, không phù hợp build cross).
- Interface: giữ `Store` hiện có, thêm interface `StoreIface` nếu cần để cả 2 Store thỏa.
