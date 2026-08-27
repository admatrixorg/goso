# SPEC 033-cp — Hide Settings placeholders (align with goso-crm SPEC 033)

> LOCKED: 2026-08-27. Sibling source of truth: `goso-crm/.planning/specs/033-audit-fix.md`.
> Do **not** bind or kill ports 8082, 8091, 3000, 18088.

## Goal

Control-plane Settings currently has a **placeholders** nav item (`src/pages/SettingsPage.tsx`) for the six unfinished CRM settings (SSO/webhooks/notifications/heatmapcfg/export/integrations). goso-crm SPEC 033 **cuts** those pages. Hide the placeholders item so CP matches.

Keep live panels: account, users, roles, nicks, quotas, templates, billing, theme.

## AC

- [ ] AC-01 Placeholders menu item gone from Settings nav (demo and non-demo).
- [ ] AC-02 Live 6 settings + billing + theme still load.
- [ ] AC-03 `npm run typecheck` in `control-plane/` exit 0. No product change outside Settings nav.
- [ ] AC-04 Do not restart the advisor Vite demo on :3000.

## Non-goals

SSO/webhooks implementation, K8s, Grafana, notarize, billing money, CRM Go code (different repo).
