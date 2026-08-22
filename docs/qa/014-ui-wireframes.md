# QA — ZAgent UI on GOSO Control Plane

Date: 2026-08-23  
Branch: `admatrixmdp/ui-zagent-wireframes`  
Source: `zcrm-wireframes-d-n/project/ZAgent UI.dc.html` + `SKILL.md` / design foundation.

Skin only. API không đổi: `api/client.ts`, `api/crm.ts`, `VITE_GOSOCRM_API_URL`, `X-Org-ID`.

## Design tokens đã dùng

| File | Role |
|------|------|
| `control-plane/src/styles/tokens/colors.css` | Light + `body.dark` aliases (`--accent #2463eb`, semantic green/orange/red) |
| `control-plane/src/styles/tokens/typography.css` | Inter ramp, 600 workhorse, tabular-nums |
| `control-plane/src/styles/tokens/spacing.css` | 22px gutter, 12px card gap, 1280 min-width |
| `control-plane/src/styles/tokens/shape.css` | Card 12px, no card shadow |
| `control-plane/src/styles/tokens/motion.css` | 130/140/190/200/450ms, zFade/zRise/zBar/zPulse, reduced-motion `.01ms` |
| `control-plane/src/styles/tokens/fonts.css` | Inter CDN |
| `control-plane/src/assets/icons.svg` | Sprite 49 + `arrow-up` / `chev-right` / `mic` from the HTML |

Components ported (TS): `Icon`, `Button`, `Badge`, `Avatar`, `Card`/`CardHeader`, `SectionHeader`, `KpiCard`, `EmptyState`.

## Wireframe ↔ trạng thái

| Màn ZAgent UI | Mapping GOSO | Status |
|---------------|--------------|--------|
| Chrome bar (Z + nav + search + theme + bell + avatar) | `App.tsx` top bar | **done** (nick-online pill → “Gateway”; search local filter chưa lọc list) |
| Sidebar groups + Cài đặt | `App.tsx` sidebar | **tương đương** — 6 mục GOSO, không 50 mục CRM |
| Home / Agent prompt + meeting sources | không có meeting API | **thiếu** — không clone mock họp |
| Việc của tôi / 3 dashboard | không có task API | **thiếu** |
| Chat 3 cột (tin nhắn) | tab Chat = phiên compact + `ChatPage` | **tương đương** — 2 cột, API session/message |
| Bạn bè / Lịch / Gallery / Marketing | không có API | **thiếu** |
| Báo cáo 7 tab (KPI/funnel/heatmap) | tab Tổng quan = `CrmMetricsPage` | **tương đương** — 6 KPI + advisor từ `/api/crm/metrics` + `/api/crm/advisor` |
| Settings 13 trang / wizards / modal | không có | **thiếu** |
| Agents list | tab Agent | **done** (skin) |
| Sessions list | tab Phiên | **done** (skin) |
| Connectors | tab Kết nối | **done** (skin; health badge) |
| Events | tab Nhật ký | **done** (skin; kind badge) |

## Verify

```
cd control-plane && npm run typecheck && npm run build
make -C desktop verify
go vet ./...
```

## goso-crm `web/` (phụ, không sửa)

Đã align chrome/sidebar/tokens (`web/static/tokens/*`, `layout.html` brand Z, dark toggle). Label còn mix English (`Dashboard`). Không đụng trong task này.

## Giả định

1. Pixel-perfect = chrome + density + tokens + sprite trên **6 màn GOSO**, không rebuild 50 màn CRM.
2. Placeholder “Z” 26px accent square đúng spec (không vendor logo AGPL).
3. Desktop reuse 3 pages; import CSS tokens từ control-plane. `wails.json` không đổi.
4. Search ⌘K / bell là chrome visual; không thêm API.
