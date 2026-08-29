# PLAN 084 — Channels MVP Live

> SPEC: `.planning/specs/084-channels-mvp-live.md`  
> **LOCKED: 2026-08-29** — approved Recommended §10 (17 answers + 2 extras).  
> Cook TDD. Không merge. Không bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.  
> Không copy GoClaw / zca-js / CRM. Không invent production tokens.

**Scope LOCKED:** Telegram `poll|webhook` + WS internal (`ws_up`, không catalog row) + Zalo OA webhook + Zalo Personal **QR surface + sidecar inject**. Secrets-box optional (`GOSO_MASTER_KEY`). Phase-2 Discord/Slack/Feishu/WhatsApp **parked** (Slack `env_names` bot+app ngay).

---

## Non-goals (không cook)

- Live Start Discord / Slack / Feishu / WhatsApp (kể cả khi env đầy → `health=parked`).
- In-process Zalo Personal reverse protocol.
- OA long-poll. Catalog row `websocket`. Multi-instance cùng platform.
- Telegram STT / forum / HTML pipeline. `chat_behavior` multi-bubble.
- CRM/ZCRM bridge. PATCH plaintext tokens. Demo ports. Merge.

---

## Code hôm nay (scout 2026-08-29 — đừng bịa path)

| Bề mặt | File thật | Gap 084 |
|--------|-----------|---------|
| Catalog 7 tên, 1 env/name | `gateway/internal/channel/catalog.go`, `catalog_test.go` | OA cần 2 required; Slack bot+app; không health/policy |
| GET/PATCH channels | `gateway/internal/httpapi/handlers.go` `registerChannels` / `handlePatchChannel` | PATCH **luôn 400** `channel tokens are env-only` (078); GET chỉ `Catalog()` |
| Webhooks 7 adapter | `telegram.go` `zalo_oa.go` `zalo_personal.go` + discord/slack/feishu/whatsapp | Webhook inject; **không** Start/poll/getMe/policy |
| Inbound LLM | `gateway/internal/channel/inbound.go` | Chưa `CheckPolicy` / binding |
| Wire mux | `gateway/internal/serve/serve.go` `muxWithPairing` | Tạo adapter, `HandleUpdate` only |
| View pairing 077 | `gateway/internal/auth/pairing.go`, `httpapi/handlers_pairing.go` | **Không** đụng; Channel Pairing = route **khác** |
| Secrets box | `gateway/internal/secrets/box.go` `Put`/`Get`; `store` `PutSecret`/`GetSecret`; table `secrets` trong `sqlite.go` | **Chưa** `DeleteSecret`; chưa key `channel:<name>:<kind>` |
| healthz / stats | `httpapi/handlers.go` `GET /healthz` `{ok,version}`; `observe/stats.go` `GET /api/stats` | Chưa `ws_up` |
| Lite | `store.LiteEnabled()` `GOSO_LITE` | UI lite-off; **chưa** cấm Start (không có Start) |
| CP Channels | `control-plane/src/pages/ChannelsPage.tsx`, `api/channels.ts`, `i18n/vi.ts` `en.ts` | configured/missing/env_names only |
| Agents list (picker) | `control-plane/src/api/client.ts` `listAgents` | Dùng lại, không invent `agentsApi` |
| Live smoke mẫu | `scripts/e2e-router9.sh` | Copy pattern skip exit 0 |
| QA | `docs/qa/078-channels-config.md` (pattern) | Tạo `docs/qa/084-channels-mvp-live.md` |

**Không có** `channel_config` table, `manager.go`, `policy.go`. Thêm file mới theo pattern `sqlite_cron.go` / `handlers_pairing.go`.

---

## Critical path (tuần tự)

```
T01 glossary (optional, //)
    → T02–T03 store channel_config + channel_pairing + DeleteSecret
    → T04 policy CheckPolicy
    → T05 pairing HTTP (tách 077)
    → T06 PATCH binding/policy persist
    → T07 secrets-box overlay + env wins
    → T08 catalog health + Slack/OA env_names + Lite no-Start + parked
        → T09 Telegram Start  // T10 ws_up  // T11 OA verify  (parallel sau T08)
        → T12 Personal QR/logout (sau T07+T08)
        → T13 CP UI (sau T08 + pairing + QR routes)
        → T14 docs + smoke script
        → T15 QC
```

