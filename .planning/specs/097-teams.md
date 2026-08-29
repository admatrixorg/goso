# SPEC 097 — Teams operator surface

> After 096. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 097. Audit: `docs/qa/090-goclaw-sidebar-ux.md` Agent Link & Team.

## Goal

Finish **TeamsPage** as the operational team workspace: create/edit, lead/member roles, directed/bidirectional agent links, task/message coordination, guarded evolution suggestions. Searchable list/detail, empty/error, validation, unlink/remove with confirmation.

## AC

- [x] List: search, empty/loading/error. Detail: members + lead, links (direction visible), tasks/messages if APIs exist.
- [x] Create/edit team; add/remove member; set lead. Unlink/remove requires named confirmation.
- [x] Evolution suggestions remain guarded (existing auto_adapt/locked). Do not dump full system prompts.
- [x] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/097-teams.md`.

## Out of scope

Tenants admin (112). Copying GoClaw dialogs. Live vendor tokens.
