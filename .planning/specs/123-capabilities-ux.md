# SPEC 123 — CAPABILITIES operator UI/UX

Status: queued after SPEC 122 CTO QC
Owner: Grok implementation; Codex CTO architecture, merge, and live QC
Benchmark method: clean-room behavioral inspection of live Dewee only

## Goal

Make goso's CAPABILITIES entries answer the same operator questions and cover the same operational states as live Dewee without copying pixels, wording, CSS, components, or source. Scope is Skills, Built-in Tools, MCP Servers, TTS, Cron, Webhooks, and the goso-only Connectors extra. Functions may remain one implementation page, but each nav entry must behave like a first-class operator surface rather than a buried scroll target.

## Live surfaces and APIs

The worker must re-open both live apps before editing and record its own observations.

| Surface | Live Dewee behavior route | Live goso entry/page | Existing goso APIs |
| --- | --- | --- | --- |
| Skills | `http://127.0.0.1:18791/skills` | App tab `skills`, `FunctionsPage.tsx` focus `skills` | list/search/create/delete `/api/skills*` via `api/skills.ts` |
| Built-in Tools | `http://127.0.0.1:18791/builtin-tools` | tab `tools`, `FunctionsPage.tsx` focus `tools` | per-agent list/toggle `/api/agents/:id/tools*` via `api/tools.ts` |
| MCP Servers | `http://127.0.0.1:18791/mcp` | tab `mcp`, `FunctionsPage.tsx` focus `mcp` | connector list/create/patch/test `/api/connectors*` via `api/client.ts`, `api/tools.ts` |
| TTS | `http://127.0.0.1:18791/tts` | tab `tts`, `TTSPage.tsx` | public status, write, test, typed clear `/api/tts*` via `api/tts.ts` |
| Cron | `http://127.0.0.1:18791/cron` | tab `cron`, `FunctionsPage.tsx` focus `cron` | list/create/toggle/delete `/api/cron*`; session inventory `/api/sessions` via `api/cron.ts` |
| Hooks/Webhooks | `http://127.0.0.1:18791/hooks` | tab `webhooks`, `WebhooksPage.tsx` | signed HTTP webhook list/create/rotate/revoke/test/replay `/api/webhooks*` via `api/webhooks.ts` |
| Connectors | no separate Dewee peer; MCP and tool behavior are the relevant benchmark | tab `connectors`, `Connectors.tsx` | connector registration and agent assignment `/api/connectors`, `/api/agents/:id/connectors` |

Do not install dependencies, invoke model/media vendors, connect MCP servers, send outbound webhook traffic, run cron jobs, or claim TTS success during QC. Test actions must remain disabled or DI-labeled unless the real local dependency is configured. Never add a live-looking action for a Dewee behavior that goso's cited APIs do not support.

## Verified live benchmark contracts

### Skills

Live Dewee exposes Core/Custom inventory, refresh/rescan/install-dependency actions, missing-dependency diagnosis, attention/disabled/archived counts, search, status filters, sort, pagination, per-skill enable/edit, and bulk selection. Goso only supports list/search/create/delete. It must answer name/path/status available from its API, creation/archive consequence, loading/empty/error/permission/stale, and explicitly mark dependency rescan/install, enable/edit, bulk work, and richer status as unavailable rather than faking them.

### Built-in Tools

Live Dewee groups tools by category and shows display name, stable tool key, description, requirement/config hints, global enable switches, search, and settings where supported. Goso is per-agent and exposes connector, approval requirement, configured/granted state, and enable toggle. Agent selection, permission, configuration readiness, and toggle result must be unambiguous; no global/settings claim may be inferred from a per-agent endpoint.

### MCP Servers and Connectors

