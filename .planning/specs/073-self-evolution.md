# SPEC 073 — Self-evolution auto-adapt + guardrails

> LOCKED after SPEC 072 merge. Clean-room. Do not kill `:8082` `:8091`.

Closes matrix **E1** (PARTIAL today: suggestions + apply-to-instructions, not auto-adapt).

## GoClaw (cite, no copy)

| Behavior | Cite |
|----------|------|
| Metrics → suggestions → auto-adapt; 3 stages; guardrail apply/rollback | goclaw-source `docs/01-agent-loop.md` Self-Evolution; README self-evolution |
| Guardrails: max delta, min data points, rollback on quality drop, locked params | `docs/01-agent-loop.md` Adaptation Guardrails |
| Never silently change identity / name / core purpose | same (locked_params / identity anchoring) |

## goso today

- `GET /api/agents/{id}/evolution` + `POST .../apply` (038). Display-name apply **rejected**. Instructions apply allowed. No scheduled auto-adapt, no rollback.

## goso plan (self-written)

1. Guardrails persisted on agent (JSON on existing row or `evolution_guardrails` text): `auto_adapt` default **false**, `min_runs` default 20, `locked`: always include `display_name`, `agent_key`, identity/core purpose. Cannot unlock name/key via API.
2. `POST /api/agents/{id}/evolution/tick` (admin) — or in-process ticker when `GOSO_EVOLUTION_AUTO=1` (default **off** so demo unchanged): if `auto_adapt` and `chat_runs >= min_runs`, apply the **first pending suggestion that is not locked**. Record apply + baseline run count. Second tick with a synthetic quality drop (`error_rate` up) rolls back instructions to the pre-apply snapshot.
3. Apply path: **instructions / style only**. `display_name`, `agent_key`, model provider identity → 400.
4. Tests: auto-adapt off → tick no-op; on + enough runs → instructions change; name suggestion still 400; rollback restores previous instructions.
5. CP: existing evolution UI — checkbox “Tự thích nghi” + min_runs. i18n. StatusLine.

## Non-goals

Live quality ML. Changing SOUL.md files. Copying goclaw. Binding demo ports.

QC: typecheck, go test, build, agpl, agpl-docs.
`docs/qa/073-self-evolution.md` with cite table.
Commit `admatrixmdp/spec073-self-evolution`. Do not merge. Do not start 074.
