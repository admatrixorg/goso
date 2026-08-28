# SPEC 080 — Cron heartbeat + sessions_* agent tools

> After 079. Ticker already 054.

Closes **K8**.

## GoClaw cite

`docs/08-scheduling-cron.md`, `docs/22-heartbeat-system.md` — periodic heartbeat jobs; session tools.

## goso plan

1. Heartbeat: optional `GOSO_HEARTBEAT=1` (default off) fires a documented no-op or `POST /api/system/heartbeat` that records `last_heartbeat` in stats. Cron can target session chat as today.
2. Agent tools `sessions_list` / `sessions_history` (read-only, jail to store ListSessions/ListMessages, cap 50). LLM ToolCalls only.
3. Tests: tools via scripted provider; heartbeat default off.
4. CP: show last heartbeat on health chrome if present.

Commit `admatrixmdp/spec080-heartbeat-tools`. After 080: queue empty except DI parked.
