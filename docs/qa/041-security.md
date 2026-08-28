# QA — SPEC 041 Security 5-layer + key encryption (SQLite tenancy light)

Date: 2026-08-27. Clean-room. Closes **S1–S9** as much as possible **without** Postgres (**N1** still THIẾU / **DI-09**). AES-256-GCM for **provider API keys at rest** if stored; env-only keys stay env. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. No goclaw copy. No product secrets in git. Tests use a **random** `GOSO_MASTER_KEY`.

## Layers

1. **Transport:** `crypto/subtle.ConstantTimeCompare` for Bearer (`security.Equal` / `auth.RequireTokens`). HTTP `MaxBytesReader` 1MiB on `/api`. WS `SetReadLimit` 512KiB. `GOSO_WS_ORIGINS` still allowlists Origin when set.
2. **Input:** scan user chat for four documented patterns: `ignore previous instructions`, `exfiltrate system prompt`, `drop table`, `dump credentials`. `GOSO_INJECTION=log|block` (default **log**). Block → **400** on `POST /api/chat`.
3. **Tool:** reject path `..` in vault/filesystem-like args; SSRF blocks literal localhost/private IPs on connector HTTP **when `GOSO_SSRF=1`** (default off so local fake e2e still works).
4. **Output:** `eventstore.Redact` token-shaped regex (`sk-` / `Bearer `). Untrusted tool output wrapped with `GOSO_UNTRUSTED_BEGIN` / `GOSO_UNTRUSTED_END` in messages sent back to the LLM.
5. **Isolation:** optional `GOSO_WORKSPACE`; tools/vault cannot write outside. Sandbox Docker = DI-12, skip.

## AES-256-GCM

Table `secrets(name, nonce, ct)`. `GOSO_MASTER_KEY` is 32-byte hex. Empty master key → refuse store; env providers still work.

## RBAC light

`GOSO_ADMIN_TOKEN` stays. If `GOSO_VIEW_TOKEN` is set, that token may only **GET** `/healthz` `/api/agents` `/api/sessions` — not POST chat (403).

## Commands

```
go test ./...
gofmt -l gateway desktop
go vet ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- Constant-time Bearer (`TestEqual_ConstantTimeCompare`, `TestRequireToken_ConstantTimeBearer`).
- View token GET-only; `GET .../messages` is 403 (`TestRequireTokens_ViewGETOnly`, `TestViewToken_GETOnly`).
- Next-turn untrusted wrap via `ToLLM` (`TestToLLM_RoundTripToolUse`, `TestChat_NextTurnKeepsUntrustedWrap`).
- SSRF redirect hook (`TestGuardClient_Redirect`).
- Workspace jail on absolute tool paths (`TestCallTool_WorkspaceAbsolute`).
- `/api` MaxBytesReader 1MiB (`TestMaxBytesReader_API`).
- WS 512KiB (`TestWS_ReadLimit`). Origin allowlist unchanged (`TestWS_OriginAllowlist`).
- Injection log vs block (`TestScanInjection_SixPatterns`, `TestInspectChat_LogAndBlock`, `TestChat_InjectionLogAllows`, `TestChat_InjectionBlock400`). Six exact strings: `docs/qa/079-injection-patterns.md`.
- Path `..` + workspace (`TestHasDotDotAndRejectPathArgs`, `TestConfine_Workspace`, `TestCallTool_RejectsDotDotPath`, `TestPut_WorkspaceAndDotDot`).
- SSRF default off / `GOSO_SSRF=1` (`TestCheckURL_DefaultOff`, `TestCheckURL_SSRFOn`, `TestHTTPTransport_SSRFBlocksLocalhost`).
- Untrusted wrap (`TestWrapUntrusted`, `TestTools_InvokeStoresRoleToolAndTrace`).
- Token-shape redact (`TestEventStore_TokenShapes`, `TestEventStore_NoCredentials`).
- AES-256-GCM random key + empty key refuse (`TestPutGet_RandomMasterKey`, `TestPut_EmptyMasterKeyRefuses`, `TestSQLiteRoundTrip`).

## Non-goals

Postgres `tenant_id` (DI-09), CRM SSO (DI-19), K8s, Docker sandbox (DI-12), product secrets in git, live paid APIs.
