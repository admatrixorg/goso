# SPEC 119 — Seven-group operator navigation

Status: locked
Owner: Codex CTO
Scope: control-plane navigation IA only

## Problem

The live goso sidebar has three groups (`OVERVIEW`, `WORK`, `SYSTEM`). Most operator surfaces are dumped into `SYSTEM`, so users cannot scan the product by operational intent.

## Clean-room benchmark

The live Dewee UI at `http://127.0.0.1:18791` was inspected behaviorally on 2026-08-30. Its operator journey is organized as CORE, CONVERSATIONS, CONNECTIVITY, CAPABILITIES, DATA, MONITORING, and SYSTEM. No source, layout, CSS, component, or wording is copied.

## Locked IA

1. CORE — Overview, Heatmap, Chat, Agents, Teams.
2. CONVERSATIONS — Sessions, Pending Messages, Contacts, Marketing.
3. CONNECTIVITY — Channels, Nodes, Workstations.
4. CAPABILITIES — Skills, Built-in Tools, MCP Servers, TTS, Cron, Hooks, Connectors.
5. DATA — Memory, Vault, Knowledge Graph, Storage.
6. MONITORING — Traces, Realtime Events, Activity, Logs.
7. SYSTEM — Tenants, Providers, API Keys, Packages, Config, Approvals, Import & Export.

Settings is represented as Config in SYSTEM; backup and restore remains a Settings-bounded panel. The existing Functions implementation remains shared, while Skills, Built-in Tools, MCP Servers, and Cron each receive a navigation entry that targets the corresponding section. Demo-only pages may be added to CORE or CONVERSATIONS but must not replace or hide the seven live operator groups.

## Acceptance criteria

- `liveSide()` in `control-plane/src/App.tsx` returns exactly the seven locked groups in order.
- English and Vietnamese provide every `nav.group.*` label.
- Heatmap is in CORE; Marketing is in CONVERSATIONS; Connectors is in CAPABILITIES; Config is in SYSTEM.
- Skills, Built-in Tools, MCP Servers, TTS, Cron, and Hooks are visible capability entries.
- The live app at `http://127.0.0.1:3000` is restarted and visually verified after merge.
- Control-plane tests and typecheck pass; `scripts/agpl-check.sh` exits 0 before merge.

## Non-goals

- Pixel parity or copying Dewee presentation.
- Vendor credential configuration or invented integration success.
- Top-level Backup & Restore outside Settings without new evidence.
