# SPEC 020-UI — Lot 2 control-plane pages (after API merge)

> LOCKED: 2026-08-26. Depends on goso-crm SPEC 020/021/022 on main.
> One worker owns all CP pages to avoid App.tsx races.

## Goal

Settings: 6 live panels calling `/api/settings/*` with `X-Org-Token`; other items “Đang phát triển”.  
Marketing: 7 tabs calling `/api/marketing/*` (replace mock).  
Heatmap: report view calling `/api/crm/heatmap`.  
Keep i18n if 019 already merged; else Vietnamese literals OK and 019 rebases.

## AC

- [ ] AC-01 Non-demo build still hides SPEC 017 DEMO tabs.
- [ ] AC-02 Settings 6 forms persist via API (fake CRM).
- [ ] AC-03 Marketing 7 tabs switch + list/create audience/campaign.
- [ ] AC-04 Heatmap renders buckets or empty state.
- [ ] AC-05 typecheck + build. No AGPL copy.

## Non-goals

Pixel-perfect 13 settings, Zalo send, Grafana.
