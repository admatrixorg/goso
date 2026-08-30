# SPEC 120 — CORE operator UI/UX

Status: ready for Grok worker
Owner: Grok implementation; Codex CTO architecture, merge, and live QC
Benchmark method: clean-room behavioral inspection of live Dewee only

## Goal

Make goso's CORE pages answer the same operator questions and cover the same operational states as the live Dewee CORE pages, without copying pixels, wording, CSS, components, or source. Scope is Overview, Chat, Agents, and Teams/Agent Links. Heatmap is a goso extra and is out of scope unless a shared CORE chrome change requires a small compatibility fix.

## Live surfaces and APIs

The worker must re-open both live apps before editing and record its own observations.

| Surface | Live Dewee behavior route | Live goso entry/page | Existing goso APIs |
| --- | --- | --- | --- |
| Overview | `http://127.0.0.1:18791/overview` | `http://127.0.0.1:3000`, App tab `crm`, `control-plane/src/pages/OverviewPage.tsx` | `GET /healthz`, `/api/stats`, `/api/agents`, `/api/sessions`, `/api/channels`, `/api/cron` via `overview-load.ts`; embedded CRM is a goso-only extra |
| Chat | `http://127.0.0.1:18791/chat` and a selected chat route | App tab `chat`, `ChatPage.tsx` plus compact `SessionsPage.tsx` | `/api/agents`, `/api/sessions`, `/api/sessions/:id/messages`, streaming `POST /api/chat` |
| Agents | `http://127.0.0.1:18791/agents` | App tab `agents`, `AgentsPage.tsx` | CRUD `/api/agents`, `/api/providers`; optimistic update uses `if_updated_at` |
| Teams and links | `http://127.0.0.1:18791/teams`, both Agent Teams and Agent Links tabs | App tab `teams`, `TeamsPage.tsx` | `/api/teams*`, `/api/agents/:id/links*`, `/api/agents/:id/evolution*` in `api/teams.ts` |

Do not call live paid vendors or invent successful provider, model, attachment, voice, transfer, or CRM behavior. If an operator question has no backing goso API, show an honest unavailable/DI state or link to a supported surface; do not add a live-looking action that returns 404/400.

## Verified live benchmark contracts

### Overview

Live Dewee separates Overview and Usage, shows connection/environment, scoped request/token/cost metrics, running agents and online channels, system health (uptime/database/providers/tools/sessions/clients), runtimes, channels, connected clients, cron empty state, and recent requests with status. Goso may use independent presentation and only supported data, but the operator must be able to distinguish healthy, degraded, unauthorized, offline, partial, stale, and genuinely empty data.

### Chat

Live Dewee provides an agent selector, New Chat primary action, chat list with message counts/timestamps and delete actions, a no-selection state, selected-agent readiness, context usage, transcript/tool-run state, attachment/voice affordances, composer, and send disabled rules. Goso must preserve its session-backed SSE contract and honestly mark unsupported attachment/voice behavior rather than faking it.

### Agents

Live Dewee provides Agent Transfer and Create Agent primary actions, search, type/creator filters, list/card modes, status, provider/model, prompt/evolution modes, context limit, pagination, and delete. The create dialog makes provider/model, description, evolution, and prompt-mode choices explicit. Goso can retain its own data model and wording, but create/edit/delete, current state, stale conflict, provider/model choice, prompt/orchestration semantics, and unsupported transfer status must be unambiguous.

### Teams and agent links

Live Dewee exposes Agent Teams and Agent Links as peer tabs. Teams has search, Create Team, and a truthful empty state. Agent Links remains reachable when there are no teams and shows source, target, direction, status, description, actions, Create Link, and its own empty state. Goso must not bury links behind a selected team.

## Concrete goso defects observed on 2026-08-30

