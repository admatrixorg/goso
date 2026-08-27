# Changelog

Mọi thay đổi đáng chú ý của GOSO được ghi ở đây.

## [Unreleased]

### Added

- Knowledge vault (SPEC 037): `[[wikilink]]` bidirectional registry, FTS5/substring `GET /api/vault/search`, filesystem sync under `GOSO_VAULT_DIR`, HTTP `/api/vault/docs` `/links` `POST /api/vault/sync`. Lexical only; semantic = DI-09.
- Memory L0/L1 (SPEC 036): episodic session summaries, FTS5/substring `GET /api/memory/search`, `GET`/`POST /api/memory`. Pipeline summarize/memory stages filled; no pgvector.
- Hardening (SPEC 013): `make verify` chạy gitleaks + semgrep (bắt buộc trên CI, skip nếu thiếu tool ở local) và `scripts/e2e.sh`.
- `docs/RUNBOOK.md` — khởi động, backup SQLite, xoay token, sự cố.
- `docs/RELEASE.md` — checklist version / changelog / tag.
- Pre-commit và GitHub Actions cài + chạy gitleaks 8.24.3, semgrep 1.110.0 (fail khi có secret).

## [0.1.0] — 2026-08-20

### Added

- Gateway Go: `version`, `doctor`, HTTP `/healthz`, agents/sessions/messages, chat echo/LLM, Telegram + Zalo webhooks.
- Auth Bearer (`GOSO_ADMIN_TOKEN`) + rate limit (`GOSO_RATE_LIMIT`).
- SQLite persist (`GOSO_DB_PATH`) với fallback in-memory.
- Control Plane Vite + React (agents / sessions / chat).
- Harness: Makefile, pre-commit, CI.
