# SPEC 001 — Harness GOSO (Nền tảng & Gateway Skeleton)

> LOCKED: 2026-08-19 — Harness + Gateway skeleton clean-room cho GOSO (C→B)

## Goal

Dựng **nền tảng GOSO** đủ để các SPEC tiếp theo xây lên: harness chuẩn (hooks, CI, pre-commit), cấu trúc repo, gateway skeleton Go chạy được (`version` + `doctor` + `gateway --help`), và tài liệu vận hành. Không chứa nghiệp vụ channel/LLM/billing.

## User stories

- **US-01** Là nhà phát triển, tôi chạy `make verify` và toàn bộ gate (lint, format, secret-scan, test) chạy xanh trong <2 phút.
- **US-02** Là operator, tôi chạy `go run ./gateway/cmd/goso-gateway --help` và thấy hướng dẫn; `version` và `doctor` trả về JSON hợp lệ.
- **US-03** Là reviewer, tôi thấy `.planning/` (glossary, decisions, specs, plans) được commit và CI chặn push trực tiếp vào `main`.
- **US-04** Là người mới, tôi đọc `docs/SETUP.md` và dựng được môi trường local theo 5 bước.

## Acceptance criteria

- [ ] AC-01 `make verify` chạy lint + format check + gitleaks + semgrep (nếu có) + `go vet` + `go test ./...`
- [ ] AC-02 `goso-gateway version` in `{"name":"goso-gateway","version":"0.1.0"}` (JSON)
- [ ] AC-03 `goso-gateway doctor` kiểm tra Go version, cổng, env và trả về JSON
- [ ] AC-04 Hooks `.claude/settings.json` chặn `rm -rf`, `git push --force`, `DROP/TRUNCATE`, sửa `.env`
- [ ] AC-05 Pre-commit hook: lint + format + gitleaks (fail nếu có secret)
- [ ] AC-06 CI (GitHub Actions) chạy trên PR: verify + test, chặn merge khi đỏ
- [ ] AC-07 Branch protection: không push trực tiếp main, yêu cầu PR + CI xanh
- [ ] AC-08 `.planning/` tồn tại và được commit (glossary, decisions, specs/001, plans/001)
- [ ] AC-09 `docs/SETUP.md` và `docs/ARCHITECTURE.md` tồn tại, mô tả 5 bước setup + sơ đồ C→B

## Non-goals (out of scope)

- Channel adapter (Telegram, Zalo...), LLM provider, session/WS, auth, billing — để SPEC 002+
- Control Plane API/Dashboard — SPEC 002
- Desktop Wails — SPEC 00X riêng
- MCP server mới — tận dụng `goclaw-mcp` hiện có, đổi brand sau

## Clarifications

- Go 1.25 (tương thích GoClaw 1.26, tránh yêu cầu toolchain quá mới)
- Module `github.com/mqglobal/goso` (đổi khi có org chính thức)
- Không copy bất kỳ file GoClaw gốc; mọi file có header license GOSO
