# Orca hygiene (2026-08-24)

Full write-up: **goso-crm** `docs/qa/orca-hygiene.md`.

- 27 worktrees removed (goso 008–014 + cp-crm-metrics; goso-crm t01–t18 + deploy). Advisor + two primary mains kept.
- Local merged branches deleted; remotes kept.
- `orca terminal close` exists; dead worker terms already gone with worktrees. Keep `term_5738fa34`.
- `orca.yaml` `worktree.sharedDirectories`: `control-plane/node_modules`, `mcp/node_modules`, `.cache`.
- UI: CP preview 200; Orca snapshot **Home DEMO** OK; screenshot timeout; CRM tab click via CLI inconclusive → Design Mode GUI. Servers stopped.
