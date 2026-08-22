# SPEC 012 — Deploy (Docker + Compose Overlays)

> LOCKED: 2026-08-20 — Đóng gói và triển khai GOSO bằng Docker, giữ 8 overlay ý tưởng từ GoClaw.

## Goal

`docker compose up` chạy được toàn bộ GOSO (gateway + control-plane + sqlite) ở local; có overlay cho production (postgres, backup...).

## User stories

- **US-01** `docker compose up` build gateway + control-plane và chạy ở `http://localhost:8080` (gateway) + `http://localhost:3000` (control-plane).
- **US-02** `docker compose -f compose.yml -f compose.prod.yml up` dùng overlay prod (nếu có).

## Acceptance criteria

- [x] AC-01 `Dockerfile` cho gateway (multi-stage Go 1.25), `control-plane/Dockerfile` (node 22)
- [x] AC-02 `compose.yml` (gateway + control-plane + volume `data/`), `compose.prod.yml` overlay
- [x] AC-03 `docker compose build` xanh
- [x] AC-04 Docs `docs/DEPLOY.md`

## Non-goals

- K8s, Helm — để sau
- CI deploy tự động — để sau (CI hiện chỉ verify)

## Ghi chú

- Tham khảo 8 overlay GoClaw gốc (postgres, redis, jaeger, tailscale...) nhưng tối giản cho GOSO.
