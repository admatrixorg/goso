# Market-readiness audit — GOSO family

Date: 2026-08-26  
Method: skill `product-description` (draft from code + tests + SPEC 001–015, verify against a running bounded demo, triage). This file is the audit deliverable — not a full product-description repo.  
Auditor: read-only of product source. No product-code edits. `bin/goso-crm` was rebuilt from HEAD so the process matched current templates (previous binary dated 2026-08-22 predated Vietnamese UI). Servers stopped after curl.

| Repo | Path | HEAD (short) | Subject |
|------|------|----------------|---------|
| goso (core + control-plane) | `/Users/mqglobal/Documents/goclaw-binary/goso` | `a0c2ee3` | Merge orca hygiene runbook pointer |
| goso-crm | `/Users/mqglobal/Documents/goclaw-binary/goso-crm` | `db9e8bc` | Merge orca hygiene runbook |

**Overall market-ready: 38%.**

Equal-weight mean of the six axes below is 42%. The headline is **38%** because the missing pieces are customer-facing launch blockers (CRM HTTP has no auth, most control-plane screens are DEMO, settings/marketing/heatmap are absent). This is a working internal demo + clean-room CRM slice, not a product a paying org can run as their CRM.

---

## How this audit was done

Skill `~/.agents/skills/product-description/SKILL.md` (session opened before the skill was installed, so the files were read directly). Product kind: **web app — page/form lifecycle**. Adapted phases:

1. **Scope.** Surface = GOSO family (goso gateway + control-plane + goso-crm HTTP/UI). Out of scope: sibling ZaloCRM AGPL source, production SaaS tenancy, K8s.
2. **Draft from code/tests/docs.** Control-plane pages + `src/demo/mock.ts`, gateway `httpapi`, goso-crm `internal/httpapi` mux, SPECs `goso/.planning/specs/001`–`014` and `goso-crm/.planning/specs/015`, QA under `docs/qa/`.
3. **Verify running product (this pass).** Bounded fake demo only (see evidence). Gateway, live Postgres, and `docker compose` were **not** started here; live T18 / compose evidence is cited from existing QA with dates, not re-claimed as this curl.
4. **Triage.** Three buckets: (a) real, (b) DEMO mock, (c) missing for market. Then six-axis scores and three blockers.

Unit of interaction used for triage (from skill `product-kinds.md`): arrive on a page → leave untouched / begin using → while using → submit or persist. DEMO pages never reach a durable finish. Live KPI pages do (HTTP GET of metrics).

---

## Score by axis

| Axis | % | Why this number |
|------|---|-----------------|
| Core data / API | **55** | goso-crm exposes real JSON for health, metrics, advisor, drafts, suggestions, events, with tenant `X-Org-ID`. Domain + sqlc tenant-scope + EventStore + scoring + nudge exist and are tested. Gateway has agents/sessions/chat/connectors/events/usage in code. T18 previously proved live aggregates from `daily_message_stats` + `sales_kpis` on org test-a. Missing a sellable CRM: contact/deal/pipeline/marketing APIs, heatmap, real Zalo identity persist. Fake mode returns zeros (verified this pass). |
| UI | **35** | ZAgent chrome is real (14 control-plane tabs + 2 goso-crm Go pages). Functionally, Home / Việc / Họp / Bạn bè / Lịch / Kho ảnh / Marketing / Settings-nguồn are `demo/mock.ts` with a DEMO badge. Settings is 3 of 13 CRM pages. Marketing is 1 placeholder of 8 labels (clicks do not switch tabs). Heatmap does not exist. Agents/Sessions/Chat/Connectors/Events/CRM-metrics talk HTTP, but this pass did not run the gateway so those five tabs were not live-exercised. Desktop is a Wails skeleton (SPEC 009 AC still open in the spec file). |
| Reliability | **40** | `/healthz` + `/readyz` work; `make verify`, e2e scripts, docker healthchecks, dual fake/live. Fake stores are in-memory (process exit = data gone). Gateway traces/stats are in-memory ring buffers (SPEC 008 non-goal: no OTel/Prometheus/Grafana). No CRM Postgres backup/restore, no alerting, no SLO. |
| Security / compliance | **28** | Tenant header on CRM; gateway Bearer + IP rate-limit exist **when** `GOSO_ADMIN_TOKEN` is set (empty = dev mode, pass-through). **goso-crm has no authentication** — knowing an org UUID is enough to read metrics. No OAuth/RBAC/TLS-by-default. AGPL clean-room + `agpl-check` is a process control, not product security. SPEC 013 is gitleaks/semgrep/runbook, not a pentest (explicit non-goal). |
| Deploy / ops | **52** | goso: `Dockerfile`, `compose.yml`, `compose.prod.yml`, `docs/DEPLOY.md`, SQLite backup sidecar in prod overlay + runbook. goso-crm: multi-stage Docker + compose (app, sidecar, postgres, redis, minio) documented and previously brought up (`docs/qa/015-deploy.md`, 2026-08-22). No K8s, no CI deploy, no CRM DB backup job, no prod monitoring overlay. |
| Content / i18n | **42** | Control-plane `lang="vi"` and Vietnamese nav/labels; goso-crm templates after rebuild: title **Tổng quan — GOSO CRM**, brand **ZAgent**, **chế độ giả**, KPI labels in Vietnamese (`015-ui-vi.md`). No i18n framework, no locale files, no English pack, leftover operator English (`fake in-memory`, `next_action`). Not “đầy đủ i18n”. |

