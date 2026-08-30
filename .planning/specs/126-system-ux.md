# SPEC 126 — SYSTEM operator UI/UX

Status: queued after SPEC 125 CTO/advisor QC
Owner: Grok implementation; Codex CTO architecture, merge, and live QC (advisor live-QC while Codex credits exhausted)
Benchmark method: clean-room behavioral inspection of live Dewee only

## Goal

Make goso's SYSTEM pages answer the same operator questions and cover the same operational states as live Dewee without copying pixels, wording, CSS, components, or source. Scope is Tenants, Providers, API Keys, Packages, Config (`settings` tab), Approvals, and Import & Export. Backup/restore remains a Settings-bounded panel (SPEC 117); do not promote it into a seventh independent SYSTEM nav item. Reuse the accepted PageChrome / PageStatus / `classifyPageState` / `inventoryBlocksMutation` pattern from SPEC 120–125. Preserve write-only secrets, one-time key reveal, public-shape filtering, and named destructive confirmation.

## Live surfaces and APIs

| Surface | Live Dewee behavior route | Live goso entry/page | Existing goso APIs |
| --- | --- | --- | --- |
| Tenants | `http://127.0.0.1:18791/tenants` | App tab `tenants`, `TenantsPage.tsx` | list/get/create/status/members `/api/tenants*` via `api/tenants.ts`, `api/tenants-ops.ts` |
| Providers | `http://127.0.0.1:18791/providers` | App tab `providers`, `ProvidersPage.tsx` | list/create/patch/key/test `/api/providers*` via `api/providers.ts`, `api/provider-ops.ts` |
| API Keys | `http://127.0.0.1:18791/api-keys` | App tab `apikeys`, `ApiKeysPage.tsx` | list/get/create/revoke `/api/api-keys*` via `api/apikeys.ts`, `api/apikeys-ops.ts` |
| Packages | `http://127.0.0.1:18791/packages` | App tab `packages`, `PackagesPage.tsx` | snapshot/allow/unpin/install/uninstall/recover/cli `/api/packages*` via `api/packages.ts`, `api/packages-ops.ts` |
| Config | `http://127.0.0.1:18791/config` (also `/settings`) | App tab `settings`, `SettingsPage.tsx` | gateway GET/PUT `/api/config`; CRM users/roles/nicks/quotas/templates/account via `api/settings.ts`, `api/settings-ops.ts` |
| Approvals | `http://127.0.0.1:18791/approvals` | App tab `approvals`, `ApprovalsPage.tsx` | list/get/decision `/api/approvals*` via `api/approvals.ts`, `api/approvals-ops.ts` |
| Import & Export | `http://127.0.0.1:18791/import-export` | App tab `impexp`, `ImportExportPage.tsx` | catalog/job/export/preview/import/rollback `/api/import-export*` via `api/impexp.ts`, `api/impexp-ops.ts` |

Do not install packages against production hosts, call paid providers, reveal stored API keys after creation, include secrets in portable archives, or claim SSO/K8s/Stripe success. Test/install/import remain disabled or DI-labeled unless the real local dependency is configured. Never add a live-looking action for a Dewee behavior that goso's cited APIs do not support. Never touch CRM `:8082` or sidecar `:8091`. Settings CRM subsections may call `/crm-api` through the existing Vite proxy only.

## Constraints

- Reuse PageChrome / PageStatus / classifyPageState / inventoryBlocksMutation. Do not fork a second incompatible pattern.
- Error or permission must never render simultaneous zero-count empty claims or enabled mutation forms.
- Preserve last-known data only when clearly labeled stale with last successful refresh time.
- Credential invariant: GET only `*_set`, prefix, source, environment ownership. Inputs start empty. One-time secrets leave UI memory after copy/hide/navigation. Never render, log, screenshot, fixture, or write to QA a token, API key, CLI cred, archive secret, or raw vendor error that could contain one.
- Complete i18n in Vietnamese and English.
- Worker does not merge and does not restart Vite `:3000`. Never touch CRM `:8082`, sidecar `:8091`, or Dewee `:18791`.

## Non-goals

- Copying Dewee pixels, wording, CSS, components, or source.
- Promoting backup into a seventh SYSTEM nav item.
- Inventing paid provider, package-registry, SSO, Stripe, K8s, or import-secret success.
- Mixing Nodes/Channels pairing into Approvals.
- Merge and Vite restart (Codex CTO / advisor live QC).

## Acceptance criteria

1. All seven SYSTEM nav entries open with consistent page chrome, primary action, refresh, and filters where relevant. Backup remains Settings-bounded.
2. Loading, true empty, filtered empty, generic error, permission, partial dependency failure, not-configured/DI, and stale are independently testable and never contradict one another.
3. Tenants/Providers/API Keys/Packages/Approvals/Import-Export expose only cited goso APIs; missing Dewee features are unavailable/DI, not fake-live.
4. Provider keys, API keys, package CLI creds, and import archives follow write-only / one-time / public-shape contracts.
5. Settings distinguishes CRM vs gateway vs backup provenance; env-owned values are explicit; no token hydration.
6. Approvals remain distinct from channel/device pairing; decisions are single-resolution and redacted.
7. No paid provider, package-registry, SSO, Stripe, K8s, or import-secret success is claimed without factual configured evidence.
8. Vietnamese and English are complete.
9. `cd control-plane && npm test && npm run typecheck && npm run build` pass.
10. `GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh` and `./scripts/agpl-check-docs.sh` exit 0 before merge.
11. Delivery is merged to `main` with `--no-ff` after SPEC 125 passes live QC, then only Vite `:3000` is restarted.
