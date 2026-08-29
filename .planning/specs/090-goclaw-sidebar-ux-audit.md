# SPEC 090 — CTO audit: GoClaw dashboard sidebar UX (lock queue 091+)

> LOCKED: 2026-08-29. **Docs-only this pass.** No product UI merge. Clean-room. CC-BY-NC: do **not** copy GoClaw / ZaloCRM code. Do not paste secrets. Do not bind/kill `:8082` `:8091`.

## Goal

Operator-visible GoClaw dashboard (`http://127.0.0.1:18791`, title Dewee) — **every left-nav item** — behavior study → goso Control Plane mapping → locked SPEC queue 091+.

## Login (no secrets in git/QA/chat)

- URL `http://127.0.0.1:18791/login`. Port **18789 is dead** — do not use.
- Form: **User ID + Gateway Token** (`type=password`). Not a web username/password pair.
- User ID = `system`.
- Token = process env `GOCLAW_GATEWAY_TOKEN` of `goclaw` listening **18791**. Read from the process. **Never echo.** File `/tmp/goclaw-orb-creds.txt` `password` is **not** the gateway token.

## Scope — every sidebar item

Groups seen live: CORE, CONVERSATIONS, CONNECTIVITY, CAPABILITIES, DATA, MONITORING, SYSTEM.

For each item record: operator purpose; layout (list/detail/tabs/dialogs); primary actions (create/edit/connect/approve/empty/error); credential pattern if any (write-only, mask, never-return); proposed goso CP mapping (existing page vs THIẾU).

## Deliverables (this pass)

1. `docs/qa/090-goclaw-sidebar-ux.md` — one section per nav item, behavior cites only.
2. `.planning/specs/090-sidebar-ux-queue.md` — SPEC numbers **091+** in operator priority:

   Overview → Chat/Sessions → Agents → Channels → Providers → Functions → Teams → Vault → Memory → Webhooks → Traces → Settings → remainder.

Do **not** implement UI in this pass. Commit on the worker branch. Coordinator merges docs `--no-ff`.

## Worker contract

`worker_done` once. Heartbeat if long. `orca orchestration ask` for blockers. No product secrets. No GoClaw Go copy.