Unweighted mean: (55+35+40+28+52+42)/6 = **42%**. Headline **38%** after treating security + incomplete UI as launch gates.

---

## This-pass verification (bounded demo)

Commands (then **servers stopped**):

```bash
cd /Users/mqglobal/Documents/goclaw-binary/goso-crm
make build    # existing bin was 2026-08-22; rebuilt 2026-08-26 12:32 so UI matched HEAD
GOSOCRM_FAKE=1 PORT=8089 ./bin/goso-crm
# log: goso-crm listening on :8089 fake=true

cd /Users/mqglobal/Documents/goclaw-binary/goso/control-plane
# dist/ already present (vite preview); not rebuilt
npm run preview -- --host 127.0.0.1 --port 3000
# Local: http://127.0.0.1:3000/
```

Org header used: test-a `01a01fe5-704c-7375-aa1f-6e50a9d0296d`.

| Probe | HTTP | Body / note |
|-------|------|-------------|
| `GET :8089/healthz` | 200 | `{"status":"ok"}` |
| `GET :8089/readyz` | 200 | `{"status":"ok","fake":true}` |
| `GET :8089/ui/dashboard` | 200 | title **Tổng quan — GOSO CRM**; nav ZAgent; **chế độ giả**; KPI tiles CHƯA REP / TIN GỬI / PHẢN HỒI / THỜI GIAN TB / KPI / DOANH THU all **0**; copy “Thiếu org: set X-Org-ID hoặc ?orgId= · fake in-memory” |
| `GET :8089/ui/crm` | 200 | title **Tin nhắn — GOSO CRM**; **ƯU TIÊN** / **TƯƠNG TÁC**; 0 hội thoại |
| `GET :8089/api/crm/metrics` `X-Org-ID: test-a` | 200 | zeros, `sampleDays=0`, window `2026-08-20` → `2026-08-26` (in-memory metrics source, **not** live DB) |
| `GET :8089/api/crm/advisor` same header | 200 | `[]` |
| `GET :8089/api/suggestions?kind=next_action` | 200 | `[]` |
| `GET :8089/api/events?connector=ZaloCRM` | 200 | `[]` |
| `GET :8089/api/crm/ai/drafts` | 200 | `[]` |
| `GET :8089/api/crm/metrics` **no** `X-Org-ID` | 400 | `{"error":"org id is required"}` |
| `GET :3000/` | 200 | `<title>ZAgent — GOSO Control Plane</title>`, `lang="vi"`, assets 200 |
| Bundled CP JS `index-jnNL9n21.js` | — | `DEMO` appears 8 times; mock fixture “Vinh Phát” / “Dat Nguyen Ai” present; also `crm/metrics` + `healthz` (live client, not mock) |

