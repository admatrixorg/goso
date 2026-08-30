# QA — SPEC 122 CONNECTIVITY operator UI/UX

Date: 2026-08-30. Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. Worker does not merge and does not restart Vite `:3000`. Demos `:8082` `:8091` `:18791` were not bound or killed. No live paid vendors. No secrets, pairing codes, private keys, or vendor tokens in this record.

## Live surfaces inspected (worker, before edits)

| Surface | URL | Observation (operator questions, not pixels) |
| --- | --- | --- |
| Dewee Channels | `http://127.0.0.1:18791/channels` | Title “Manage channel instances”; Add Channel + Refresh; search; two ZaloCRM Personal-bridge instances with display name, type, key, assigned agent, enabled/running/attention, last diagnosis, next step, detail; `2 items`; Rows 20; Page 1 of 1. |
| Dewee Add Channel dialog | same page, dialog only, not submitted | Create Channel Instance: unique Key, Display Name, type, agent, empty Bot Token with never-returned contract, API server/proxy, DM/group/media/streaming/reasoning, allowlist, group/topic overrides, Enabled. Cancel/Create/Close. |
| Dewee Nodes | `http://127.0.0.1:18791/nodes` | Title “Manage paired devices and pending pairing requests”; Refresh; truthful empty “No devices” / “No paired devices or pending pairing requests.” |
| Dewee Workstations | `http://127.0.0.1:18791/workstations` | Title remote SSH/Docker targets; Refresh + Add Workstation; truthful empty “No workstations configured”. |
| Dewee Add Workstation dialog | same page, dialog only, not submitted | Display Name, Workstation Key, Backend (SSH), Host, Port 22, SSH User, Identity File (optional path), Cancel, Create disabled until required fields. |
| goso Channels | App tab `channels` | Chrome `Gateway · unauthorized`; page `non-JSON response` together with pairing meta `0`, “No pending pairing requests.”, catalog `0 channels`, “No channels.”, and enabled QR logout. |
| goso Nodes | App tab `nodes` | `Error: 401 {"error":"unauthorized"}` together with `0 pending` / “No pending requests.” and `0 devices` / “No devices.” |
| goso Workstations | App tab `workstations` | `Error: 401 {"error":"unauthorized"}` together with enabled Add workstation, `0 targets`, and “No workstations.” |

HTTP probes before edits: Dewee `/channels` `/nodes` `/workstations` 200 HTML; goso `:3000/` and `/healthz` 200; goso `/api/channels` `/api/nodes` `/api/workstations` 401 `{"error":"unauthorized"}`. CRM `:8082`, sidecar `:8091`, Dewee `:18791`, Vite `:3000` were left running. No QR scan, no SSH, no Docker, no live channel call.

## What changed

Shared CORE chrome (`PageChrome`, `PageStatus`) plus `classifyPageState` so loading / true-empty / filtered-empty / error / permission / stale are exclusive. Error and permission never render a zero-count empty claim. Mutations stay closed while inventory is error or permission. Independent Channel catalog and pairing loads keep their own current/stale provenance. Stale keeps last-known data only when labeled with last-load time.

| Page | Operator contract |
| --- | --- |
| Channels | Refresh is the primary (no Add Channel; no create API). Search/health filter, catalog health/enabled/agent/required env, last diagnosis, next remediation step, write-only rotate/clear (empty inputs, empty save rejected), named Clear and Zalo Personal logout, pairing approve with progress, typed deny of request id/sender (never a code), QR status only. Streaming/reasoning/media/topic-override and channel create stay unavailable copy. |
| Nodes | Refresh is the primary. Separate pending and paired tables. Channel pairing stays linked to Channels and is not duplicated. Approve/deny/revoke are typed id/display confirms; deny/revoke state consequence. Pending-empty and paired-empty only after a successful load. |
| Workstations | Add workstation is the primary, Refresh beside it. Create/edit/test/disconnect/delete. Identity is a path/ref; PEM/key material is rejected. Test chrome is local configuration validation, not live SSH/Docker. Server id after create; no operator-chosen workstation-key field. Disconnect/delete are typed named confirms. Add/form/test/mutations hide while authorization is blocking. |