Telegram poll loop **phụ thuộc** T08 health/manager skeleton. Không Start Phase-2.

---

## Parallel (sau khi file ownership rõ)

| Wave | Tasks | Cùng lúc? |
|------|-------|-----------|
| A | T01 glossary | Có — không đụng Go |
| B | T02 schema vs (không) | Không — cùng `store.go` / `sqlite.go` |
| C | T04 policy vs T05 pairing HTTP | **Có** nếu T02 xong: `channel/policy.go` vs `httpapi/handlers_channel_pairing.go` |
| D | T10 `ws_up` vs T09 Telegram vs T11 OA | **Có** sau T08: `observe/stats.go`+`handlers.go` healthz vs `telegram.go` vs `zalo_oa.go` |
| E | T13 CP vs T14 docs | **Có** (TS vs md) sau API ổn |
| — | Không parallel cùng `handlers.go` `registerChannels` / `catalog.go` / `catalog_test.go` | |

Cook **một** writer trên `catalog.go` + `handlers.go` + `catalog_test.go` (T08 chạm cả ba).

---

## Tasks (TDD: test đỏ → code tối thiểu → xanh)

Mỗi task ~2–5 phút net (không tính nghĩ spec). Checkbox cook.

### T01 — Glossary terms (optional)

- **Làm:** Thêm hàng `InboundMessage`, `ChannelBinding`, `Channel Pairing` (khác 077) vào `.planning/glossary.md`. Không đổi code.
- **Verify:** Định nghĩa khớp SPEC §7.
- **Parallel:** có (wave A).
- **Commit:** `docs(glossary): add 084 channel pairing terms`

### T02 — SQLite `channel_config` + `channel_pairing` + memory store

- **Làm:** Test trước `TestChannelConfig_PutGet` / `TestChannelPairing_PendingCap` (memory + sqlite temp file).
  - Files **mới:** `gateway/internal/store/store_channels.go`, `store_channels_test.go`, `sqlite_channels.go`
  - Sửa: `gateway/internal/store/sqlite.go` (CREATE IF NOT EXISTS, ALTER-safe), `store.go` (`StoreIface` + memory maps)
  - Schema (SPEC §7.1 / §7.3):  
    `channel_config(name PK, enabled, agent_id, dm_policy, group_policy, require_mention, allow_from JSON, updated_at)`  
    `channel_pairing(id, channel, sender_id, code_hash, status, expires_at, created_at, approved_at)`  
  - **Thêm** `DeleteSecret(name) error` trên `StoreIface` + memory + sqlite (cần logout T12). Không cột token trên `channel_config`.
- **Verify:** `go test ./gateway/internal/store -count=1 -run 'ChannelConfig|ChannelPairing|DeleteSecret'`
- **Commit:** `test(store): channel_config pairing tables`

### T03 — Pairing helper (hash, alphabet, TTL 60m, max 3)

- **Làm:** Test đỏ alphabet `ABCDEFGHJKLMNPQRSTUVWXYZ23456789`, TTL 60m, max 3 pending `(sender, channel)`, hashed at rest.  
  File **mới:** `gateway/internal/channel/pairing.go`, `pairing_test.go` (logic; persist qua store T02). **Không** import `auth.Pairing` (077).
- **Verify:** `go test ./gateway/internal/channel -count=1 -run Pairing`
- **Commit:** `test(channel): pairing codes 60m cap3`

### T04 — `CheckPolicy` matrix

- **Làm:** Test đỏ mọi `dm_policy` × `group_policy` × mention; Personal default allowlist **kể cả** `GOSO_ENV=demo`; Telegram demo **được** `open`; reject không gọi LLM.  
  Files **mới:** `gateway/internal/channel/policy.go`, `policy_test.go`. Defaults SPEC §6. Debounce 60s = function + fake clock test.  
  **Chưa** wire HandleUpdate (T09/T11/T12).
- **Verify:** `go test ./gateway/internal/channel -count=1 -run Policy`
- **Parallel:** với T05 sau T02.
- **Commit:** `test(channel): CheckPolicy matrix`

### T05 — HTTP `/api/channel-pairing` (tách 077)

