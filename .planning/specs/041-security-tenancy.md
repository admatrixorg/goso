# SPEC 041 — Security 5-layer + key encryption (SQLite tenancy light)

> LOCKED: 2026-08-27. Closes **S1–S9** as much as possible **without** Postgres (N1 = still THIẾU / DI-09). AES-256-GCM for **provider API keys at rest** if stored; env-only keys stay env.
> Demos untouched. No goclaw copy.

## Layers

1. **Transport:** `crypto/subtle.ConstantTimeCompare` for Bearer; HTTP `MaxBytesReader` 1MB on `/api`; WS 512KiB read limit. If `GOSO_WS_ORIGINS` set, check Origin.
2. **Input:** scan user chat text for ≥4 documented injection patterns (e.g. “ignore previous instructions”, “exfiltrate system prompt”, “drop table”, credential-dump phrasing). Action: `GOSO_INJECTION=log|block` default **log** (test both). Block → 400 on `/api/chat`.
3. **Tool:** reject path `..` in vault/filesystem args; SSRF: block literal localhost/private IPs on connector HTTP **when `GOSO_SSRF=1`** (default off so local fake e2e still works; tests enable it).
4. **Output:** extend `eventstore.Redact` with simple token-shaped regex (sk-/Bearer ) already-ish; wrap untrusted tool output with a marker string `GOSO_UNTRUSTED_BEGIN` / `_END` in messages sent back to LLM (test).
5. **Isolation:** optional `GOSO_WORKSPACE` dir; tools/vault cannot write outside. Sandbox Docker = DI-12, skip.

## AES-256-GCM

If we persist connector `credential_ref` secrets later — for 041: encrypt a **provider key blob** table `secrets(name, nonce, ct)` using `GOSO_MASTER_KEY` (32-byte hex). Tests use random key. Empty master key → refuse store, env providers still work.

## RBAC light

Optional `GOSO_ADMIN_TOKEN` stays. If `GOSO_VIEW_TOKEN` set, that token may only GET `/healthz` `/api/agents` `/api/sessions` not POST chat. Test. Not full admin/operator/viewer matrix.

## Non-goals

Postgres `tenant_id` (DI-09), CRM SSO (DI-19), K8s.

## QC

`go test ./...`, build, agpl 0, `docs/qa/041-security.md`. Constant-time compare test. Commit, do not merge.
