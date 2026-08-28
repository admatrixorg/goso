# CTO audit retry — goso main after SPEC 034–065

> LOCKED: 2026-08-28. **Docs only.** No product-code edits. No merge.
> Auditor: Orca `--agent codex --model gpt-5.6-sol` (user confirmed this runner works). Do **not** use 9Router `cx/*`.
> Do **not** bind/kill `:8082` `:8091`. Curl `:18080` / `:3000` allowed. Do not fake success.

Attempt 1 (`ctx_98467875940e`, 05:49Z) **failed**: 0 heartbeat, 0 commits, TUI stuck after MCP `goclaw-admin`/`stitch` fail + pasted spec. This is **attempt 2**.

Style: `docs/qa/audit-market-readiness-2026-08-26.md` (+ v2/v3) + SPEC 034 matrix honesty. **Every finding: file path + short commit.** No vibe.

## Scope

This worktree at **current `main`** (`ae592a9` Merge SPEC 065 or newer). SPECs **034–065** + control-plane live/DEMO.

Out of scope for edits: goso-crm product, ZaloCRM/goclaw-source (read-only if citing parity).

## Questions

1. **Parity remaining** vs `docs/qa/034-goclaw-parity-matrix.md` + `docs/qa/034-goclaw-parity-update-054.md` after 035–065.
2. **Security**: empty admin token / dev mode, `GOSO_MASTER_KEY`, provider/channel keys never in GET, SSRF flag, webhook hashing, injection scan. Real vs documented.
3. **UX / CP**: after UI-gaps 057–065 (agent editor, session create, chat chrome, ⌘K, responsive, cron UX, channels env names, DEMO honesty, form validation) — what is **still** a no-op or missing? Cross-check `.planning/specs/ui-gaps-queue.md`. Confirm closed vs leftover with evidence.
4. **AGPL / no-copy**: no `minhhaiphan` / `locphamnguyen` outside `.planning`; no goclaw/ZaloCRM copy in product.
5. **Secrets in git**: no product API keys, `GOSO_ADMIN_TOKEN` values, master keys in tracked files.
6. **Do not** claim `cx/*` chat works. Live model is `ocg/deepseek-v4-flash` via `router9`.

## Deliverable

Write **one** file: `docs/qa/audit-cto-2026-08-28.md`

- HEAD commit + date
- Method (files read, curls)
- Honest % or RAG per axis: parity, security, UX, AGPL/secrets, ops
- Findings table: id | severity | evidence (path + commit) | vs SPEC | next SPEC or DI
- Non-findings (looks broken but CẮT / parked / already 057–065)
- Exit **0** if the report is written and evidence-backed; **1** only if you could not read the tree or wrote no report.

Do **not** implement fixes. Do not kill demo PIDs. No product secrets in the report.

Heartbeat every few minutes: `orca orchestration send --type heartbeat --subject alive --task-id … --dispatch-id …` (IDs from preamble).

## QC

agpl-check 0 on the docs commit. Commit `admatrixmdp/audit-cto-034-065`. Do not merge.
