# SPEC 121 — CONVERSATIONS operator UI/UX

Status: queued after SPEC 120 CTO QC
Owner: Grok implementation; Codex CTO architecture, merge, and live QC
Benchmark method: clean-room behavioral inspection of live Dewee only

## Goal

Make goso's CONVERSATIONS pages answer the same operator questions and cover the same operational states as the live Dewee pages, without copying pixels, wording, CSS, components, or source. Scope is Sessions, Pending Messages, Contacts, and the goso-only Marketing extra. Preserve supported goso workflows such as contact merge and pending compaction where they add operator value.

## Live surfaces and APIs

The worker must re-open both live apps before editing and record its own observations.

| Surface | Live Dewee behavior route | Live goso entry/page | Existing goso APIs |
| --- | --- | --- | --- |
| Sessions | `http://127.0.0.1:18791/sessions` and selected rows linking to Chat | App tab `sessions`, `control-plane/src/pages/SessionsPage.tsx`; compact variant also appears inside Chat | `GET/POST /api/sessions`, `PATCH/DELETE /api/sessions/:id`, `GET /api/agents` via `control-plane/src/api/client.ts` |
| Pending Messages | `http://127.0.0.1:18791/pending-messages` | App tab `pending`, `PendingPage.tsx` | metadata-only `GET /api/pending-messages`, `POST /api/pending-messages/:id/compact`, `DELETE /api/pending-messages/:id/clear` via `api/pending.ts` |
| Contacts | `http://127.0.0.1:18791/contacts` | App tab `contacts`, `ContactsPage.tsx` | `GET /api/contacts`, `GET /api/contacts/:id`, `POST /api/contacts/:id/merge`, `POST /api/contacts/:id/undo` via `api/contacts.ts` |
| Marketing | no peer Dewee navigation item; use Dewee's shared operator state and page-chrome behavior only | App tab `marketing`, `MarketingPage.tsx` | CRM-bound `/api/marketing/overview`, `/audiences`, `/campaigns`, and campaign `PATCH` through `api/marketing.ts` and `api/crm.ts`; organization is carried by `X-Org-ID` |

Do not call live paid vendors or invent successful channel delivery, compaction, CRM campaign scheduling, file import, Lead Ads import, or broadcast behavior. If an operator question has no backing goso API, show an honest unavailable/DI state or omit the action; do not add a live-looking control that returns 404/400.

## Verified live benchmark contracts

### Sessions

Live Dewee makes the session inventory a browse surface: search, rows identifying session and agent, context usage, message count, last update, row-to-chat navigation, result count, page size, and pagination. Goso may expose only fields its API actually returns. The operator still needs to know which session and agent are involved, prompt mode, creation/last-activity metadata when supported, how to open chat, and whether the list is loading, unavailable, stale, filtered empty, or truly empty. Session creation should be a deliberate primary action instead of an always-expanded form that competes with browsing.

### Pending Messages

Live Dewee explains that inbound channel messages are buffered by conversation group, exposes refresh and truthful empty state, and distinguishes Compact from Clear: compact is an LLM-backed summarization operation while clear permanently removes pending messages. Goso's current metadata-only privacy boundary is stronger and must remain: counts, age, agent, channel, and state are allowed; message bodies and credentials are not. Compact and Clear must state their prerequisites, irreversible effects, and progress/result without implying success before the API confirms it.

### Contacts

Live Dewee explains contacts as identities discovered from channel interactions, provides refresh, channel-permission guidance, search by name/username/ID, channel and type filters, selection, identity/channel/type/last-seen fields, result count, and pagination. Goso's canonical identity, channel-ID view, merge, and undo are valid extras. Merge/undo must make target versus source and data-loss consequences clear.

### Marketing

Marketing is goso-only. It must follow the same operator contract for title/description, organization boundary, refresh, loading, permission, error, stale, filtered empty, and true empty. Tabs for Audiences, Group Scan, Goals, Care, Sequence, Broadcast, and Content Blocks must disclose which behavior is actually backed by the CRM API. Paste/file/Lead Ads sources and campaign states must not look executable if the server only stores metadata or a dependency is not configured.

## Concrete goso defects observed on 2026-08-30

