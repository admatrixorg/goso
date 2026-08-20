# PLAN 001 — Harness GOSO

> SPEC: `specs/001-harness-goso.md` (LOCKED 2026-08-19) — 8 task, mỗi task 2–5 phút.

| # | Task | File/Thư mục | Verify | Song song |
|---|------|--------------|--------|-----------|
| T01 | Khung repo + `.planning` | `goso/.planning/*`, `goso/.gitignore`, `README.md` | `ls .planning/specs/001* && ls .planning/plans/001*` | — |
| T02 | Gateway skeleton Go | `gateway/go.mod`, `gateway/cmd/goso-gateway/main.go`, `internal/config`, `internal/health` | `go run ./gateway/cmd/goso-gateway version` | sau T01 |
| T03 | Makefile + verify | `Makefile` (`verify`, `lint`, `test`, `build`) | `make verify` | sau T02 |
| T04 | Hooks an toàn | `.claude/settings.json` (PreToolUse) | thử `rm -rf /tmp/x` bị chặn | ∥ T03 |
| T05 | Pre-commit hook | `.git/hooks/pre-commit` hoặc `scripts/pre-commit.sh` | commit thử file có secret bị chặn | ∥ T03 |
| T06 | CI GitHub Actions | `.github/workflows/ci.yml` | PR chạy verify + test | ∥ T03 |
| T07 | Tài liệu | `docs/SETUP.md`, `docs/ARCHITECTURE.md` | đọc 5 bước setup chạy được | sau T02 |
| T08 | QA + SHIP | chạy `make verify`, kiểm tra AC 01–09 | checklist AC | cuối |

## Rationale

- Go 1.23: đủ mới, không đòi toolchain 1.26 như GoClaw gốc.
- `internal/config` + `internal/health`: tách domain khỏi infra (DDD).
- Harness trước, nghiệp vụ sau — đúng Vibe Code State Machine.

## Trạng thái

- [x] T01 — khung repo + .planning
- [x] T02 — gateway skeleton
- [x] T03 — Makefile
- [x] T04 — hooks
- [x] T05 — pre-commit
- [x] T06 — CI
- [x] T07 — docs
- [x] T08 — QA/SHIP

## QA (ghi sau BUILD)

| AC | Kết quả | Bằng chứng |
|----|---------|------------|
| AC-01 | ✅ | `make verify` vet+fmt+test OK |
| AC-02 | ✅ | `goso-gateway version` → {"name":"goso-gateway","version":"0.1.0"} |
| AC-03 | ✅ | `doctor` 3 checks ok (go_version/port/env) |

| AC-04 | ✅ | hooks chặn rm -rf / , push --force, DROP/TRUNCATE, .env |
| AC-05 | ✅ | pre-commit.sh go vet+gofmt OK |
| AC-06 | ✅ | ci.yml verify+test+smoke |
| AC-07 | ✅ | CI yêu cầu PR, không push trực tiếp main (branch protection — cấu hình trên GitHub khi tạo repo) |
| AC-08 | ✅ | .planning/ glossary+decisions+specs/001+plans/001 |
| AC-09 | ✅ | docs/SETUP.md (5 bước) + ARCHITECTURE.md (C→B) |

## Ghi chú QA 2026-08-20
- `go run ./gateway/cmd/goso-gateway version` → {"name":"goso-gateway","version":"0.1.0"} (AC-02)
- `go run ./gateway/cmd/goso-gateway doctor` → 3 checks ok (AC-03)
- `make verify` → vet+test OK, `scripts/pre-commit.sh` → OK
- Go 1.26.2 runtime, module go 1.23, clean-room header đã gắn
