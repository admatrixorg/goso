# SPEC 069 — Health chrome + polish + CTO-09

> LOCKED after 068. Clean-room. Do not kill `:8082` `:8091`.

Closes **CTO-08** (static green pill), **CTO-11** (English leftovers if still in vi UI), **CTO-09** (banned author ids in `docs/qa` + agpl-check scan).

## GoClaw (cite)

- Dashboard/login + live gateway health as operational truth (`docs/23-multi-tenant-architecture.md` Mode 1 dashboard; `docs/18-http-api.md` system endpoints).
- Not a copy of their chrome — goso CP already exists.

## goso today

- `control-plane/src/App.tsx` paints a pulsing green gateway status without calling `api.health` (CTO-08).
- `GET /healthz` is live JSON (`audit-cto-2026-08-28.md` proof).
- `docs/qa/014-qa.md` still contains banned identifiers `minhhaiphan` / `locphamnguyen` outside `.planning`; current agpl-check misses docs/qa.

## goso plan

1. **Chrome:** probe `/healthz` on load + interval (backoff 2s→15s). States: connected / degraded (non-200 body) / offline (network) / unauthorized (401/403). Do **not** stay green when gateway is down. i18n vi+en.
2. **Polish:** remaining English placeholders in vi locale (CTO-11) — grep `control-plane/src/i18n/vi.ts` for untranslated English user-visible strings; fix only real leftovers.
3. **CTO-09:** replace banned author ids in `docs/qa/*.md` with `upstream-author` or delete the quoted names. Extend `goso-crm/scripts/agpl-check.sh` **or** add `goso/scripts/agpl-check-docs.sh` invoked in QC so `docs/qa` is scanned. Product Go stays clean. Do not put secrets in docs.

## Tests

- Component or API client test: mock fetch `/healthz` 200 vs fail vs 401 maps to three labels (if no component test harness, a small pure function `healthKind(status, ok)` + unit test is enough).
- `rg -n 'minhhaiphan|locphamnguyen' docs/qa` empty.
- agpl-check (extended) exit 0.

QC: typecheck, go test, build, agpl 0, `docs/qa/069-health-chrome.md`. Commit `admatrixmdp/spec069-health-chrome`. Do not merge.
