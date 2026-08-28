# SPEC 077 — Pairing codes + RBAC matrix

> After 076. Clean-room. Do not kill `:8082` `:8091`. Live IdP = DI-19 parked.

Closes **C9, N5**. Origin allowlist already 040/066.

## GoClaw cite

`docs/20-api-keys-auth.md`, `docs/23-ai-agent-permission-matrix.md` — pairing / role matrix (read-only docs).

## goso plan

1. Pairing: generate a one-time code (admin) bound to `GOSO_VIEW_TOKEN` or a short-lived view grant; exchange once for view token. Expire 10 minutes. Tests only — no QR vendor.
2. RBAC: keep admin vs view-token GET-only; add an explicit matrix table in QA + `auth` tests for POST deny on view-token for new routes (backup, kg write, skills write, evolution tick). Do not invent OAuth.
3. CP: pairing display once; i18n.

Non-goals: SSO, Apple, Stripe. Commit `admatrixmdp/spec077-pairing-rbac`.
