# SPEC 122 — CONNECTIVITY operator UI/UX

Status: queued after SPEC 121 CTO QC
Owner: Grok implementation; Codex CTO architecture, merge, and live QC
Benchmark method: clean-room behavioral inspection of live Dewee only

## Goal

Make goso's CONNECTIVITY pages answer the same operator questions and cover the same operational states as the live Dewee pages, without copying pixels, wording, CSS, components, or source. Scope is Channels, Nodes, and Workstations. Preserve goso's stronger write-only channel-secret, pairing, and safe workstation-test contracts.

## Live surfaces and APIs

The worker must re-open both live apps before editing and record its own observations.

| Surface | Live Dewee behavior route | Live goso entry/page | Existing goso APIs |
| --- | --- | --- | --- |
| Channels | `http://127.0.0.1:18791/channels`, channel detail, and Add Channel dialog | App tab `channels`, `control-plane/src/pages/ChannelsPage.tsx` | catalog `GET /api/channels`; secret `PUT/DELETE /api/channels/:name/secrets`; test and policy patch; Zalo Personal QR/logout; pairing list/approve/deny via `api/channels.ts` |
| Nodes | `http://127.0.0.1:18791/nodes` | App tab `nodes`, `NodesPage.tsx` | `GET /api/nodes`; typed approve/deny/revoke via `api/nodes.ts` |
| Workstations | `http://127.0.0.1:18791/workstations` and Add Workstation dialog | App tab `workstations`, `WorkstationsPage.tsx` | list/get/create/patch/test plus typed disconnect/delete via `api/workstations.ts`; agent inventory via `/api/agents` |

Do not call live channels, scan QR codes, SSH to hosts, run Docker, or invent connectivity success. Do not add an Add Channel action: the cited goso API has no channel-create route. Unsupported vendor setup remains an honest configuration/DI answer rather than a live-looking 404/400 action.

## Verified live benchmark contracts

### Channels

Live Dewee presents searchable, paginated channel instances with display name, type, key, assigned agent, enabled/running/attention state, last diagnosis, next step, detail access, Add Channel, and refresh. Its create dialog asks for a unique key, display name, channel type, agent, an empty credential input with an explicit never-returned server-side contract, enabled state, DM/group policies, streaming/reasoning/media behavior, allowlists, and group/topic overrides. Goso must answer the same supported questions using its catalog, health/remediation, policy, pairing, and secret endpoints. Where a Dewee setting has no goso route, omit it or label it unavailable.

### Nodes

Live Dewee describes Nodes as paired devices plus pending pairing requests, provides Refresh, and currently shows a truthful single empty state: no paired devices or pending requests. Goso has separate pending and paired tables and supports approve, deny, and revoke. The operator must see request/device identity, kind, expiry/last seen, health, action progress, and an unambiguous distinction between channel pairing on Channels and dashboard-device pairing on Nodes.

### Workstations

Live Dewee describes remote SSH/Docker execution targets, provides Refresh and Add Workstation, and shows a truthful empty state. The add dialog asks for display name, workstation key, backend, host/port, SSH user, and optional identity-file path. Goso additionally supports agent assignment, safe config validation, disconnect, and delete. An identity path/ref is metadata, never private-key content; configuration validation is not a claim that an external host was contacted successfully.

## Concrete goso defects observed on 2026-08-30

1. Channels currently shows global `Gateway · connected` with `non-JSON response`, then simultaneously claims zero pairing requests and zero channels. Permission/error and true-empty states are conflated.
2. Channels exposes valuable catalog, health, required environment, pairing, QR, boxed-secret, policy, and remediation behavior, but the page lacks a single coherent state boundary when catalog and pairing requests fail independently.
3. Channel secret guidance is strong, yet mutation availability and catalog failure are not consistently linked. Inputs must remain empty on every open; GET state may show only `secret_set`, environment/source, or a safe prefix. Empty save remains rejected, Rotate/Replace and Clear must be explicit, and clear/logout must confirm the named target.
4. Dewee has Add Channel, while goso has no channel-create API. Goso must not add or imply that action; it should explain how catalog instances become available using factual existing configuration only.
5. Pairing approval can be operationally buried by the channel error and must never ask the operator to re-enter or reveal a pairing code. Pending rows, action progress, success/failure, expiry, and true empty must be distinct.
6. Nodes currently shows `Error: 401 unauthorized`, `0 pending`/`No pending requests`, and `0 devices`/`No devices` together. This is a false empty state during a permission failure.
7. Nodes already has typed approve/deny/revoke safety and secret filtering, but the action hierarchy, pending-versus-paired status, stale refresh handling, and channel-pairing boundary need consistent operator chrome.
8. Workstations currently shows `Error: 401 unauthorized`, an enabled Add Workstation form/action, `0 targets`, and `No workstations` together. Mutations remain available while authorization is blocked.
9. Workstation creation does not visibly distinguish field validation, safe configuration test, remote-connect availability, and stored state strongly enough. Test must never imply real SSH/Docker execution if the backend only validates configuration.
10. Title, primary CTA, search/filter, refresh, loading, permission, error, empty, stale, and last-refresh treatment vary across the three surfaces.

