# SPEC 065 — Form validation sweep (live tabs)

> LOCKED: 2026-08-28. Clean-room React. No ZaloCRM / goclaw copy. No banned author ids.
> Do **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.

Several live create handlers `return` without StatusLine when required fields are empty (Teams name, Vault title, Memory note, Connectors name/endpoint, Marketing audience/campaign, Events filters are OK).

## UI

For each live page that POSTs/PUTs:

- Trim required fields. If missing → `StatusLine` kind=error with i18n key (`*.needName` etc.). No network call.
- Keep existing server errors via `formatPublicError`.
- Do not change DEMO mock pages except they must not regress typecheck.
- Reuse patterns from 057/058/062 if already merged (do not duplicate keys).

Pages in scope: `TeamsPage`, `VaultPage`, `MemoryPage`, `Connectors.tsx`, `MarketingPage` (create audience/campaign). Skip Providers (056) if already validating.

`docs/qa/065-form-validation.md` table page | field | key. Commit `admatrixmdp/spec065-form-validation`. Do not merge.

## QC

`cd control-plane && npm run typecheck` · agpl-check 0.

## Non-goals

Schema library, HTML5-only without StatusLine, new APIs.
