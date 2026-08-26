# QA — SPEC 017 DEMO gate (control-plane)

Date: 2026-08-26.

`VITE_DEMO_MODE=true|1` → tab Home / Việc / Họp / Bạn bè / Lịch / Kho ảnh / Marketing + mock `demo/mock.ts`.
Unset / `false` (default build) → those tabs **omitted** from nav; default tab = Tổng quan. Non-goal: 13 settings, 7 marketing, heatmap.

## Builds

| Mode | JS | `Trang chủ` | `Vinh Phát` | `Tổng quan` |
|------|----|-------------|-------------|-------------|
| `VITE_DEMO_MODE=false` `dist/assets/index-CyjaNoG8.js` (179 kB) | **absent** | **absent** | present |
| `VITE_DEMO_MODE=true` `/tmp/cp-demo/.../index-BFl1jfup.js` (205 kB) | present | present | present |

`npm run typecheck` OK. Committed `dist/` is the **non-demo** build (market default). Preview not left running.

Scripts: `npm run build` (non-demo), `npm run build:demo`.