Live Dewee provides search, Add Server, refresh, and truthful empty state. Its create flow covers name/display name, stdio/SSE/streamable HTTP transport, command or endpoint, args, environment variables, agent hints, tool prefix, timeout, enabled, per-user credentials, and connection test. Goso supports a smaller connector model: name, transport, endpoint/command, optional credential ref/write-only token, enabled state, test, and agent assignment. Keep that scope honest and make the relationship between MCP Servers and the goso-only Connectors entry clear.

### TTS

Live Dewee exposes provider choice, disabled state, auto-apply mode, reply mode, text/timeout limits, refresh, and Save. Goso supports provider/config/status, key source/set metadata, empty API-key writes, Test, and typed Clear. Provider readiness, environment ownership, disabled/not-configured/ready, dirty/saved state, and test outcome must be distinct; no vendor success is assumed.

### Cron

Live Dewee provides search, refresh, New Job, truthful empty state, and schedule choices for interval, cron, or once with agent and message. Goso jobs target a session and accept supported `every:Nm|Nh` or five-field UTC specs. The operator must see spec, session, message metadata, enabled state, last run/error, validation, and named delete consequence without implying unsupported schedule types.

### Hooks versus HTTP webhooks

Live Dewee Hooks are beta agent-lifecycle interceptors with event, handler, scope, enable, test, audit-aware guidance, and built-in edit restrictions. Goso's page is signed inbound/outbound HTTP Webhooks, not lifecycle hooks. Preserve this explicit distinction. Webhook secrets are shown once only on create/rotate, then removed from UI memory after copy/hide; GET lists prefix/status only. Test/replay must not imply delivery when an outbound endpoint/dependency is absent.

## Concrete goso defects observed on 2026-08-30

1. The four Functions entries open one long page with a global agent selector and every section/form mounted. Entry labels are first-class in navigation but the page chrome still reads only `Functions`, so operator context and primary action are ambiguous.
2. In the unauthorized live state, Tools, MCP, Skills, and Cron each show `non-JSON response` while also showing zero counts, empty copy, and enabled create/toggle forms. Permission/error and true-empty are conflated four times on one screen.
3. Built-in Tools starts with `0 tools` and `Pick an agent` while the shared agent load itself failed. It cannot distinguish missing selection, no agents, permission, tool-route 404, true empty, and filtered empty.
4. MCP shows `0 connectors`, a live Add Connector form, and `No connectors` during the same error. Its token field is empty, which is correct, but permission gating, environment ownership, rotate/clear semantics, per-transport validation, and test readiness need consistent treatment.
5. Skills shows `0 skills`, create fields, and an error simultaneously. Delete confirmation does not name the target; no honest unavailable answer exists for Dewee dependency/status/edit/enable questions.
6. Cron shows `0 jobs`, create fields, and an error simultaneously. Session dependency failure is merged into the job failure, deletion does not name the job, and unsupported once schedules must not be invented.
7. TTS shows `401 unauthorized`, `not configured`, `no API key`, and enabled Save/Test inputs at once. Permission state is mislabeled as configuration absence and mutations remain active.
8. HTTP Webhooks shows `401 unauthorized`, an enabled Create Webhook form, `0 webhooks`, and a last-secret empty panel together. The one-time secret contract is strong but cannot compensate for false empty/mutation state.
9. Connectors shows `non-JSON response`, prefilled registration fields, `0 connectors`/`No connectors`, and agent-assignment controls at once. It duplicates MCP connector concerns without explaining ownership or preventing drift.
10. Page title, primary CTA, refresh, search/filter, loading, permission, error, empty, stale, and last-refresh treatment are inconsistent across capability entries.

## Required implementation

