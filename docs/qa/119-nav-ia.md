# QA — SPEC 119 navigation IA

Date: 2026-08-30
Benchmark: live Dewee behavior at `http://127.0.0.1:18791`
Target: live goso at `http://127.0.0.1:3000`

## Before

Live goso exposed three groups from `control-plane/src/App.tsx`: TỔNG QUAN (Overview, Heatmap), LÀM VIỆC (Agents through Storage), and HỆ THỐNG (Connectors through Traces). The embedded-browser snapshot confirmed that specialist connectivity, capabilities, monitoring, and system pages were visually flattened into HỆ THỐNG.

## Benchmark group map

| Group | Live Dewee operator entries |
| --- | --- |
| CORE | Overview, Chat, Agents, Agent Link & Team |
| CONVERSATIONS | Sessions, Pending Messages, Contacts |
| CONNECTIVITY | Channels, Nodes, Workstations |
| CAPABILITIES | Skills, Built-in Tools, MCP Servers, TTS, Cron, Hooks |
| DATA | Memory, Vault, Knowledge Graph, Storage |
| MONITORING | Traces, Realtime Events, Activity, Logs |
| SYSTEM | Tenants, Providers, API Keys, Packages, Config, Approvals, Import & Export, Backup & Restore |

## After map

| Group | goso entries | Intentional difference |
| --- | --- | --- |
| CORE | Overview, Heatmap, Chat, Agents, Teams | Heatmap is a goso extra; Teams answers the team entry point. |
| CONVERSATIONS | Sessions, Pending, Contacts, Marketing | Marketing is a goso extra. |
| CONNECTIVITY | Channels, Nodes, Workstations | Same operator questions. |
| CAPABILITIES | Skills, Built-in Tools, MCP Servers, TTS, Cron, Webhooks, Connectors | Webhooks and Connectors are goso extras; Functions sections remain shared implementation. |
| DATA | Memory, Vault, Knowledge Graph, Storage | Same operator questions. |
| MONITORING | Traces, Events, Activity, Logs | Independent goso wording. |
| SYSTEM | Tenants, Providers, API Keys, Packages, Config, Approvals, Import & Export | Backup remains inside Config/Settings. |

## Verification record

- [x] Before state opened at `:3000`; three group headings observed.
- [x] Live Dewee opened at `:18791`; seven group headings and 32 routes observed.
- [x] After state hard-refreshed at `:3000` after merge `344ae35347354479814652fa0e6512614effdde5` and a Vite-only restart; exactly seven group headings observed.
- [x] Vietnamese and English group labels visually checked in the live sidebar.
- [x] Skills navigation opened Functions, marked Skills active, and moved the scroll container toward `functions-skills`; the section was visible. The same code path targets Tools, MCP, and Cron by stable section id.
- [x] `npm test`: 169/169 pass; `npm run build`: pass; TypeScript: pass.
- [x] Clean-room gates: sibling `agpl-check.sh` with `GOSO_ROOT=$PWD` exit 0; repo-local `scripts/agpl-check-docs.sh` exit 0.

Verdict: **PASS**. The live operator navigation now follows the locked seven-group journey in both locales. Live gateway/CRM degradation remained visible and was not misrepresented as vendor success.

No credentials or secret values are included in this record.
