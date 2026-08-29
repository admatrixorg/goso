# SPEC 113 — API Keys

> After 112. Clean-room React/Go. Do **not** copy GoClaw/ZaloCRM. i18n vi+en. Do not bind/kill `:8082` `:8091`. `:3000` may restart. No invent live vendor tokens.

Queue: `.planning/specs/090-sidebar-ux-queue.md` row 113. Audit: `docs/qa/090-goclaw-sidebar-ux.md` API Keys. **THIẾU**.

## Goal

Add **gateway API-key issuance, masked inventory, usage, expiry, fine-grained scopes, and revocation**. Reveal a full key exactly once at creation, store/return only hash plus safe prefix metadata, support copy acknowledgment, prohibit later retrieval, and audit create/revoke without logging the secret.

## AC

- [ ] Live nav tab + page. Masked inventory, loading/empty/error.
- [ ] Create reveals full key once. GET never returns the secret (hash + prefix only). Revoke with confirm. Audit create/revoke without logging the secret.
- [ ] i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/113-api-keys.md`.

## Out of scope

Packages (114). Approvals (115). Copying GoClaw chrome. Live vendor tokens. Tenants (112) already merged.
