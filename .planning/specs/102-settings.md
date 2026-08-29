# SPEC 102 — Settings operator surface

> After 101. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 102. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Config.

## Goal

Reorganize **SettingsPage** into account/team/messaging/system sections. Add missing gateway configuration contracts for behavior defaults, quotas, tools, integrations, and environment-managed fields. Mark read-only/env-owned values. Never return auth tokens. Preserve backup/pairing/theme. Permission-aware save, validation, conflict feedback.

## AC

- [ ] Sections: account / team / messaging / system (or equivalent grouping of existing cards). Loading/error/save feedback.
- [ ] Env-owned fields marked read-only. GET never returns gateway auth tokens.
- [ ] Keep backup, pairing, theme. Validation + conflict (409) if APIs support it.
- [ ] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/102-settings.md`.

## Out of scope

API Keys page (113). Tenants admin (112). Packages (114). Copying GoClaw Config tabs. Live vendor tokens.