- **Làm:** Test đỏ: admin GET list (no `code`); POST approve/deny; 404; 409 expired; view-token 403; `POST /api/pairing/exchange` **không** approve channel sender.  
  Files **mới:** `gateway/internal/httpapi/handlers_channel_pairing.go`, `handlers_channel_pairing_test.go`  
  Wire `aliasAPI` trong file này (pattern 077 `handlers_pairing.go`). **Không** sửa `handlers_pairing.go` trừ conflict test import. Routes:  
  `GET /api/channel-pairing`  
  `POST /api/channel-pairing/{id}/approve`  
  `POST /api/channel-pairing/{id}/deny`  
  + `/v1` alias.
- **Verify:** `go test ./gateway/internal/httpapi -count=1 -run ChannelPairing`
- **Commit:** `feat(httpapi): channel-pairing approve deny`

### T06 — PATCH non-secret + ChannelBinding

- **Làm:** Đổi `handlePatchChannel` (hiện **luôn 400**): token-like vẫn 400; `{enabled, dm_policy, group_policy, allow_from, require_mention, agent_id}` persist `channel_config`. `agent_id` lạ → 404. GET list sau đó trả policy + `bound_agent_id`. Cần truyền `Store` vào handler (`registerChannels` / `Options` đã có `Store`).  
  Sửa: `gateway/internal/httpapi/handlers.go` (`handlePatchChannel`, `registerChannels`)  
  Test: `gateway/internal/httpapi/handlers_webhooks_test.go` (078 `TestChannelsAPI_PatchDoesNotWriteSecrets` **giữ** 400 token); **thêm** test persist policy — file `handlers_channels_test.go` **mới** nếu file webhook test đã dày.
- **Verify:** `go test ./gateway/internal/httpapi -count=1 -run 'ChannelsAPI|PatchChannel'`
- **Commit:** `feat(httpapi): patch channel non-secret config`

### T07 — Secrets-box overlay (env thắng)

- **Làm:** Test: `secrets.Put` key `channel:zalo-personal:session`; GET `/api/channels` `secret_set=true` không plaintext; env `GOSO_ZALO_PERSONAL_TOKEN` thắng box; PATCH token 400; JSON grep không fixture.  
  Files: `gateway/internal/secrets/box.go` (giữ Put/Get; dùng `DeleteSecret` T02); helper `channel/creds.go` **mới** `SecretName(name, kind string)`; catalog/manager đọc env rồi box.  
  Test: `gateway/internal/channel/creds_test.go` + no-leak trong httpapi.
- **Verify:** `go test ./gateway/internal/channel ./gateway/internal/secrets ./gateway/internal/httpapi -count=1 -run 'SecretSet|ChannelCred|JSONOmitsToken'`
- **Commit:** `feat(channel): secrets-box overlay env wins`

### T08 — Health catalog + Lite + Phase-2 parked + env_names

- **Làm:** Mở rộng `channel.Info` + `Catalog` (hoặc `CatalogWith(st, mgr)`): fields SPEC AC-F6.  
  `requiredEnv`:  
  - `zalo-oa`: `GOSO_ZALO_OA_ACCESS_TOKEN`, `GOSO_ZALO_OA_SECRET` (+ list optional `GOSO_ZALO_OA_APP_ID` trong `env_names` nhưng **không** chặn `configured`)  
  - `slack`: `GOSO_SLACK_BOT_TOKEN`, `GOSO_SLACK_APP_TOKEN`  
  Slack/discord/feishu/whatsapp: `phase=2`, `health=parked` (không Start).  
  `GOSO_LITE=1`: manager **không** Start; GET `"lite": true`; 7 tên.  
  **Manager skeleton** file **mới** `gateway/internal/channel/manager.go` `manager_test.go`: Start/Stop no-op Phase-2; Lite skip; một instance/name.  
  Sửa tests cứng 1 env: `catalog_test.go` `TestCatalog_SevenNamesUnconfigured`; `handlers_webhooks_test.go` list-seven.  
  GET `/api/channels/{name}/health` admin+view.
- **Verify:** `go test ./gateway/internal/channel ./gateway/internal/httpapi -count=1 -run 'Catalog|ChannelsAPI|Lite|Parked'` — **đúng 7 tên**, không `websocket`.
- **Commit:** `feat(channel): health parked lite env_names`

### T09 — Telegram live Start (poll/webhook)

