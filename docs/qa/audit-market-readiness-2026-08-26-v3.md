# Market-readiness audit v3 — after feature-parity SPEC 026–029

Date: 2026-08-26. Compared to v2 (`docs/qa/audit-market-readiness-2026-08-26-v2.md`, headline **62%**).  
HEADs: goso **`9e42ccf`**, goso-crm **`8f25d52`**. Method: 5 Grok workers + coordinator QC + `go test` + curl fake CRM (SPEC 016). Server on `:8078` stopped after smoke.

**Overall: 70%** (was 62%). Equal-weight mean of axes = 69.5%; headline 70%. The only remaining **real** blocker is live Zalo QR against zca-js (operator credentials, not missing product code).

Parity table: `goso-crm/docs/qa/parity-upstream-2026-08-26.md` (pointer `docs/qa/parity-upstream-2026-08-26.md`).

## Axes

| Axis | v2 | v3 | Why |
|------|----|----|-----|
| Core data / API | 78 | **88** | Fake QR sessions pending→scanned→connected; `GET /api/settings/billing` is `usage_quota` (not developing); gateway `GET /api/quota` + POST `/api/chat` **429** when `GOSO_QUOTA_DAY` set. Live Zalo send still `not_configured`. |
| UI | 68 | **72** | `/ui/settings/billing` live (quota JSON, not “Đang phát triển” on the page card). Other settings placeholders stay developing (SSO, webhooks, …). |
| Reliability | 55 | **68** | `INSTALL=1` writes `vn.goso.crm-backup.plist`; `UNLOAD=1` removes it (verified with `HOME` override). `backup-bundle.sh` tars dump + `manifest.json`. `check-health` still logs `ALERT` on probe fail. |
| Security / compliance | 48 | **52** | SPEC 016 curl still 401 without `X-Org-Token`. QR routes 401 without token. No live OAuth; no TLS-in-process. |
| Deploy / ops | 58 | **65** | `SKIP_WAILS=1 ./scripts/package-desktop.sh` writes `dist/GOSO-darwin-arm64-unsigned.zip` + `README-UNSIGNED.md`. **N/A** Apple notarize / auto-update. Compose unchanged. |
| Content / i18n | 72 | **72** | vi/en unchanged this round. |

## Curl smoke (fake, PORT 8078, then stopped)

Env isolated (`cd /tmp`, no `.env`): `GOSOCRM_FAKE=1` `GOSOCRM_ORG_ID=org-smoke` `GOSOCRM_ORG_TOKEN=demo-org-token` `GOSOCRM_ADMIN_PASSWORD=demo-pass`.

| Probe | HTTP |
|-------|------|
| `/healthz` | 200 `{"status":"ok"}` |
| `/api/settings/users` no token | **401** |
| `/api/settings/users` + token + org | 200 `[]` |
| `/api/settings/billing` | 200 `status=ok` `kind=usage_quota` `stripe=false` `dailySendCap=0` |
| PUT quotas 42 then GET billing | 200 `dailySendCap=42` |
| `/api/settings/placeholders` | 200 `developing` |
| `POST /api/zalo/qr/sessions` no token | **401** |
| QR start → scan → confirm | 200 pending → scanned → connected (1×1 PNG data URL) |
| `POST /ui/login` username+password | **302** `Set-Cookie: goso_crm_session=…` |
| `GET /ui/settings/billing` cookie | 200, page contains `usage_quota`; “Đang phát triển” only on other nav placeholders |

Gateway QC (no extra listen): `go test ./gateway/...` OK including quota 429 tests.

`agpl-check.sh` **OK** (exit 0). `go test ./...` goso-crm **OK**. `make build` goso-crm **OK**.

## Workers this round

5 Grok workers on `run_390ddaca3a71` (all `worker_done` succeeded, then `worker-release`):

| SPEC | Repo | Dispatch | Commit | QC |
|------|------|----------|--------|----|
| 026 QR fake | goso-crm | `ctx_cabd49afe524` | `ca666ed` | node 15/15; `go test ./internal/zaloqr ./internal/zalo ./internal/httpapi`; agpl 0; mux +1 Mount |
| 027-crm billing | goso-crm | `ctx_177e547b3105` | `c53c64f` | `go test ./internal/orgset ./internal/httpapi`; agpl 0 |
| 028 backup INSTALL=1 | goso-crm | `ctx_e8f99b700b6d` | `2da0511` | `test-backup-install.sh` OK; agpl 0 |
| 027-gw quota 429 | goso | `ctx_eb82d3c9e304` | `8611bf5` | `go test ./gateway/...` |
| 029 unsigned zip | goso | `ctx_7c70e48a38a7` | `e305a17` | `SKIP_WAILS=1` zip lists README-UNSIGNED.md + STUB.txt; no codesign |

Merges `--no-ff` onto main: goso `8644d90` + `9e42ccf`; goso-crm `9aa2d0e` + `3448421` + `8f25d52`. No mux conflicts.

## Blockers remaining (real only)

1. **Zalo QR live-test** — fake state machine is the shippable AC. Live `zca-js` `loginQR` is **not implemented** and needs an operator Zalo account + credentials outside this repo (`docs/qa/026-zalo-qr.md`). No app OAuth client in git.

Dropped from blockers (upstream **N/A** or rewritten this round):

- SaaS Stripe / paid plan — ZaloCRM `Payment` is customer invoices, not org subscription. Billing page is `usage_quota`, `stripe=false`.
- Apple notarize / Developer ID — **N/A**; unsigned zip is the deliverable.
- Auto-update — **N/A** (needs a signed channel).
- Facebook OAuth — **N/A** this round (out of GOSO CRM Zalo scope).
- OpenAI OAuth — goclaw LLM login, not CRM.
- K8s / Grafana — **N/A** (compose only).

## Non-goals still true

No copy/vendor from ZaloCRM or goclaw-source. Author identifiers outside `.planning` empty (`agpl-check.sh` exit 0).