**Stopped** `:8089` and `:3000` after the curls. `lsof` confirmed both ports free.

### Not started this pass (do not treat as live-now)

| Surface | Why skipped | Prior evidence (docs, not this curl) |
|---------|-------------|--------------------------------------|
| Gateway (`bin/goso-gateway`) | User scoped demo to goso-crm fake + CP preview | Agents/Sessions/Chat/Connectors/Events clients exist in `control-plane/src/api/client.ts` |
| `GOSOCRM_FAKE=0` live Postgres | Bounded fake only | `goso-crm/docs/qa/015-t18-live-seed.md` (2026-08-22): test-a after seed `sampleDays=7`, `messagesSent=101`, `messagesReceived=72`, `unreplied=13`, `avgResponseTime=56`, `kpiCompletionRate=72`, `revenueMonth=15000000`; Master read-only zeros on empty `daily_message_stats` |
| `docker compose` goso-crm | Host binary used | `docs/qa/015-deploy.md` (2026-08-22): compose up, `/healthz` 200, app healthy on 8089 |

---

## Feature inventory — (a) real / (b) DEMO mock / (c) missing

Classification rule from the skill + the user: **real** = API or DB path that exists in code, has tests, and either this curl or a dated QA live run proved it. **DEMO** = anything sourced from `control-plane/src/demo/mock.ts` or a page that always shows `DemoBadge` / “không ghi DB”. **Missing** = named market surface with no durable implementation.

### (a) Real (verified in code ± this pass ± dated live QA)

| Feature | Where | Evidence |
|---------|--------|----------|
| Liveness / readiness | goso-crm mux | This pass: `/healthz` 200, `/readyz` `fake=true` |
| Tenant org header | goso-crm | 400 without `X-Org-ID`; metrics echo orgId |
| CRM metrics window | `GET /api/crm/metrics` | Fake zeros this pass; T18 live non-zero on test-a (QA 2026-08-22) from `daily_message_stats` + `sales_kpis` |
| Advisor | `GET /api/crm/advisor` | 200 `[]` fake; unit tests `internal/advisor`; T18 live path documented |
| AI drafts + approval gate | `GET/POST /api/crm/ai/drafts…` | mux + `internal/approval`; AC-02 tests |
| Sale nudge suggestions | `GET /api/suggestions` | mux + `internal/nudge`; AC-03 |
| EventStore query | `GET /api/events` | mux + `internal/eventstore`; AC-05 |
| Scoring gate | `internal/scoring` | tests AC-04 (no extra HTTP this pass) |
| Domain + sqlc tenant-scope | `internal/domain`, `db/` | AC-01; goose additive; Prisma tables not owned |
| Zalo sidecar protocol (fake) | `sidecar-zalo`, `internal/zalo` | AC-07 sanitize; `ZALO_FAKE=1` |
| Go UI dashboard + tin nhắn | `/ui/dashboard`, `/ui/crm` | this pass, Vietnamese after rebuild |
| Seed org test-a | `scripts/seed-test-a-demo.sql` | T18; default `VITE_GOSOCRM_ORG_ID` / `CRM_ORG_DEFAULT` |
| Docker deploy goso-crm | `Dockerfile`, `docker-compose.yml` | 015-deploy QA 2026-08-22 |
| Control-plane CRM metrics tab | `pages/CrmMetrics.tsx` + `api/crm.ts` | HTTP client, 3s timeout, no goso-crm Go import; needs CRM up (this pass CRM was up; SPA itself not browser-clicked) |
| Gateway agents / sessions / chat | `gateway/internal/httpapi` | code + tests; **not curled this pass** (gateway not started) |
| Connector registry persist (gateway SQLite) | SPEC 014, `POST/GET /api/connectors` | code; CP Connectors page calls it; persist is gateway-side, not “nguồn họp” |
| Gateway auth + rate limit | `internal/auth`, `internal/ratelimit` | wired when `GOSO_ADMIN_TOKEN` set; empty = dev mode |
| Observe stats/traces (in-memory) | SPEC 008 | `/api/stats` or traces buffer — not production monitoring |
| Usage metering stub | `GET /api/usage` | code present; SPEC 010 AC boxes still unchecked in the spec file |
| Channel webhooks (telegram / zalo-oa / zalo-personal) | SPEC 003–004 | adapters in `httpapi/handlers.go`; Zalo login/QR is a documented non-goal |
| goso Docker + SQLite backup runbook | SPEC 012/013 | `compose.yml`, `docs/RUNBOOK.md` |
| AGPL process | `scripts/agpl-check.sh`, CI | process, not a user feature |
| Control-plane theme toggle | Settings “Giao diện” | local `document.body.dark` — real UI state, not a backend |