1. The top gateway chrome can say `connected` while Overview reports unauthorized and dependent calls report `non-JSON response`; this is contradictory permission handling.
2. Overview removes nearly all operator questions in the unauthorized state and does not distinguish cached/stale values from missing values. Embedded CRM can add a second authorization failure without clear boundary from gateway health.
3. Chat currently shows a disabled empty agent selector, `non-JSON response`, `No sessions yet`, and a second `New session` action at once. Permission/error and true-empty states are conflated.
4. Chat lacks clear list metadata comparable to message count/last activity when data is available, and unsupported attachment/voice capability is not explicitly stated. SSE reconnect/error behavior exists but must remain visible and testable after chrome changes.
5. Agents currently shows `0 agents` and `No agents yet` alongside `non-JSON response`, which lies about emptiness. The create form is always expanded rather than led by a consistent primary action, and Agent Transfer availability is not answered.
6. Agents has optimistic concurrency data but no clear stale-conflict recovery chrome. Provider/model readiness and prompt-mode versus orchestration-mode meaning are not sufficiently clear at the decision point.
7. Teams currently shows `0 teams`, an always-visible create form, `Pick a team`, and `non-JSON response` together. This is another false empty state.
8. Agent links are buried inside selected-team detail even though they are an independent operator function; no peer Links tab/table/empty state is reachable when there are zero teams.
9. Page chrome and action placement vary across the four pages; title, primary CTA, filters, refresh, loading, permission, error, and empty treatment must be made consistent without a pixel clone.

## Required implementation

- Build a small shared state/chrome pattern only if it reduces inconsistencies without forcing unrelated pages into this change.
- For every scoped page implement explicit loading, empty, error, permission/401/403, and stale states. Error/permission must never render a simultaneous zero-count empty claim.
- Preserve last-known data only when it is clearly labeled stale with last refresh time; otherwise clear it and show the blocking state.
- Provide one obvious primary action per page and keep refresh/filter placement consistent.
- Overview: reconcile global gateway status with authenticated API readiness; show supported operational metrics and honest unavailable states for unsupported Dewee questions. Keep CRM visually separated as a goso-only operational extra.
- Chat: make agent/session selection and New Chat flow coherent; show list metadata available from current APIs; preserve SSE connecting/streaming/reconnect/error; keep send disabled until valid; selected-session deletion requires confirmation. Unsupported attachment/voice must be absent or explicitly unavailable, never fake-live.
- Agents: make create/edit modes explicit; show provider/model and agent status; surface stale conflict recovery; destructive delete must confirm the named target. Answer transfer availability honestly if no backing API exists.
- Teams: expose Agent Teams and Agent Links as peer views; links must be reachable with zero teams. Keep team/member/task/mailbox/evolution depth that already exists. Team deletion uses typed target confirmation; member removal and unlink use named/directional confirmation.
- Credential invariant: this scope should not add secrets. If a credential dependency is encountered, GET may expose only `*_set`, source, or safe prefix metadata; inputs start empty; rotate/clear are explicit writes; never hydrate a secret value.
- i18n is complete in Vietnamese and English; no new hard-coded operator copy.
- Add focused tests for state classification, stale handling, and destructive-confirm helpers, plus component/API helper tests feasible in the existing test harness.

## Acceptance criteria

1. All four goso CORE entries open with consistent title, primary CTA, filters where relevant, and refresh behavior.
2. Loading, true empty, generic error, permission, and stale are independently testable and never contradict one another.
3. No scoped action looks live if its API is absent or known to return 404/400.
4. Chat keeps SSE behavior and selection safe across refresh/deletion; destructive session delete confirms the target.
5. Agent create/edit/delete and provider/model choices are clear; stale conflict is actionable; delete confirms the target.
6. Agent Teams and Agent Links are peer views; links stay discoverable with no teams and show direction/status/empty state.
7. Vietnamese and English are complete; no credential values, prompts, or private message content are added to QA artifacts.
8. `cd control-plane && npm test && npm run typecheck && npm run build` pass.
9. `GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh` and `./scripts/agpl-check-docs.sh` exit 0 before merge.
10. Delivery is merged to `main` with `--no-ff` in sequence, then only Vite `:3000` is restarted. Never touch CRM `:8082`, sidecar `:8091`, or Dewee `:18791`.

## Worker delivery

Commit all implementation, i18n, tests, and a factual `docs/qa/120-core-ux.md` to the worker branch. Report the branch commit, exact checks, live pages inspected, and any remaining DI-only gaps. The Codex CTO will inspect the diff, run the merge gate, perform the `--no-ff` merge if the branch is acceptable, restart only `:3000`, and conduct live QC; a failed QC returns to a Grok follow-up worker.
