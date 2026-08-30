# CTO live QC — SPEC 119–126 resume

Date: 2026-08-30  
Role: Codex CTO, post-credit-refill QC only  
Target: `origin/main` at `7a167011965f3fba7e6ea48c03d1eb726e4fc4c5b1`

## Verdict

**PASS.** The merged SPEC 119–126 operator UX remains consistent with the advisor QA records after a fresh CTO live check. No product-code follow-up is required from this run.

## Live setup and scope

- Fetched `origin` and confirmed `origin/main`, local `HEAD`, and the requested merge point all resolve to `7a16701`.
- Confirmed the existing Vite demo at `http://127.0.0.1:3000` and Dewee benchmark at `http://127.0.0.1:18791` were listening and returned HTTP 200. Vite did not need a restart.
- Opened a fresh Orca browser tab, separate from the stale credit-limit pane, and hard-reloaded goso with `Meta+Shift+R`; the page reached network idle.
- Used Dewee read-only and for behavior questions only. No Dewee layout, wording, or source was copied.
- Did not restart, kill, or mutate CRM `:8082`, sidecar `:8091`, or Dewee `:18791`. No goso mutation, vendor call, QR flow, SSH/Docker execution, package install, stream start, export/import, backup, or secret flow was exercised.

## Navigation IA

The live goso sidebar contains exactly the required seven groups in the required order:

| English | Vietnamese |
| --- | --- |
| CORE | CỐT LÕI |
| CONVERSATIONS | HỘI THOẠI |
| CONNECTIVITY | KẾT NỐI |
| CAPABILITIES | NĂNG LỰC |
| DATA | DỮ LIỆU |
| MONITORING | GIÁM SÁT |
| SYSTEM | HỆ THỐNG |

## Goso pages checked

The live sweep opened every sidebar page in the unauthenticated/blocking state, plus the two previously sensitive peer/subviews:

| Group | Pages checked |
| --- | --- |
| CORE | Overview, Heatmap, Chat, Agents, Teams, Agent Links |
| CONVERSATIONS | Sessions, Pending, Contacts, Marketing |
| CONNECTIVITY | Channels, Nodes, Workstations |
| CAPABILITIES | Skills, Built-in Tools, MCP Servers, TTS, Cron, Webhooks, Connectors |
| DATA | Memory, Vault, Knowledge Graph, Storage |
| MONITORING | Traces, Events, Activity, Logs |
| SYSTEM | Tenants, Providers, API Keys, Packages, Config, Approvals, Import & Export; Config Gateway and Backup subviews |

Observed state contracts:

- Page chrome remained first-class and page-specific. Refresh/retry stayed available where appropriate.
- Blocking inventory failures rendered explicit unauthorized, error, or offline provenance. Numeric inventory metadata used `—`; no blocking inventory was presented as a successful zero-count/true-empty result.
- Create/write actions were disabled or hidden while their required inventory was blocked. This included New Chat, Create agent/team/link, conversation mutations, connectivity registration, capability writes, data writes, stream controls, SYSTEM creates/installs/saves, export, and backup creation.
- Agent Links preserved the failed agent-inventory state, showed metadata `—`, omitted the true-empty link claim, and kept Create Link disabled.
- Overview showed one authorization status alert; the page and chrome badges reflected the same health state without a duplicated alert. The offline CRM advisor used `—` and did not claim zero tips or an empty successful response.
- Config kept CRM and gateway provenance separate. The Gateway subview showed authorization failure with Save disabled; Backup showed authorization failure with Create snapshot disabled and made no S3 success claim.
- Forms containing credentials or other write-only values were not rendered in the blocking states, and no secret-shaped input was hydrated during the sweep.
- No goso page claimed live provider, TTS, webhook, channel, SSH/Docker, embedding, remote vault, S3, Grafana/OTEL, SSO, package-registry, or import/export success.

## Behavior benchmark

The Dewee sidebar again exposed the seven benchmark groups. Read-only live checks covered representative pages from every group: Overview, Sessions, Channels, Skills, Memory, Traces, and Providers. The comparison was limited to operator questions such as primary actions, tabs, filters, table fields, state visibility, and pagination; goso's independent wording and information architecture were retained.

## Verification gates

Run from this checkout after the live sweep:

- `control-plane`: `npm test` — 280/280 passed.
- `control-plane`: `npm run typecheck` — passed.
- `control-plane`: `npm run build` — passed.
- Source clean-room/AGPL check — passed.
- QA-document clean-room/AGPL check — passed.

This record contains no credentials, tokens, private message bodies, live vendor assertions, or secret values.