1. Sessions currently shows a gateway error, `0 sessions`, `No sessions`, and an always-expanded New Session form together. Permission/error and true-empty are conflated.
2. Sessions browse rows omit the live benchmark's context, message-count, updated-at, and pagination questions. The current API type exposes only id, agent, label, prompt mode, and created time, so unsupported fields must be answered honestly rather than fabricated or fetched through an uncontrolled N+1 loop.
3. Session creation and browsing compete for attention; there is no consistent primary-action disclosure pattern. Delete already confirms, but the named target and selection handoff to Chat must remain safe after refresh/deletion.
4. Pending Messages already has useful privacy-safe grouping and typed Compact/Clear confirmation, but live unauthorized shows `Error: 401 unauthorized`, `0 groups`, and table chrome without a blocking permission state. Error and empty semantics are not fully separated.
5. Pending Compact and Clear share nearly identical action weight even though their dependency and irreversibility differ. The UI must say when compaction is unavailable or in progress and must never surface buffered message content in public state or QA.
6. Contacts currently shows `Error: 401 unauthorized`, `0 contacts`, and table headers together. It does not clearly distinguish permission, true empty, filtered empty, and stale last-known data.
7. Contacts has valuable merge/undo behavior, but selection, target/source confirmation, pagination reset, and post-operation recovery need consistent chrome. Channel permission guidance is generic and must remain factual for supported channel metadata only.
8. Marketing currently renders `Error: 401 unauthorized` alongside create fields and `No audiences yet`. This falsely presents both an empty result and enabled mutation controls during a permission failure.
9. Marketing's file and Lead Ads audience sources and campaign tabs can look live even when the API stores only supplied metadata or external dependencies are DI. Every CTA must be backed by the cited CRM route and must disclose unavailable execution capability.
10. Title, primary CTA, filters, refresh, loading, permission, error, empty, and last-refresh treatment vary across all four surfaces.

## Required implementation

- Reuse the state/chrome pattern accepted in SPEC 120 if available; do not fork a second incompatible pattern.
- For every scoped page implement explicit loading, true empty, filtered empty where relevant, generic error, permission/401/403, and stale states. Error or permission must never render a simultaneous zero-count empty claim or enabled mutation form.
- Preserve last-known data only when clearly labeled stale with a last successful refresh time; otherwise clear it and show the blocking state. A refresh failure must not silently relabel old data as current.
- Provide one obvious primary action per page and consistent refresh/filter placement. Creation forms should be disclosed deliberately and remain dismissible where practical.
- Sessions: make inventory browsing primary; preserve search/agent filtering, prompt-mode update, create, open-in-Chat, and named delete confirmation. Show only supported metadata and an honest unavailable answer for context/message-count/update questions the API cannot provide.
- Pending: retain counts-only privacy filtering and typed target confirmation. Separate Compact from irreversible Clear visually and semantically; expose dependency/unavailable, in-progress, success, and failure states from real responses only.
- Contacts: retain search, channel/type filters, detail, pagination, canonical identity, merge, and undo. Make two-contact selection and target/source direction unambiguous; merge and undo require typed named confirmation and safe selection/page recovery.
- Marketing: clearly separate gateway state from CRM organization/auth state. Each tab needs truthful loading/empty/error/permission/stale treatment. Disable or label actions whose external execution dependency is absent; do not turn metadata creation into a claim of imported leads, scheduled delivery, or sent messages.
- Destructive actions: session deletion, Pending Clear, contact merge/undo, and any newly exposed destructive operation must confirm the named target and describe irreversibility. Do not invent delete actions where no API exists.
- Credential invariant: GET responses may expose only `*_set`, safe source, or safe prefix metadata; credential inputs start empty; rotate/clear are explicit writes. Never hydrate, log, render, test-fixture, screenshot, or include token/secret/message-body values in QA.
- Complete i18n in Vietnamese and English; no new hard-coded operator copy.
- Add focused tests for state classification, stale handling, filter/empty distinction, pagination recovery, and destructive-confirm helpers, plus feasible API helper tests in the existing Node test harness.

## Acceptance criteria

1. Sessions, Pending Messages, Contacts, and Marketing open with consistent page chrome, primary action, refresh, and filters where relevant.
2. Loading, true empty, filtered empty, generic error, permission, and stale are independently testable and never contradict one another.
3. Sessions remains safely linked to Chat; create/update/delete use existing APIs; delete confirms a named target; unsupported inventory metrics are labeled unavailable rather than invented.
4. Pending never exposes buffered bodies or credentials; Compact and Clear have distinct consequences, typed confirmation, progress, API-confirmed result, and truthful dependency failures.
5. Contacts search/filter/pagination and canonical identity remain usable; merge/undo make direction and consequence clear and require typed confirmation.
6. Marketing visibly identifies the CRM organization boundary and never implies import, schedule, broadcast, or vendor success beyond real API state.
7. No scoped action looks live if its API is absent or known to return 404/400. No secrets or private message content appear in UI errors, tests, logs, screenshots, or QA.
8. Vietnamese and English are complete.
9. `cd control-plane && npm test && npm run typecheck && npm run build` pass.
10. `GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh` and `./scripts/agpl-check-docs.sh` exit 0 before merge.
11. Delivery is merged to `main` with `--no-ff` after SPEC 120 passes CTO QC, then only Vite `:3000` is restarted. Never touch CRM `:8082`, sidecar `:8091`, or Dewee `:18791`.

## Worker delivery

Commit all implementation, i18n, tests, and a factual `docs/qa/121-conversations-ux.md` to the worker branch. Report the branch commit, exact checks, live pages inspected, and remaining DI-only gaps. The Codex CTO will inspect the diff, run the merge gate, perform the `--no-ff` merge if acceptable, restart only `:3000`, and conduct live QC; a failed QC returns to a Grok follow-up worker.