### (b) DEMO mock (`src/demo/mock.ts` — always badge DEMO, not live data)

File header: “Wireframe-shaped DEMO fixtures. Not live CRM.” `DemoBadge` = warning badge **DEMO**.

| Fixture / screen | What the user sees | Persist? |
|------------------|--------------------|----------|
| `meetSources` | Meet/Zoom “Đã kết nối”, Ghi âm “Chưa bật” | No — Settings says “chưa persist” |
| `recentMeetings` / `allMeetings` | Họp Vinh Phát, Hòa An, An Tâm, … | No |
| `inbox`, `weekStats`, `agentChips` | Home inbox + weekly numbers + chip prompts | No |
| `taskKpis` (CHƯA REP=1, KH CỦA TÔI=461, …) | Việc của tôi KPI | No — **conflicts with live CRM unreplied** if both shown |
| `taskTimeline` | 09:12 duyệt đề xuất, … | No |
| `friends` (Dat Nguyen Ai, Phan Duy, …) | Bạn bè; copy “không gọi Zalo” | No |
| `mkMenu` (8 labels) | Marketing sidebar; only “Tệp khách hàng” pane; “Tạo tệp không ghi DB” | No |
| HomePage, MeetingsPage, TasksPage, FriendsPage, CalendarPage (0 lịch), GalleryPage (empty), MarketingPage, Settings sources | All `DemoBadge` | No |
| CP top “Gateway” green dot | Static chrome, not a `/healthz` probe of the gateway | No |
| CP search / bell / avatar “G” | Chrome only | No |
| goso-crm fake in-memory metrics/advisor/drafts/events | Real HTTP shape, **fake zeros** — honest `fake:true` on `/readyz` | Process memory only |

Anything in `mock.ts` must never be presented as customer data.

### (c) Missing for market (named gaps)

| Gap | Current state | Why it blocks market |
|-----|---------------|----------------------|
| **13 CRM settings pages** | 3 items: Nguồn (mock), Tài khoản (env copy), Giao diện (theme). QA `014-ui-zagent-screens.md` explicitly did not build the 13. | Cannot configure a real org (users, roles, nicks, quotas, templates, billing, …). |
| **7 marketing tabs** | 8 labels in `mkMenu`; only first pane; buttons do not switch; no marketing API. | Cannot run campaigns. |
| **Heatmap** | `rg heatmap` in control-plane src = none. Wireframe “Báo cáo 7 tab (KPI/funnel/heatmap)” reduced to 6 KPI cards. | Reporting product incomplete. |
| **Connector / nguồn thật persist** | Gateway connectors persist in SQLite **if gateway runs**. Meeting sources (Meet/Zoom/file) and Zalo nick identity in the CRM UI do **not**. Sidecar default `ZALO_FAKE=1`. T18 seed is synthetic `goso-t18-demo-*`, not customer onboarding. | A customer cannot connect their real Zalo/Meet and keep it. |
| **i18n đầy đủ** | Hardcoded Vietnamese + leftover English. No locale packs. | Cannot ship EN or mixed teams. |
| **Auth / security hardening** | CRM: no auth. Gateway: optional Bearer, default off. No RBAC, session expiry UX, TLS mandate. SPEC 013 non-goal: pentest. | Cannot expose to the internet or a second tenant safely. |
| **Backup / recovery** | goso SQLite runbook exists. goso-crm Postgres: no automated backup/restore tested as a product path. Fake mode: no backup by design. | Data-loss risk for paid CRM. |
| **Monitoring** | healthz/readyz + in-memory traces. No Prometheus/Grafana/alerting. SPEC 008 non-goal. | Cannot operate 24/7. |