- Reuse the accepted shared state/chrome pattern. Each nav entry must display its own title, description, primary action, refresh, filters, and section-scoped state even if `FunctionsPage` remains shared internally. A tabbed Functions shell is acceptable; a misleading generic page with all mutation forms active is not.
- Implement loading, true empty, filtered empty, generic error, permission/401/403, dependency unavailable, and stale states for every scoped entry. Error/permission must not render a simultaneous zero-count claim or enabled mutation form.
- Skills: keep list/search/create/archive using current APIs; archive requires named confirmation. Label dependency management, enable/edit, bulk, and richer status unavailable when no API backs them.
- Tools: require a successfully loaded agent inventory and deliberate agent selection; separate no-agent, no-selection, route-unsupported/404, true empty, filtered empty, permission, and stale. Toggle is per-agent and must show configured/granted/approval facts without implying global scope.
- MCP/Connectors: define one coherent connector inventory and ownership model reused by both entries. Preserve name/transport/endpoint, environment ownership, token/credential-ref, enabled, test, and agent assignment supported by APIs. Do not duplicate independent forms/state that drift.
- TTS: preserve public-shape filtering and provider-specific validation. GET may expose only key/source/set metadata; form API key starts and remains empty. Save, Test, and typed Clear are disabled in permission state and distinguish DI/not configured from failure.
- Cron: retain session-backed schedule contract, validation, enabled toggle, last-run/error, and create/delete. Delete confirms a named job. Do not add unsupported once/agent-target schedule semantics.
- Webhooks: keep the explicit HTTP-versus-lifecycle distinction. Preserve show-once token/HMAC behavior, prefix-only GET, rotate/revoke confirmation, test/replay prerequisites, and removal from UI memory after copy/hide/navigation. Name destructive targets.
- Preserve last-known lists only as visibly stale with last successful refresh time; otherwise clear them on blocking failure. Independent dependency failures may remain visible only with clear provenance.
- Credential invariant: GET only `*_set`, safe source/environment ownership, or safe prefix. Secret inputs start empty; rotate/replace/clear are explicit. Never hydrate, log, screenshot, fixture, QA, or retain token, HMAC, API key, MCP env value, credential ref content, hook payload, or private message.
- Complete i18n in Vietnamese and English; no new hard-coded operator copy.
- Add focused tests for scoped state classification, selection/dependency states, write-only request bodies/public responses, one-time secret disposal, connector transport validation, schedule validation, and named destructive confirms.

## Acceptance criteria

1. Every CAPABILITIES nav entry opens at a stable first-class title/state/action context; no operator function remains buried behind a generic Functions heading.
2. Loading, true empty, filtered empty, generic error, permission, dependency unavailable, and stale are independently testable and never contradict one another.
3. Skills, Tools, MCP, TTS, Cron, Webhooks, and Connectors expose only behavior supported by cited goso APIs; missing Dewee features are unavailable/DI, not fake-live.
4. MCP and Connectors share a coherent inventory/configuration model; per-agent tool scope and connector assignment are clear.
5. TTS/MCP/Webhook credentials are write-only and public responses are shape-filtered; one-time secrets are not retained after copy/hide/navigation.
6. Destructive skill archive, cron delete, TTS clear, webhook rotate/revoke, and connector credential clear confirm a named target and show API-confirmed results.
7. No dependency install, paid vendor, MCP connect, webhook delivery, cron execution, or TTS success is claimed without factual evidence.
8. Vietnamese and English are complete.
9. `cd control-plane && npm test && npm run typecheck && npm run build` pass.
10. `GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh` and `./scripts/agpl-check-docs.sh` exit 0 before merge.
11. Delivery is merged to `main` with `--no-ff` after SPEC 122 passes CTO QC, then only Vite `:3000` is restarted. Never touch CRM `:8082`, sidecar `:8091`, or Dewee `:18791`.

## Worker delivery

Commit all implementation, i18n, tests, and a factual `docs/qa/123-capabilities-ux.md` to the worker branch. Report the branch commit, exact checks, live pages inspected, and remaining DI-only gaps. The Codex CTO will inspect the diff, run the merge gate, perform the `--no-ff` merge if acceptable, restart only `:3000`, and conduct live QC; a failed QC returns to a Grok follow-up worker.