- **Làm:** TDD trên `telegram.go` / `telegram_test.go` + `manager.go`:  
  - `GOSO_TELEGRAM_MODE` default `poll`; webhook route **giữ**.  
  - Start: một `getMe` qua `HTTPClient`/`APIBase` injectable (httptest).  
  - `mode=webhook` không `GOSO_PUBLIC_URL` → `health=failed`, error `public url required for webhook mode`, không giả running.  
  - Secret set: header `X-Goso-Telegram-Secret` hoặc `?secret=` → sai = 401.  
  - Group `require_mention` default true; wire `CheckPolicy` trước LLM.  
  - Defaults DM `pairing` / group `allowlist`.  
  - Re-probe 5m: constructor interval override (ms) cho test.  
  - Lite: không poll.  
  Wire `serve.go` `Start` khi không Lite. **Không** Dial `api.telegram.org` trong unit.
- **Verify:** `go test ./gateway/internal/channel -count=1 -run Telegram`  
  `go test ./gateway/internal/serve -count=1 -run Telegram` nếu có.
- **Parallel:** T10, T11 sau T08.
- **Commit:** `feat(telegram): poll webhook start getMe policy`

### T10 — `ws_up` (không catalog row)

- **Làm:** `observe.Stats` thêm `WsUp bool \`json:"ws_up"\``; `HandleStats` trả field. `GET /healthz` thêm `ws_up` (cùng process: true sau `RegisterWS`). Không thêm tên catalog. Tests `ws_test.go` / `observe` / `handlers.go` healthz.  
  Files: `gateway/internal/observe/stats.go`, `observe.go` Register (đã mount `/api/stats`); `httpapi/handlers.go` healthz; `httpapi/ws.go` (set flag khi upgrade path registered — **up = server sẵn sàng**, không cần client connected).
- **Verify:** `go test ./gateway/internal/observe ./gateway/internal/httpapi -count=1 -run 'Stats|Healthz|WS'` + catalog vẫn 7.
- **Commit:** `feat(observe): ws_up on healthz stats`

### T11 — Zalo OA webhook-only + verify matrix

- **Làm:** `zalo_oa.go` / `zalo_oa_test.go`: required token+secret cho `configured`; APP_ID optional. Secret set → sai/thiếu verify **401**. Secret missing: `GOSO_ENV=demo` accept + warn; `production` fail-closed. Không long-poll. DM pairing / group disabled. Wire `CheckPolicy`. Catalog `env_names` khớp T08.
- **Verify:** `go test ./gateway/internal/channel -count=1 -run ZaloOA`
- **Commit:** `feat(zalo-oa): webhook verify demo vs production`

### T12 — Zalo Personal QR surface + logout (không protocol)

- **Làm:**  
  - `GET /api/channels/zalo-personal/qr` → `{status, expires_at}` **không** cookie/imei/token. Không sidecar URL trong LOCK → status `unconfigured` khi không env/box; `confirmed` khi env hoặc box session (`secret_set`). **Không** generate unofficial QR in-process.  
  - `POST .../logout` → `DeleteSecret("channel:zalo-personal:session")` only; env vẫn thắng.  
  - Webhook 004 **giữ**; policy default allowlist (demo không `open`).  
  - Log một dòng `zalo-personal unofficial`.  
  Files **mới:** `httpapi/handlers_zalo_personal_qr.go`, `_test.go`. Sửa `zalo_personal.go` policy only.  
  **Cấm** import zca / copy CRM.
- **Verify:** `go test ./gateway/internal/httpapi ./gateway/internal/channel -count=1 -run 'ZaloPersonal|PersonalQR'`
- **Commit:** `feat(zalo-personal): qr surface logout sidecar inject`

### T13 — Control Plane Channels UI

- **Làm:**  
  - `control-plane/src/api/channels.ts` — type health/policy/binding; `patch`, `qr`, `logout`, `channelPairing` list/approve/deny.  
  - `ChannelsPage.tsx` — badge health, policy summary, agent picker (`client.listAgents`), pending pairing, QR panel + **risk banner**, Slack 2 env names, parked. **Không** input token.  
  - `control-plane/src/i18n/vi.ts` + `en.ts` keys mới.  
  - Typecheck only (`package.json` `"typecheck": "tsc --noEmit"`; CP test hiện chỉ `health.test.ts`).
- **Verify:** `cd control-plane && npm run typecheck`
- **Parallel:** T14.
- **Commit:** `feat(cp): channels health policy qr i18n`

### T14 — Docs + live smoke gated OFF