Also not market-complete (secondary): desktop installer/code-sign (SPEC 009), real Telegram/Zalo send loops, Stripe/quota enforcement (SPEC 010 non-goal), K8s.

---

## SPEC 001–015 vs market (one line each)

Locked SPECs describe an MVP platform, not a full CRM. Checked AC in the spec file ≠ “sold”.

| SPEC | Intent | Market read |
|------|--------|-------------|
| 001 Harness | Gateway skeleton | Done as skeleton |
| 002 HTTP session | Agents/sessions/chat | Code present; not curled this pass |
| 003 LLM + Telegram | Real LLM + webhook stub | Code; live LLM depends on keys; Telegram stub |
| 004 Zalo channels | OA + Personal webhooks | Adapters; QR/login non-goal |
| 005 DB persist | SQLite | Gateway only; CRM uses Prisma-owned Postgres read |
| 006 Auth + rate limit | Bearer + 60/min | Implemented, **opt-in**; CRM not covered |
| 007 Control plane | Admin stub | Far past stub visually; most new screens DEMO |
| 008 Observability | JSON log + in-memory traces | Not production monitoring |
| 009 Desktop | Wails skeleton | Skeleton; AC open in spec |
| 010 Billing quota | Metering stub | `/api/usage` in code; no charging |
| 011 MCP rebrand | goso-mcp | Package exists; not a CRM UI |
| 012 Deploy | Docker compose | Present for both repos |
| 013 Hardening | gitleaks/semgrep/runbook | Process, not product lock-down |
| 014 Connector | Registry, ZaloCRM first connector | HTTP/MCP registry real; first connector is a URL, not a sold Zalo login |
| 015 goso-crm T01–T18 | Go AI-CRM slice | Domain+API+fake/live+seed+docker **done**; not a full CRM product |

---

## Top 3 BLOCKERS before market

1. **CRM HTTP is unauthenticated.** `X-Org-ID` is a tenant selector, not a credential. This pass: any client that sends test-a’s UUID gets 200 metrics. Gateway Bearer does not wrap goso-crm. Until org auth (token/session/mTLS) exists, this cannot face a network that is not a laptop loopback.

2. **The product the wireframes sell is still DEMO.** 13 settings pages → 3 stubs; 7 marketing tabs → 1 empty pane; heatmap absent; Home/Việc/Họp/Bạn bè/Lịch/Kho ảnh/Nguồn họp are `mock.ts`. A buyer clicking those screens sees numbers (461 KH, họp Vinh Phát) that are fiction. Shipping that as a CRM is a trust failure even with badges.

3. **Cannot operate or onboard a real customer.** No CRM backup/restore; no production monitoring/alerting; meeting sources and Zalo nicks do not persist; live path still depends on OrbStack Prisma schema + synthetic seed, not a first-run wizard. Docker compose is a lab, not a paid tenant.

Fix order: (1) auth on CRM + stop default-open gateway, (2) either hide DEMO tabs from any non-demo build or replace them with live APIs, (3) backup + health dashboard + real connector onboarding.

---

## Open questions (skill verification footer)

- Gateway `/healthz`, `/api/agents`, connector persist were not curled this pass (gateway not started). Code + prior SPEC 014 QA exist.
- Browser click-through of control-plane tabs was not repeated here (SPA title + asset 200 + JS contains DEMO + crm/metrics). Last UI smoke: `docs/qa/orca-hygiene.md`.
- Live T18 numbers were not re-seeded today; cited as 2026-08-22 evidence only.
- SPEC 006 and 010 still show unchecked AC boxes in the markdown even though packages exist — spec drift, not re-scored as “missing code”.

Verified against goso commit `a0c2ee3` and goso-crm commit `db9e8bc`. Bounded demo: `GOSOCRM_FAKE=1` `:8089` + control-plane preview `:3000`, then stopped.
