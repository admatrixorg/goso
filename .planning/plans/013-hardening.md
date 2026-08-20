# PLAN 013 — Hardening & Release Readiness

> SPEC: `specs/013-hardening.md` (LOCKED 2026-08-20)

| # | Task | File | Verify |
|---|------|------|--------|
| T01 | Cứng hóa pre-commit/CI (gitleaks/semgrep) | `.github/workflows/ci.yml`, `scripts/pre-commit.sh` | `make verify` |
| T02 | Runbook + release docs | `docs/RUNBOOK.md`, `docs/RELEASE.md` | `cat docs/RUNBOOK.md` |
| T03 | E2E script | `scripts/e2e.sh` | `./scripts/e2e.sh` |

## Rationale

- **gitleaks/semgrep đã có skeleton**: làm chặt hơn, không thêm tool mới.
- **E2E shell**: đơn giản, chạy mọi luồng.
- **Runbook/Release docs**: bắt buộc trước khi thu phí.

## Trạng thái

- [x] T01 — scan
- [x] T02 — runbook
- [x] T03 — e2e
