# CTO audit — goso main after SPEC 034–056

> LOCKED: 2026-08-28. **Docs only.** No product-code edits. No merge. Clean-room read of goso.
> Auditor: Orca `--agent codex --model gpt-5.6-sol` (CLI PONG verified; do **not** use 9Router `cx/*`).
> Do **not** bind/kill `:8082` `:8091`. Curl `:18080` / `:3000` is allowed (live demo). Do not fake success.

Style: previous market-readiness audits (`docs/qa/audit-market-readiness-2026-08-26.md` and v2/v3) + SPEC 034 matrix honesty. **Findings need file path + commit** (short hash). No vibe.

## Scope

Repo `/Users/mqglobal/Documents/goclaw-binary/goso` (this worktree) at **current `main`** after merge SPEC 056 (`7b0f5a3` or newer). SPECs **034–056** + control-plane live/DEMO tabs.

Out of scope for edits: goso-crm product code, ZaloCRM/goclaw-source (read-only if citing parity).

## Questions

1. **Parity remaining** vs `docs/qa/034-goclaw-parity-matrix.md` + `docs/qa/034-goclaw-parity-update-054.md`: which rows are still THIẾU/PARTIAL/CẮT, with evidence.
2. **Security**: auth (empty token / dev mode), secrets (`GOSO_MASTER_KEY`, provider keys never in GET), SSRF flag, webhook hashing, injection scan. Real vs documented.
3. **UX / CP**: live tabs vs DEMO; leftover “looks clickable but no-ops” (⌘K, session create, agent edit, channels env, cron silent submit, minWidth 1280). Cross-check `.planning/specs/ui-gaps-queue.md` — confirm, add, or reject items with evidence.
4. **Do not** claim chat/`cx/*` works. Locked live model is `ocg/deepseek-v4-flash` via `router9`. 9Router `cx/*` was 401 token_expired.

## Deliverable

Write **one** file: `docs/qa/audit-cto-2026-08-28.md`

Must include:

- HEAD commit + date
- Method (files read, curls run)
- Score or RAG status per axis: parity, security, UX, ops (honest %)
- Findings table: id | severity | evidence (path + commit) | vs SPEC | suggested next SPEC or DI
- Explicit **non-findings** (things that look broken but are CẮT / parked)
- Exit: process exit **0** if the report is written and evidence-backed; **1** only if you could not read the tree or wrote no report. Findings themselves are not a failing exit.

Do **not** implement fixes. Do not kill demo PIDs. Do not put product secrets in the report.

## QC

agpl-check 0 on the docs commit. No `minhhaiphan` / `locphamnguyen` outside `.planning`. Commit `admatrixmdp/audit-cto-034-056`. Do not merge.
