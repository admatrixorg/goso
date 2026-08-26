# Market-readiness audit v2 — after SPEC 019–025

Date: 2026-08-26. Compared to v1 (`docs/qa/audit-market-readiness-2026-08-26.md`, headline **38%**).  
HEADs: goso **`480bbdf`**, goso-crm **`6ecf946`**. Method: code + worker QC + curl fake CRM (token SPEC 016). Servers stopped.

**Overall: 62%** (was 38%). Equal-weight mean of axes = 63%; headline 62% because Zalo login and paid billing remain blocked.

## Axes

| Axis | v1 | v2 | Why |
|------|----|----|-----|
| Core data / API | 55 | **78** | Settings 6 CRUDs, marketing audiences/campaigns, heatmap buckets, meeting sources persist. Curl 200 with `X-Org-Token`; 401 without. Still no Zalo send, no payment capture. |
| UI | 35 | **68** | DEMO tabs hidden unless `VITE_DEMO_MODE`. CP Settings 6 live + placeholders; Marketing 7 tabs; Heatmap page. goso-crm Go templates settings/marketing/heatmap + i18n. Remaining of 13 settings still “developing”. |
| Reliability | 40 | **55** | launchd/cron wrapper around pg_dump + check-health ALERT. Restore loop was SPEC 018 (7=7). INSTALL=1 not run on this host. |
| Security / compliance | 28 | **48** | SPEC 016 still holds (curl 401). No OAuth, no TLS-in-process, no RBAC beyond org token. |
| Deploy / ops | 52 | **58** | Unsigned Wails desktop (`make -C desktop verify` green). Docker compose unchanged. No K8s. |
| Content / i18n | 42 | **72** | vi/en maps in CP (`src/i18n`) and CRM (`web/locales`). Default vi. Not 20 locales. |

## Curl smoke (fake, PORT 8077, then stopped)

| Probe | HTTP |
|-------|------|
| `/api/metrics` | 200 |
| `/api/settings/users` + token | 200 `[]` |
| `/api/settings/users` no token | **401** |
| `/api/settings/billing` | 200 `{"status":"developing"}` |
| `/api/marketing/overview` | 200 7 kind zeros |
| `/api/crm/heatmap` | 200 empty buckets grain=day |
| `/api/settings/sources` | 200 `[]` |
| POST `/ui/login` | 302 |
| GET `/ui/settings` cookie | 200 |

`agpl-check.sh` **OK**. `go test ./...` goso-crm **OK**.

## Blockers now (3)

1. **Zalo OAuth / real nick connect** — nicks and meeting sources are operator-declared rows; no QR/OAuth (non-goal / blocked on app credentials).
2. **Paid billing** — `/api/settings/billing` is explicitly `developing`. No Stripe/money gateway.
3. **Production hardening leftover** — desktop unsigned; no K8s; no Prometheus/Grafana (all listed non-goals). Backup schedule is dry-run unless `INSTALL=1`.

## Workers this run

8 Grok workers on `run_8e4c78e814c3`. Lot 1: 020, 021, 022, 025 (goso-crm) + 023 mcp + 024 desktop. Lot 2: 019 CRM web + 019/020-UI control-plane.
