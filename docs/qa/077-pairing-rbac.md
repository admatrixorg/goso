# QA — SPEC 077 Pairing codes + RBAC matrix

Date: 2026-08-29. Clean-room Go/React. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not invent OAuth/SSO (DI-19 parked). Do not merge. Do not start SPEC 078.

Closes matrix **C9, N5**. Origin allowlist already 040/066.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Gateway token vs scoped keys; pairing as a distinct auth path; show-once secret | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/20-api-keys-auth.md` |
| Viewer / operator / admin role matrix (not OAuth) | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/23-ai-agent-permission-matrix.md` |

goso mapping (self-written): admin `POST /api/pairing` mints an 8-character one-time code (10-minute TTL, hashed at rest). `POST /api/pairing/exchange` `{code}` is an exact POST path (unauthenticated; the code is the secret) and returns the view grant **once**: `GOSO_VIEW_TOKEN` when set (`minted:false`, no token `expires_at` — standing env secret), otherwise a short-lived minted `gv_` token (`minted:true` + `expires_at`, same GET-only matrix). Control-plane Settings shows the code only in the create response. No QR vendor. No browser-device pairing. No OAuth/SSO.

## RBAC matrix (view-token GET-only)

| Route | Admin | View token / pairing grant |
|-------|-------|----------------------------|
| `GET /healthz` | 200 | 200 (bypass or GET allowlist) |
| `GET /api/agents`, `GET /api/sessions` (+ one id segment, `/v1` aliases) | 200 | 200 |
| `GET /api/sessions/{id}/messages` | 200 | **403** |
| `POST /api/chat` | 200 | **403** |
| `POST /api/pairing` | 201 | **403** |
| `POST /api/pairing/exchange` | 200 (code) | 200 (code; no Bearer) |
| `POST /api/system/backup` | 200 | **403** |
| `POST /api/kg/entities`, `POST /api/kg/relations` | 200/201 | **403** |
| `POST /api/skills` | 200/201 | **403** |
| `POST /api/agents/{id}/evolution/tick` | 200 | **403** |

Second exchange or expired code → 401. Demo `GOSO_ENV=demo` is unchanged (pairing store is in-memory; CP demo mode does not mint codes).

## What changed

- `gateway/internal/auth`: pairing store + `Require` accepts minted grants; view POST deny matrix tests.
- HTTP `POST /api/pairing`, `POST /api/pairing/exchange` (`/v1` aliases). Exchange is auth-bypass; generate is admin-only.
- Control-plane Settings → Pairing / Ghép máy: generate, show-once copy, i18n vi+en.
- QA matrix above. DI-19 stays parked.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 078.

## Proof

- Issue + exchange once, second exchange 401: `TestPairing_IssueExchangeOnce`, `TestPairing_HTTPExchangeOnce`, `TestPairing_ExchangeViewGrant`.
- TTL 10 minutes: `TestPairing_ExpiredAndUnknown`, `TestPairing_MintedGrantExpires`.
- Env view token still GET-ok after code TTL: `TestRequire_EnvViewAfterCodeExpiry`. Minted `expires_at` only: `TestPairing_HTTPMintedGrant`. Env exchange omits `expires_at`: `TestPairing_HTTPExchangeOnce`.
- Minted grant GET-only: `TestPairing_MintedGrantAccepts`, `TestRequire_MintedGrantGETOnly`, `TestPairing_HTTPMintedGrant`.
- Exchange bypass is exact POST path: `TestRequire_PairingExchangeExactPath`.
- View POST deny (backup, kg write, skills write, evolution tick): `TestRequireTokens_ViewPOSTDenyMatrix`, `TestViewToken_GETOnly`.
- View cannot generate pairing: `TestPairing_HTTPViewCannotCreate`.

## Non-goals

OAuth / SSO / Apple / Stripe (DI-19). QR vendor. Browser device pairing. Copying goclaw Go. Merge. SPEC 078. Binding/killing demo ports. Product secrets in git.