## Required implementation

- Reuse the state/chrome pattern accepted in earlier batches; do not fork a second incompatible pattern.
- For each page implement explicit loading, true empty, filtered empty where relevant, generic error, permission/401/403, partial dependency failure, and stale states. Error or permission must never render simultaneous zero-count empty claims or enabled mutation forms.
- Preserve last-known data only when clearly labeled stale with last successful refresh time; otherwise clear it and show the blocking state. Independent Channel catalog and pairing failures may be shown separately only when each section's current/stale provenance is clear.
- Use one obvious primary action and consistent refresh/search/filter placement. Channel's primary operator action may be refresh or supported pairing/configuration work because channel creation is not backed by goso.
- Channels: keep catalog search/health filtering, remediation, policy, safe secret rotation/clear, test, pairing approve/deny, QR state, and Zalo Personal logout. Make enabled/health/agent/required-config/last diagnosis and next safe step clear from real data. Do not add unsupported channel creation or vendor controls.
- Nodes: keep separate pending and paired views, secret filtering, approve, deny, and revoke. Actions require typed device id/display confirmation; revoke and deny state their consequence; channel pairing remains linked conceptually to Channels and is not duplicated here.
- Workstations: keep create/edit, SSH/Docker fields, agent assignment, safe test, disconnect, and delete. Make identity path/ref metadata explicit and reject private-key-looking content. Disconnect/delete require typed named confirmation. Test/result chrome must distinguish validation from real connectivity.
- Destructive actions: channel secret Clear, Zalo Personal logout, pairing denial where irreversible, node deny/revoke, workstation disconnect/delete, and any newly exposed destructive operation must confirm the named target and describe consequence.
- Credential invariant: GET returns only `*_set`, safe source, environment ownership, or safe prefix metadata. Inputs always start empty and never rehydrate after save/refresh. Rotate/Replace and Clear are explicit writes. Never render, log, screenshot, fixture, or write to QA a token, code, private key, password, or raw vendor error that could contain one.
- Server/API errors must pass through existing public redaction and truncation. Pairing lists and node data must retain public-shape filtering.
- Complete i18n in Vietnamese and English; no new hard-coded operator copy.
- Add focused tests for state classification, partial/stale handling, empty versus filtered-empty behavior, secret-body formation, private-key rejection, pairing public-shape filtering, and destructive-confirm helpers.

## Acceptance criteria

1. Channels, Nodes, and Workstations open with consistent page chrome, primary action, refresh, and filters where relevant.
2. Loading, true empty, filtered empty, generic error, permission, partial dependency failure, and stale are independently testable and never contradict one another.
3. Channels answers instance health/configuration/pairing questions using existing APIs and presents no Add Channel or other fake-live action.
4. Channel credentials are write-only: GET safe metadata only, empty inputs, explicit rotate/clear, named confirmation, redacted errors, and no code/token/private-key exposure.
5. Node pending/paired states and channel-versus-device pairing boundaries are clear; approve/deny/revoke are typed, progress-aware, and API-confirmed.
6. Workstation create/edit/test/disconnect/delete uses existing APIs; identity remains a path/ref; validation is not represented as real remote success; destructive actions are typed.
7. No live vendor, QR, SSH, Docker, or paid-provider success is claimed without factual configured evidence.
8. Vietnamese and English are complete.
9. `cd control-plane && npm test && npm run typecheck && npm run build` pass.
10. `GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh` and `./scripts/agpl-check-docs.sh` exit 0 before merge.
11. Delivery is merged to `main` with `--no-ff` after SPEC 121 passes CTO QC, then only Vite `:3000` is restarted. Never touch CRM `:8082`, sidecar `:8091`, or Dewee `:18791`.

## Worker delivery

Commit all implementation, i18n, tests, and a factual `docs/qa/122-connectivity-ux.md` to the worker branch. Report the branch commit, exact checks, live pages inspected, and remaining DI-only gaps. The Codex CTO will inspect the diff, run the merge gate, perform the `--no-ff` merge if acceptable, restart only `:3000`, and conduct live QC; a failed QC returns to a Grok follow-up worker.