- **Làm:**  
  - **Mới:** `docs/qa/084-channels-mvp-live.md` (cite GoClaw **paths only**, mapping, proof, skip-always).  
  - `docs/SETUP.md`, `docs/RUNBOOK.md`, `.env.example`: `GOSO_TELEGRAM_MODE`, `GOSO_PUBLIC_URL`, `GOSO_TELEGRAM_WEBHOOK_SECRET`, OA secret+optional APP_ID, `GOSO_SLACK_APP_TOKEN`, live flags empty, Personal sidecar inject + risk. Placeholder rỗng.  
  - **Mới:** `scripts/e2e-channels-live.sh` pattern `e2e-router9.sh`: thiếu flag/token → **exit 0**; `--port 0`; không demo ports. **Không** thêm vào `make verify`.
- **Verify:** `./scripts/e2e-channels-live.sh` exit 0 không flag; `./scripts/agpl-check-docs.sh`; không secret trong diff.
- **Commit:** `docs(qa): 084 channels mvp live evidence`

### T15 — Final QC

- **Làm:** Chạy QC; sửa regression test `env_names` length; không merge; không start SPEC khác.
- **Verify:**

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
gofmt -l gateway desktop
go vet ./...
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

- **Commit:** (chỉ nếu QC bắt buộc sửa nhỏ) `test(channel): 084 qc follow-up` — hoặc không commit nếu xanh.

---

## Trạng thái cook

- [ ] T01 glossary (optional)
- [ ] T02 store tables + DeleteSecret
- [ ] T03 pairing helper
- [ ] T04 CheckPolicy
- [ ] T05 `/api/channel-pairing`
- [ ] T06 PATCH non-secret + binding
- [ ] T07 secrets-box overlay
- [ ] T08 health / lite / parked / env_names
- [ ] T09 Telegram Start
- [ ] T10 `ws_up`
- [ ] T11 Zalo OA verify
- [ ] T12 Personal QR/logout
- [ ] T13 CP UI
- [ ] T14 docs + smoke script
- [ ] T15 QC

---

## AC map (LOCKED spec → task)

| AC | Task |
|----|------|
| F1 Manager, 1 instance, no Phase-2 Start | T08, T09 |
| F2 Policy defaults / demo open TG / Personal never default open | T04, T09, T11, T12 |
| F3 Pairing 60m 8char cap3 debounce 60s | T03, T05 |
| F4 Binding agent_id | T06 |
| F5 Secrets env wins, box, no PATCH token | T07, T06 |
| F6 Health fields GET channels | T08 |
| F7 Lite forbid Start | T08, T09 |
| F8 PATCH non-secret | T06 |
| T1–T7 Telegram | T09 |
| W1–W4 WS `ws_up` no catalog row | T10 |
| Z1–Z6 OA | T08, T11 |
| P1–P7 Personal QR+sidecar inject | T12, T13 |
| S1–S6 no leak | T07, T09, T12, T15 |
| D1–D4 docs/QA | T14, T15 |

---

## Mơ hồ còn lại (không mở scope — cook theo default)

1. **Personal QR image:** LOCK không có sidecar URL env. Cook: GET QR **không** gọi unofficial protocol; `unconfigured` nếu không env/box; không bắt buộc `image_png_base64`. Sidecar chỉ **inject webhook 004**. Đủ AC-P2/P4.
2. **`ws_up` nghĩa:** server đã `RegisterWS` (listen) chứ không phải có client. Gắn `true` khi mux mount `/ws`.
3. **OA optional APP_ID trong `env_names`:** catalog help list 3 tên; `configured` chỉ 2 required. Test T08 phải tách `required` vs `env_names`.
4. **PATCH `enabled`:** 078 từ chối mọi PATCH; 084 LOCK cho `enabled`. T06 lật 078 chỉ với non-secret keys — test 078 token 400 **giữ**.

Không hỏi Dat trừ khi cook bị chặn bởi (1)–(4) trái AC. Default trên **đúng LOCK**.

---

## Quy tắc cook

- Test đỏ trước. Injectable HTTP (`APIBase`, `HTTPClient`, `Sender`) như 003/040.
- Không `net.Dial` vendor trong `go test`.
- Không đọc goclaw `.go`.
- Không merge. Không start SPEC 085+.
- Commit atomic theo cột trên; conventional; không AI trailer.
