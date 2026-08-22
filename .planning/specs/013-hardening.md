# SPEC 013 — Hardening & Release Readiness

> LOCKED: 2026-08-20 — Cứng hóa trước khi thu phí: secret scan, semgrep, E2E, runbook.

## Goal

GOSO sẵn sàng cho user trả tiền thử nghiệm: quét secret, semgrep, e2e đầy đủ, và runbook vận hành.

## User stories

- **US-01** `make verify` chạy thêm `gitleaks` + `semgrep` (nếu có) và fail khi có secret.
- **US-02** Có `docs/RUNBOOK.md` (khởi động, backup SQLite, xoay token, sự cố).
- **US-03** Release checklist (`docs/RELEASE.md`) — version, changelog, tag.

## Acceptance criteria

- [x] AC-01 Pre-commit + CI chạy gitleaks/semgrep (đã có skeleton, làm chặt hơn)
- [x] AC-02 `docs/RUNBOOK.md` + `docs/RELEASE.md`
- [x] AC-03 E2E script `scripts/e2e.sh` chạy toàn bộ luồng (healthz → agents → sessions → chat → webhook)
- [x] AC-04 `make verify` xanh trên CI và local

## Non-goals

- Pentest chính thức — thuê ngoài sau
- SLA 99.9% — để sau

## Ghi chú

- E2E có thể là shell script curl hoặc Go test e2e.