i18n vi+en for all new operator copy. GET listings still drop secret-shaped rows. Pairing public-shape filtering drops code/code_hash rows.

## Checks

```
cd control-plane && npm test && npm run typecheck && npm run build
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

- `npm test`: 218/218 pass (includes catalog/pairing independent `resolveSettled`, `publicPairingList` code drop, `pairingConfirmMatch`, empty secret draft / empty PUT body, `wsFormError` private-key rejection, `testOutcome` validation-only, permission-over-empty for nodes and workstations, filtered-empty vs catalog error).
- `npm run typecheck`: pass.
- `npm run build`: pass.
- `agpl-check` and `agpl-check-docs`: exit 0.

## DI-only gaps (honest unavailable, no fake live action)

- Channel create / Add Channel (no goso channel-create route). Catalog instances come from configuration and environment.
- Dewee streaming, reasoning, media-size, human-like delivery, intermediate/quick-ack, and per-group topic override fields.
- Separate Dewee display-name vs key on catalog rows (goso catalog `name` is the instance key).
- Workstation operator-chosen key field (goso stores a server id after create).
- Live vendor connect, QR image/scan, SSH, or Docker execution success. Channel test reports catalog health from the existing test route. Workstation test validates stored configuration only.

## Out of scope

Merge and Vite `:3000` restart belong to Codex CTO. CRM `:8082`, sidecar `:8091`, Dewee `:18791` untouched.

No credentials, pairing codes, private keys, or secret values are included in this record.

## Advisor live QC (CTO credit exhausted)

Date: 2026-08-30. Codex CTO did not repeat the browser checks. Grok advisor ran them after merge `d19c979` (`Merge SPEC 122 CONNECTIVITY operator UX`, `--no-ff`) of `2bbb1e2` + `2f90bc6` on top of SPEC 121 `08a76ae` (follow-up `inventoryBlocksMutation` still present). Clean-room React. No ZaloCRM / goclaw-source copy. No banned author ids. No secrets, pairing codes, private keys, or vendor tokens in this record.

Restart: Vite `:3000` only (new listen pid `57332`). Unchanged: CRM `:8082` pid `85417`, sidecar `:8091` pid `83346`, gateway `:18080` pid `68421`. Dewee `:18791` not bound or killed.

Advisor re-ran `npm test` (218/218) and `npm run typecheck` on the worker worktree before merge. Source and QA AGPL checks passed. i18n en/vi key sets match (1745).

Browser: Orca isolated profiles `qc120-unauth` (no `goso_token`) and `qc120-auth`. Hard-reload `http://127.0.0.1:3000/`, then CONNECTIVITY Channels / Nodes / Workstations.

| Defect | Live unauth (401) | Live auth | Verdict |
| --- | --- | --- | --- |
| Channels error + `0` pairing/`0 channels` + implied Add Channel | Refresh primary. Honest copy “Không có Thêm kênh”. Pairing and catalog both 401 with meta `—`. `0 channels` / `No channels` / pending-empty copy absent. Logout button `disabled=true`. | Catalog loads. No Add Channel button. Refresh enabled. | PASS |
| Nodes 401 + `0 pending`/`0 devices` empty | Refresh primary. 401. Pending meta `—`, paired meta `—`. Empty claims absent. | True-empty after successful load: `0 chờ` / `Không có yêu cầu ghép.` and `0 thiết bị` / `Không có thiết bị.` | PASS |
| Workstations 401 + enabled Add + `0 targets` empty | `Thêm máy chạy` `disabled=true`. 401. List meta `—`. Empty-on-error copy absent. | True-empty `0 máy` / `Không có máy chạy.` Add enabled. | PASS |

### Advisor verdict: PASS — SPEC 122 closed

Do not spawn a second 122 worker. Sequential 123+ only when asked. CRM `:8082` and sidecar `:8091` remain untouched.
