# PLAN 012 — Deploy (Docker + Compose)

> SPEC: `specs/012-deploy.md` (LOCKED 2026-08-20)

| # | Task | File | Verify |
|---|------|------|--------|
| T01 | Dockerfile gateway + control-plane | `Dockerfile`, `control-plane/Dockerfile` | `docker build` |
| T02 | compose.yml + overlay | `compose.yml`, `compose.prod.yml` | `docker compose config` |
| T03 | Docs | `docs/DEPLOY.md` | `docker compose up` smoke |

## Rationale

- **Multi-stage Dockerfile**: gọn, không CGO (pure Go).
- **Compose + overlay prod**: tối giản, tham khảo 8 overlay GoClaw.
- **Volume data/**: persist SQLite.

## Trạng thái

- [ ] T01 — dockerfiles
- [ ] T02 — compose
- [ ] T03 — docs
