# PLAN 007 — Control Plane

> SPEC: `specs/007-control-plane.md` (LOCKED 2026-08-20)

| # | Task | File | Verify |
|---|------|------|--------|
| T01 | Khung Vite+React+TS | `control-plane/package.json`, `vite.config.ts`, `tsconfig.json`, `src/main.tsx` | `npm run build` |
| T02 | API client (gateway) | `control-plane/src/api/client.ts` | `npm run typecheck` |
| T03 | Pages (Agents/Sessions/Messages/Chat) | `control-plane/src/pages/*`, `src/App.tsx` | `npm run build` |
| T04 | Verify & docs | `control-plane/README.md`, repo `make verify` | `make verify` + `npm run verify` |
| T05 | QA AC 01–06 | smoke dashboard + gateway | checklist |

## Trạng thái

- [x] T01 — khung
- [x] T02 — api client
- [x] T03 — pages
- [x] T04 — verify
- [x] T05 — QA

## QA 2026-08-20
| AC | Kết quả | Bằng chứng |
| AC-01 | ✅ | Vite+React+TS, api/client.ts (GATEWAY_URL + token localStorage/env), proxy /api + /healthz |
| AC-02 | ✅ | Agents/Sessions/Chat pages, App tabs |
| AC-04 | ✅ | control-plane typecheck + build xanh, gateway make verify xanh |
| AC-05 | ✅ | token qua VITE_GOSO_ADMIN_TOKEN hoặc localStorage, không hardcode |
| Smoke | ✅ | gateway + preview (dist) + chat echo OK |
