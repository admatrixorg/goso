# SPEC 084 — Channels MVP Live (Telegram + WebSocket + Zalo OA + Zalo Personal QR/sidecar)

> DRAFT: 2026-08-29 — Advisor soạn. **Không LOCK** trừ khi operator (Dat) xác nhận.
> Proposed LOCK answers: §10 (Recommended). STATUS vẫn DRAFT.
> Clean-room. Không copy GoClaw / ZaloCRM / zca-js. Không invent production tokens.
> Số **002 đã dùng** (`002-gateway-http-session.md`) — SPEC này là **084**.
> Không bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.
> DI còn lại không liên quan (DI-08 search, DI-09 pgvector, DI-12/13/21 spawn, DI-19 SSO, …) **giữ parked**.

**Zalo Personal trong 084:** **QR surface + sidecar HTTP inject** (webhook 004). **Không** in-process reverse protocol.

Mở khóa **live connect có kiểm soát** cho 4 Channel MVP (Decision #06). Adapter fake/inject (003/004/040) và config depth không-secret (063/078) giữ nguyên; SPEC này thêm vận hành live: secrets tách config, policy, Channel Pairing, ChannelBinding, health, QR surface, sidecar inject, runbook.

---

## 0. GoClaw behavior (READ-ONLY cite — do not copy code)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Đọc **docs only**. Không đọc/paste `.go`. Không vendor.

| Hành vi công khai | Cite |
|-------------------|------|
| Channel interface `Name` / `Start` / `Stop` / `Send` / `IsRunning` / `IsAllowed`; webhook mount trên mux chính | `docs/05-channels-messaging.md` §2 |
| `dm_policy`: `pairing` / `allowlist` / `open` / `disabled`; `group_policy`: `open` / `allowlist` / `disabled`; `allow_from`; `requireMention` (Telegram group) | `docs/05-channels-messaging.md` §3, §5 |
| Channel DM pairing: mã 8 ký tự, TTL 60 phút, debounce 60s, admin approve | `docs/05-channels-messaging.md` §15 |
| Secrets chỉ env / `.env.local`, không ghi vào config JSON; `MaskedCopy` / `StripSecrets` | `docs/00-architecture-overview.md` §10 |
| Telegram: long poll mặc định; group mention gating; session theo chat/topic | `docs/05-channels-messaging.md` §4–§5 |
| Zalo OA: official Bot API, DM only, default `dm_policy=pairing` | `docs/05-channels-messaging.md` §10 |
| Zalo Personal: protocol unofficial; QR hoặc credential; default allowlist; warning `security.unofficial_api`; rủi ro khóa/ban | `docs/05-channels-messaging.md` §11 |
| Channel instance gắn Agent (workspace isolation) | `docs/05-channels-messaging.md` §12 |
| WhatsApp native QR + STT E2E opt-in (Phase-2 note only) | `docs/05-channels-messaging.md` §9, §5 STT Decision 6 |
| Device/view pairing (077) ≠ Channel DM pairing | `docs/20-api-keys-auth.md`; `docs/09-security.md` browser pairing |

Goso mapping (self-written, không copy schema): xem §7–§8. Personal live listener **không** port protocol GoClaw vào process GOSO.

---

## 1. Goal

Operator có thể **connect thật** bốn Channel MVP — **Telegram + WebSocket + Zalo OA + Zalo Personal** — với trải nghiệm vận hành gần GoClaw: credentials tách khỏi config không-secret, `dm_policy` / `group_policy` / `allow_from` / `require_mention`, Channel Pairing (approve), ChannelBinding (channel↔Agent), health `missing | stopped | running | failed` (Phase-2 thêm `parked`), **không lộ token** trên API/log/Control Plane.

**Zalo Personal (084):** Control Plane + API **QR surface**, risk banner, allowlist default, logout, `secret_set`; inbound live qua **sidecar HTTP** do operator host, inject `POST /api/channels/zalo-personal/webhook` (shape 004). Gateway **không** chạy reverse-engineered protocol in-process (SPEC sau).

Discord / Slack / Feishu / WhatsApp: **checklist + config + health Phase-2** (`parked`), không live Start. Slack `env_names[]` = bot + app token ngay trong 084. WhatsApp giữ Cloud-API (040); native QR = DI-01 parked.

Kết thúc 084, `docs/qa/084-channels-mvp-live.md` ghi evidence; unit fake vẫn xanh; live smoke **CI skip-always** cho đến khi Dat set flag+token local (không commit).

---

## 2. User Stories

- **US-01 Operator — Telegram live.** Đặt `GOSO_TELEGRAM_BOT_TOKEN`. `GOSO_TELEGRAM_MODE` default `poll` (không cần public URL). Mode `webhook` cần `GOSO_PUBLIC_URL` — thiếu → `health=failed` + `last_error` rõ, không im lặng. `GET /api/channels` hàng `telegram`: `configured`/`missing`/`health`. DM đã allow/pair → Session `telegram:<chat_id>` → Agent bound → Bot API. Token không ra JSON/log/UI.
- **US-02 Operator — WebSocket / internal chat.** `GET /ws` (admin Bearer; `GOSO_WS_ORIGINS`) `{op:"chat", payload:{session_id, message}}` → LLM. **Không** hàng catalog `websocket` (vẫn 7 tên). Health `ws_up` trên `/healthz` hoặc `/api/stats`.
- **US-03 Operator — Zalo OA live.** Đặt `GOSO_ZALO_OA_ACCESS_TOKEN` **và** `GOSO_ZALO_OA_SECRET` (required). Optional `GOSO_ZALO_OA_APP_ID`. Chỉ webhook (không OA long-poll). Secret set → verify fail-closed. Secret thiếu: demo chấp nhận + warn; production fail-closed. Session `zalo-oa:<user_id>`. Default DM `pairing`; group `disabled`.
- **US-04 Operator — Zalo Personal QR surface + sidecar.** CP Channels → QR login + banner unofficial/ban. QR API không trả cookie/imei/token. Listener live = sidecar operator → inject webhook 004. Env `GOSO_ZALO_PERSONAL_TOKEN` thắng nếu set; không thì secrets-box QR session. Logout chỉ xóa secrets-box, **không** unset process env. Default allowlist (kể cả demo).
- **US-05 Operator — policy + pairing + binding.** PATCH không-secret. Channel Pairing: TTL 60 phút, 8 ký tự (loại `0O1IL`), max 3 pending / (sender, channel), debounce 60s. Routes `/api/channel-pairing` + approve/deny. 077 không đổi. Một instance / catalog name.
- **US-06 Engineer — fake độc lập.** `make verify` không gọi vendor. Live smoke: không flag/token → **exit 0 skip**. CI không set flag. Token không commit.
- **US-07 End-user.** User đã pair/allow nhận text từ Agent. User chưa pair (`pairing`) chỉ nhận mã, không LLM. Personal chỉ trả lời ID trong allowlist trừ khi PATCH `open` tường minh.

---

## 3. Acceptance Criteria

Checkbox đo được. Implementation sau **human LOCK**; dưới đây khớp Proposed LOCK answers §10 (Recommended, chưa LOCK).

### 3.1 Framework / runtime readiness

- [ ] **AC-F1 Manager.** Channel manager goso-shaped: `Start` khi credential đủ **và** `enabled` (default true khi configured) **và** không Lite **và** `phase=1`. `Stop` lúc shutdown. **Một instance / catalog name** (multi-bot cùng platform = SPEC sau). Discord/Slack/Feishu/WhatsApp **không** Start live (`health=parked` dù env đầy). Webhook inject stub 040 giữ cho unit.
- [ ] **AC-F2 Policy.** Trước LLM, inbound `CheckPolicy`:
  - `dm_policy`: `pairing` | `allowlist` | `open` | `disabled`
  - `group_policy`: `open` | `allowlist` | `disabled` (không `pairing` group trong 084)
  - `allow_from`: sender/chat ID (non-secret)
  - `require_mention`: bool; Telegram group default `true`
  - Reject → không LLM; pairing outbound hướng dẫn, **debounce 60s**
  - Default giữ bảng §6. `GOSO_ENV=demo` **được** Telegram DM `open`. **Personal không default `open`** kể cả demo — chỉ PATCH tường minh. Production: warn khi Personal `dm_policy=open` (log + UI), không cấm.
- [ ] **AC-F3 Channel Pairing.** Khác 077. `dm_policy=pairing` + sender chưa pair/allow → mã **8 ký tự** alphabet `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (loại `0 O 1 I L`), **TTL 60 phút** (không khớp 077 10 phút), **max 3 pending** per `(sender_id, channel)`, debounce 60s. Admin `GET /api/channel-pairing`; `POST .../approve` và `POST .../deny`. Store hashed; GET không raw code. Approve xong inbound vào pipeline.
- [ ] **AC-F4 ChannelBinding.** `agent_id` nullable. Null → synthetic `agent_key` = tên channel (003/004). Non-null → Session thuộc Agent đó; PATCH 404 nếu Agent không tồn tại. Config JSON, không secret. Một binding / catalog name.
- [ ] **AC-F5 Secrets vs config.** Secret không nằm config JSON/sqlite plaintext. Nguồn: process env `GOSO_*` **thắng nếu set**. Overlay: `secrets` AES-256-GCM khi `GOSO_MASTER_KEY` có, key `channel:<name>:<kind>` (vd. `channel:zalo-personal:session`). GET chỉ `secret_set: true/false`. PATCH token/secret field → **400** `channel tokens are env-only` (078). Không PATCH plaintext tokens.
- [ ] **AC-F6 Health fields.** `GET /api/channels` (và `/v1/channels`) bổ sung, không phá 078:

  | Field | Ý nghĩa |
  |-------|---------|
  | `name` `configured` `missing` `env` `env_names` | Giữ 063/078; `env_names` cập nhật §6 |
  | `health` | `missing` \| `stopped` \| `running` \| `failed` \| `parked` |
  | `transport` | `webhook` \| `poll` \| `ws` \| `qr` \| `sidecar` \| `none` |
  | `secret_set` | bool |
  | `bound_agent_id` | string, có thể rỗng |
  | `dm_policy` `group_policy` `require_mention` | non-secret |
  | `allow_from_count` | số phần tử; optional `allow_from` |
  | `phase` | `1` hoặc `2` |
  | `last_error` | public, redact token-like |

  Mapping: required env/secret thiếu → `missing` + `configured=false`. Credential đủ, chưa Start / `enabled=false` → `stopped`. `GOSO_LITE=1` → không Start, catalog vẫn list + `"lite": true` (health `stopped` hoặc tương đương documented, **không** live). Listener/webhook OK → `running`. Lỗi auth/network/mode (vd. webhook không `GOSO_PUBLIC_URL`) → `failed` + error rõ, **không silent**. Phase-2 → `parked`.
- [ ] **AC-F7 Lite.** `GOSO_LITE=1`: **cấm live Start** (Telegram poll/setWebhook, OA live send path, Personal sidecar-connect nếu có) — không chỉ ẩn UI. Catalog vẫn 7 tên + `"lite": true`. UI Lite off giữ (055).
- [ ] **AC-F8 PATCH non-secret.** `{enabled?, dm_policy?, group_policy?, allow_from?, require_mention?, agent_id?}`. Unknown name 404. Token-like 400. Persist `channel_config` (hoặc tương đương) — không cột token plaintext.

### 3.2 Live Telegram

- [ ] **AC-T1** Required: `GOSO_TELEGRAM_BOT_TOKEN`. Optional: `GOSO_TELEGRAM_WEBHOOK_SECRET`. Nếu secret set: webhook thiếu/sai **`X-Goso-Telegram-Secret`** (hoặc `?secret=` nếu configured) → **401**, không LLM. **Không** copy tên header GoClaw (`X-GoClaw-*`).
- [ ] **AC-T2 Transport.** `GOSO_TELEGRAM_MODE=poll|webhook`, **default `poll`**. Route `POST /api/channels/telegram/webhook` **giữ** (003) mọi mode (inject/unit). Mode `poll`: không cần `GOSO_PUBLIC_URL`; Start long-poll. Mode `webhook`: dùng `GOSO_PUBLIC_URL` (optional env, **bắt buộc khi mode=webhook**) để `setWebhook`. Mode `webhook` **không** public URL → `health=failed`, `last_error` rõ (vd. `public url required for webhook mode`), không Start giả `running`.
- [ ] **AC-T3** Inbound text → policy → `telegram:<chat_id>` → Agent bound → `sendMessage` khi token có. Sender injectable (003).
- [ ] **AC-T4** Group `require_mention=true` default; không mention và không reply-to-bot → bỏ qua, không LLM.
- [ ] **AC-T5** Default: DM `pairing`, group `allowlist`, `require_mention=true`. Demo **được** PATCH/default-override Telegram DM `open`. Production Personal/`open` = warn (AC-F2).
- [ ] **AC-T6** Không STT / forum / HTML pipeline / 10+ commands. Text in/out.
- [ ] **AC-T7 Health.** Start = **một** `getMe` (httptest trong unit; live chỉ khi smoke flag). Runtime = poll loop **hoặc** webhook path sống. Background **re-probe 5 phút**. Token revoke / getMe fail → `failed`, `last_error` redact. Webhook mode thiếu URL = `failed` ngay (AC-T2).

### 3.3 Live WebSocket / internal chat

- [ ] **AC-W1** `GET /ws` JSON `{op, payload}` ping/pong + chat (040) giữ. Admin Bearer; view-token không chat (077). Origin `GOSO_WS_ORIGINS`.
- [ ] **AC-W2** **Không** thêm catalog row `websocket`. Catalog **đúng 7 tên**. WS health: `GET /healthz` hoặc `GET /api/stats` field **`ws_up`**. QA ghi “internal chat live”. CP Chat = surface operator.
- [ ] **AC-W3** Session phải tồn tại; thiếu `session_id` → error frame. Không lộ token trên WS log.
- [ ] **AC-W4** Không public URL. Không vendor token.

### 3.4 Live Zalo OA

- [ ] **AC-Z1** Required env: `GOSO_ZALO_OA_ACCESS_TOKEN` **và** `GOSO_ZALO_OA_SECRET`. Optional: `GOSO_ZALO_OA_APP_ID`. `env_names[]` liệt kê cả ba; `configured=true` chỉ khi **hai required** non-empty (APP_ID không chặn configured).
- [ ] **AC-Z2 Verify.** Secret set → webhook verify **fail-closed** (sai/thiếu → 401, không LLM). Secret **missing**: `GOSO_ENV=demo` (và non-production) **accept** webhook + **warn** log/health; **production fail-closed** (reject unverified).
- [ ] **AC-Z3** `POST /api/channels/zalo-oa/webhook` parse 004. DM only; group bỏ qua. Default `dm_policy=pairing`, `group_policy=disabled`.
- [ ] **AC-Z4** Outbound OA send API; unit httptest/inject. Live mạng chỉ smoke flag.
- [ ] **AC-Z5 Transport.** **Webhook-first only.** Không OA long-poll trong 084. Public URL **cần** (operator tunnel) — document runbook; không thêm OA poll mode.
- [ ] **AC-Z6** Health: thiếu required → `missing`; webhook OK + required set → `running` cho đến send/auth fail → `failed`.

### 3.5 Live Zalo Personal (QR surface + sidecar HTTP inject)

- [ ] **AC-P1** CP (vi+en): nút QR login + **banner cố định** unofficial / có thể ban / không dùng account chính công ty. Log public goso-shaped `zalo-personal unofficial` (không copy chuỗi GoClaw).
- [ ] **AC-P2** `GET /api/channels/zalo-personal/qr` → `{status: pending|scanned|confirmed|expired|failed|unconfigured, expires_at}` + ảnh QR optional **không** cookie/imei/token. Poll hoặc WS op `channel.qr`. Credential sau confirm → secrets-box (nếu master key) hoặc hướng dẫn env; GET không echo.
- [ ] **AC-P3** Default `dm_policy=allowlist`, `group_policy=allowlist` **kể cả demo**. `open` chỉ PATCH tường minh. Production warn nếu Personal `open`.
- [ ] **AC-P4 Sidecar, không in-process protocol.** Webhook inject 004 **giữ**. Live listener = **operator-hosted sidecar HTTP** POST vào `POST /api/channels/zalo-personal/webhook`. Gateway **không** implement reverse protocol, không copy `zca-js` / goclaw-source / CRM internals. In-process clean-room protocol = **SPEC sau**. QR surface có thể hiển thị payload/status sidecar trả về; không tự nói unofficial wire protocol.
- [ ] **AC-P5 Precedence.** `GOSO_ZALO_PERSONAL_TOKEN` (env) **thắng nếu set**; else secrets-box QR session. `secret_set` true nếu env **hoặc** box có session.
- [ ] **AC-P6 Logout.** `POST /api/channels/zalo-personal/logout` xóa **secrets-box only**, **không** unset process env. Nếu env vẫn set → vẫn `secret_set`/configured theo env. Không cookie plaintext trên disk.
- [ ] **AC-P7** Text in/out. Không sticker/image.

### 3.6 Observability / security

- [ ] **AC-S1** GET/PATCH/list/logs/traces/`last_error`/QR: không token, cookie, `access_token`, `imei`, webhook secret. Grep token-like (078) mở rộng live handlers.
- [ ] **AC-S2** Access log path + status; không query token, không Authorization. `?secret=` nếu dùng: không log raw query.
- [ ] **AC-S3** CP badge `running` / `failed` / `missing` / `parked` (vi+en). Không input password token (078).
- [ ] **AC-S4** Channel Pairing approve/deny = admin. View-token 403. 077 routes untouched.
- [ ] **AC-S5** Production warn Personal/`open` và Telegram `open` nếu muốn; Personal default không `open`.
- [ ] **AC-S6** Không secret trong git / `.env.example` (placeholder rỗng). Secret-scan 0.

### 3.7 Docs / runbook / QA evidence

- [ ] **AC-D1** `docs/qa/084-channels-mvp-live.md` — cite paths only, mapping, proof, live smoke **skip-always** trên CI.
- [ ] **AC-D2** SETUP + RUNBOOK + `.env.example`: §6, Personal sidecar + risk, `GOSO_TELEGRAM_MODE`, `GOSO_PUBLIC_URL`, OA secret required, Slack bot+app names, live flags empty.
- [ ] **AC-D3** QC sau implement: typecheck, `go test ./...`, build, gofmt, vet, agpl. Không merge. Không start SPEC khác từ worker 084.
- [ ] **AC-D4** Phase-2 runbook: Discord/Feishu/WhatsApp parked; Slack `env_names` bot+app, `health=parked`, no Start. WhatsApp Cloud-API retained; native DI-01.

---

## 4. Non-Goals

- Copy code / schema / header GoClaw (`X-GoClaw-*`). Giữ `/api/channels` + alias `/v1` (052).
- **In-process** Zalo Personal reverse protocol (zca / unofficial wire). 084 = QR surface + sidecar inject only.
- Zalo OA long-poll.
- Hàng catalog `websocket`.
- Nhiều instance cùng platform (hai bot Telegram, …).
- Live full Discord / Slack / Feishu / WhatsApp. Slack không Start dù đủ bot+app. Native WhatsApp = **DI-01 parked**.
- Telegram STT, forum, HTML pipeline, voice, 10+ commands.
- `chat_behavior` multi-bubble (084 không thêm).
- CRM / ZCRM / ZaloCRM bridge; reuse `goso-crm` inbound như Channel GOSO. X3 CRM QR ≠ parity 084.
- Ghi secret sqlite plaintext; PATCH token.
- OAuth/SSO (DI-19), Apple, Stripe, K8s, Grafana Cloud, Tailscale, Redis, pgvector (DI-09), sandbox/browser/media, paid search (DI-08).
- Đổi / tái số SPEC 002.
- Invent production tokens trong repo/QA/CI.
- Bind/kill demo ports; merge; tự LOCK.
- MCP `/v1/channels` toggle (C10).
- OA template/broadcast (backlog 2026-08-19).

---

## 5. Dependencies / prior SPECs

| SPEC | Quan hệ |
|------|---------|
| **002** HTTP + Session + `/ws` echo nền. 084 không sửa 002; WS RPC = **040**. |
| **003** Telegram webhook + LLM + Sender. Poll/setWebhook đứng trên 003; route webhook giữ. |
| **004** Zalo OA + Personal webhook stubs. OA live + Personal inject đứng trên 004. |
| **005** SQLite — `channel_config` / pairing / secrets box. |
| **006** Admin Bearer. Platform webhooks không đòi admin; verify platform/goso secret. |
| **040** 7 adapters + WS + origin. Không xóa stub Phase-2. |
| **041 / 066 / 067** AES-256-GCM secrets box, SSRF, production fail-closed. Overlay Channel dùng cùng master key. |
| **055** Lite caps. 084 **cấm Start** khi Lite. |
| **063 / 078** `env` / `env_names` / `missing`; PATCH cấm secret. 084 mở PATCH non-secret; cập nhật `env_names` OA + Slack. |
| **069** `healthz` chrome. 084 thêm per-channel health + `ws_up`. |
| **077** View-token pairing. Route và TTL **tách**. |
| Decision **#06** MVP 4 Channel. **#01** clean-room. |
| DI-05/07 live MVP. DI-06 = QR+sidecar trong 084; in-process protocol parked. DI-01..04 Phase-2. |

---

## 6. Credential & setup checklist

Placeholder **tên env** only. Git để trống.

| Channel | Required secrets (names) | Optional / non-secret | Transport 084 | Public URL | Default policies | Phase |
|---------|--------------------------|----------------------|---------------|------------|------------------|-------|
| **telegram** | `GOSO_TELEGRAM_BOT_TOKEN` | `GOSO_TELEGRAM_WEBHOOK_SECRET`; `GOSO_TELEGRAM_MODE=poll\|webhook` (default **poll**); `GOSO_PUBLIC_URL` (chỉ khi webhook) | default **poll**; webhook route 003 luôn mount | **Không** nếu poll; **cần** `GOSO_PUBLIC_URL` nếu mode=webhook (thiếu → `failed`) | DM `pairing` (demo **được** `open`); group `allowlist`; `require_mention=true` | 1 live |
| **websocket** (internal, **không** catalog row) | không vendor; `GOSO_ADMIN_TOKEN` | `GOSO_WS_ORIGINS` | `GET /ws` | không | n/a | 1 live; health `ws_up` |
| **zalo-oa** | `GOSO_ZALO_OA_ACCESS_TOKEN` **+** `GOSO_ZALO_OA_SECRET` | `GOSO_ZALO_OA_APP_ID` | **webhook only** (không poll) | **Cần** | DM `pairing`; group `disabled` | 1 live |
| **zalo-personal** | env `GOSO_ZALO_PERSONAL_TOKEN` **hoặc** secrets-box QR session | sidecar operator (HTTP inject 004) | **QR surface + sidecar inject**; không in-process protocol | không (sidecar/operator) | DM `allowlist`; group `allowlist`; **không** default `open` (kể cả demo) | 1 QR+sidecar + risk |
| **discord** | `GOSO_DISCORD_BOT_TOKEN` | intents / app id — SPEC sau | stub 040 | n/a | n/a | 2 parked |
| **slack** | `GOSO_SLACK_BOT_TOKEN` **+** `GOSO_SLACK_APP_TOKEN` (`env_names[]` **ngay** 084) | user token, signing secret — SPEC sau | stub 040; **không** Start | n/a | n/a | 2 parked |
| **feishu** | `GOSO_FEISHU_APP_SECRET` | `GOSO_FEISHU_APP_ID`, encrypt, China vs intl | stub 040 | n/a 084 | n/a | 2 parked |
| **whatsapp** | `GOSO_WHATSAPP_ACCESS_TOKEN` | phone id, verify token | **Cloud-API stub 040** (giữ) | Cloud webhook cần khi live sau | n/a | 2 parked; **native QR = DI-01** |

`configured`: mọi **required** names non-empty (OA: token+secret; Slack: bot+app). Optional không chặn `configured`.

Live smoke flags (default **empty**; **CI không set**):

| Flag | Effect |
|------|--------|
| `GOSO_LIVE_TELEGRAM=1` | getMe + 1 inbound/outbound (token local Dat) |
| `GOSO_LIVE_ZALO_OA=1` | OA send allowlisted test user |
| `GOSO_LIVE_ZALO_PERSONAL=1` | QR status (+ sidecar nếu operator có) |
| `GOSO_LIVE_CHANNELS=1` | cả ba; thiếu env → skip kênh |

Thiếu flag hoặc token: script **exit 0**. Không commit token. Không phần `make verify`.

---

## 7. Data model / API surface (hành vi — không copy schema GoClaw)

| Thuật ngữ | Định nghĩa GOSO |
|-----------|-----------------|
| **InboundMessage** | `channel`, `sender_id`, `chat_id`, `peer_kind` (`direct`\|`group`), `text`, `mention`, `session_label`. |
| **ChannelBinding** | `channel_name` → `agent_id` (một / catalog name). |
| **Channel Pairing** | Approve sender khi `dm_policy=pairing`. **Không** phải 077. |

### 7.1 Config JSON (non-secret)

Một hàng / catalog name (7 tên; không row `websocket`):

```
channel_config(
  name,                 -- PK
  enabled,
  agent_id,             -- nullable
  dm_policy,            -- pairing|allowlist|open|disabled
  group_policy,         -- open|allowlist|disabled
  require_mention,
  allow_from,           -- JSON array
  updated_at
)
```

Không cột token.

### 7.2 Credentials

1. Đọc env theo required `env_names` — **env thắng nếu set**.
2. Overlay secrets box khi `GOSO_MASTER_KEY` có: key `channel:<name>:<kind>`. QR Personal session: `channel:zalo-personal:session`.
3. GET: `secret_set` only.
4. Logout Personal: xóa box key đó; **không** `unsetenv`.

### 7.3 Channel Pairing rows

```
channel_pairing(
  id, channel, sender_id, code_hash,
  status,              -- pending|approved|denied|expired
  expires_at, created_at, approved_at
)
```

TTL 60 phút. Max 3 pending `(sender_id, channel)`. Alphabet 8 ký tự loại `0O1IL`. Admin list không `code`.

### 7.4 HTTP (goso-shaped)

Giữ:

| Method | Path | 084 |
|--------|------|-----|
| GET | `/api/channels` | Health/policy/binding; 7 tên; Slack `env_names` 2 token; không `websocket`. |
| PATCH | `/api/channels/{name}` | Non-secret only. |
| POST | `/api/channels/{telegram,zalo-oa,zalo-personal,...}/webhook` | Inject; TG `X-Goso-Telegram-Secret`; OA verify §3.4. |
| GET | `/ws` | 040. |
| GET | `/healthz` hoặc `/api/stats` | Thêm `ws_up`. |
| POST | `/api/pairing` + `/exchange` | **077 không đổi.** |

Thêm:

| Method | Path | Ai | Body / response |
|--------|------|----|-----------------|
| GET | `/api/channels/{name}/health` | admin+view | Một hàng health. |
| GET | `/api/channel-pairing` | admin | `{items:[{id,channel,sender_id,status,expires_at}]}` |
| POST | `/api/channel-pairing/{id}/approve` | admin | `{ok:true}` — 404 / 409 expired. |
| POST | `/api/channel-pairing/{id}/deny` | admin | `{ok:true}` — **bắt buộc 084** (không optional). |
| GET | `/api/channels/zalo-personal/qr` | admin | `{status, expires_at, image_png_base64?}` — no session secret. |
| POST | `/api/channels/zalo-personal/logout` | admin | Xóa secrets-box only. |

**Không** nest `/api/channels/pairing`. View-token: GET health/list OK; approve/deny/QR/logout/PATCH **403**.

Telegram webhook secret: header `X-Goso-Telegram-Secret` **hoặc** `?secret=` nếu configured. Không `X-GoClaw-*`.

### 7.5 Session labels (giữ)

`telegram:<chat_id>`, `zalo-oa:<user_id>`, `zalo-personal:<thread_id>`, `discord:<channel_id>`, `slack:<channel_id>`, `feishu:<chat_id>`, `whatsapp:<from>`.

### 7.6 Control Plane

Channels: health, policy, bound agent, pending pairing, Personal QR + risk + sidecar note. Slack hiện 2 env names, badge parked. Không ô token. i18n vi+en. StatusLine 046.

---

## 8. Security & risk notes

1. **Zalo Personal unofficial.** Banner + log + default allowlist. 084 **không** chạy protocol trong gateway — sidecar operator chịu rủi ro ban. Cấm copy zca-js/goclaw/CRM.
2. **WhatsApp STT/E2E.** Phase-2 note only. Native QR = DI-01. Cloud-API 040 giữ. STT default OFF nếu bao giờ native.
3. **Token leakage.** Test grep. `last_error` redact. Không log `?secret=`.
4. **Webhook public.** TG/OA verify theo AC-T1 / AC-Z2. Timeout, body cap. Không tin `X-Forwarded-*` để bypass verify.
5. **Pairing vs 077.** TTL 60m vs 10m; routes tách. Test không chéo mint `gv_` / approve sender.
6. **Open DM.** Telegram demo được `open`. Personal không default `open`. Production warn Personal/`open`.
7. **DI.** Live DI-05/07. DI-06 = sidecar+QR only. DI-01..04 parked. Không auto-fill token.
8. **SSRF.** Outbound chỉ host vendor allow (Telegram API, `openapi.zalo.me`). Sidecar URL nếu có phải document SSRF (066) — operator loopback sidecar: demo/SSRF-off tests.
9. **Secrets box.** Master key thiếu: QR session không persist encrypted (fail-closed persist hoặc env-only path documented). Env vẫn thắng.
10. **Lite.** Cấm Start = giảm bề mặt live trên bản Lite, không chỉ UX.

---

## 9. Test strategy

### 9.1 Unit / fake (CI bắt buộc)

Giữ 003/004/040/063/078. Thêm:

1. Policy matrix + Personal default allowlist trong demo; Telegram demo `open` cho phép.
2. Pairing: TTL 60m (test clock), 8 ký tự alphabet, max 3 pending, debounce; approve/deny; 077 untouched; view 403.
3. Binding + một instance / name.
4. PATCH token 400; overlay box không leak GET; env thắng box.
5. Health: empty `missing`; poll OK `running`; `GOSO_TELEGRAM_MODE=webhook` không `GOSO_PUBLIC_URL` → `failed` + error không silent; Phase-2 Slack `parked` dù 2 env set; catalog **7** tên, không `websocket`; `ws_up` trên healthz/stats.
6. OA: cả hai required cho `configured`; APP_ID optional; secret set sai → 401; secret missing demo accept+warn; production fail-closed.
7. TG webhook: sai `X-Goso-Telegram-Secret` → 401.
8. Personal: QR JSON không cookie; logout xóa box, env fixture vẫn `secret_set`; không import zca.
9. Lite: Start không chạy (fake manager).
10. Slack `env_names` chứa `GOSO_SLACK_BOT_TOKEN` và `GOSO_SLACK_APP_TOKEN`.
11. Grep GET body: không fixture token / `xoxb-` / imei.

Không `net.Dial` vendor trong unit.

### 9.2 Live smoke (gated; CI skip-always)

`scripts/e2e-channels-live.sh` (tên linh hoạt):

- CI / không flag / thiếu token → **exit 0**.
- Chỉ Dat local: flag + token không-commit; ephemeral `--port 0`; không demo ports; không transcript token.
- Không `make verify`.

### 9.3 QC (sau implement, không lúc draft)

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
gofmt -l gateway desktop
go vet ./...
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

---

## 10. Proposed LOCK answers (Recommended — chưa LOCK)

Bảng này **áp vào AC** ở trên. **Chưa** phải human LOCK. Dat approve nguyên bộ (hoặc sửa) rồi mới đổi STATUS.

| # | Câu hỏi gốc | Proposed answer (Recommended) |
|---|----------------|-------------------------------|
| 1 | Telegram default transport | `GOSO_TELEGRAM_MODE=poll\|webhook`, **default `poll`**. Giữ webhook route 003. Mode `webhook` **không** public URL → `health=failed`, error rõ, **không silent**. |
| 2 | `GOSO_PUBLIC_URL` | **Optional.** Dùng `setWebhook` khi `mode=webhook`. **Không** bắt buộc poll-only. |
| 3 | Zalo OA secret / app id | Required: `GOSO_ZALO_OA_ACCESS_TOKEN` **+** `GOSO_ZALO_OA_SECRET`. Optional: `GOSO_ZALO_OA_APP_ID`. Cập nhật `env_names[]`. Secret set → verify fail-closed. |
| 4 | OA poll vs webhook | **Webhook-first only** trong 084. **Không** OA long-poll. |
| 5 | DI-06 Personal protocol | **(c)+(b):** 084 = QR UI/API + risk banner + allowlist + logout + `secret_set`; live listener = **sidecar HTTP** operator; GOSO nhận inject/webhook như 004. In-process clean-room protocol = SPEC sau. **Cấm** copy zca-js / goclaw / CRM. |
| 6 | Secret overlay | **Cho phép** AES-256-GCM secrets box khi `GOSO_MASTER_KEY` có, key `channel:<name>:<kind>`. **Env thắng** nếu set. GET chỉ `secret_set`. **Không** PATCH plaintext tokens. |
| 7 | QR vs env | Env `GOSO_ZALO_PERSONAL_TOKEN` **thắng nếu set**; else secrets-box QR session. Logout **chỉ** xóa box, **không** process env. |
| 8 | Default policies demo/prod | Giữ §6. `GOSO_ENV=demo` **được** Telegram DM `open`. Personal **không** default `open` kể cả demo (chỉ PATCH). Production **warn** Personal/`open`. |
| 9 | Pairing TTL / alphabet | TTL **60 phút**, 8 ký tự loại `0O1IL`, max **3** pending per sender+channel, debounce **60s**. **Không** khớp 077 (10 phút). |
| 10 | Route pairing | `/api/channel-pairing` + approve **và** deny. **Không** nest `/api/channels/pairing`. 077 **không đổi**. |
| 11 | WebSocket catalog | **Không** thêm row `websocket`. Giữ **7** tên. WS health `/healthz` hoặc `/api/stats` (`ws_up`). |
| 12 | Telegram health probe | Start = một **`getMe`**; runtime = poll/webhook loop sống; re-probe **5 phút**; revoke → `failed` + redact. |
| 13 | Live smoke owner | **CI skip-always** đến khi Dat set flag+token **local** (không commit). Script **exit 0** nếu thiếu. |
| 14 | Slack `env_names` | Mở **ngay**: `GOSO_SLACK_BOT_TOKEN` + `GOSO_SLACK_APP_TOKEN`. Vẫn `phase=2`, `health=parked`, **không** live Start. |
| 15 | WhatsApp Phase-2 | **Giữ Cloud-API (040).** Native QR **DI-01 parked**; chỉ backlog. |
| 16 | `GOSO_LITE` | `=1` **cấm live Start** (không chỉ ẩn UI). Catalog vẫn list + `lite:true`. |
| 17 | Multi-instance | **Một instance / catalog name** trong 084. Multi-bot cùng platform = SPEC sau. |

**Chốt thêm (AC, không đánh số 1–17):**

| Extra | Proposed |
|-------|----------|
| Telegram webhook secret header | Goso-shaped **`X-Goso-Telegram-Secret`** (hoặc `?secret=` nếu configured). **Không** copy tên header GoClaw. |
| OA verify khi thiếu secret | Secret set → fail-closed. Secret **missing** → **demo accept + warn**; **production fail-closed**. |

---

## 11. Open questions (resolved by Proposed LOCK answers / still awaiting human)

Mọi mục [NEEDS CLARIFICATION] bản draft trước **đã có Proposed answer ở §10**. Không còn câu hỏi SPEC tự đoán.

| # | Status |
|---|--------|
| 1–17 + 2 extra | **Proposed LOCK (Recommended)** — chờ Dat **approved** / sửa. |
| Human LOCK | **Chưa.** STATUS dưới. Không cook cho đến khi Dat LOCK. |

Nếu Dat LOCK khác Recommended: sửa AC cho khớp câu Dat, không giữ Proposed im lặng.

---

## 12. Phase-2 (backlog — không cook trong 084)

- Discord / Feishu: env + `parked`; live SPEC sau.
- Slack: `env_names[]` **bot + app** ngay 084; vẫn `parked`, no Start.
- WhatsApp: **Cloud-API 040 retained**; native QR + STT E2E opt-in **sau DI-01**.
- Không claim Socket Mode / intents / Feishu cards / native WA.

Cùng một dòng `.planning/backlog.md`.

---

## 13. Implementation notes (sau human LOCK — không code lúc draft)

- `gateway/internal/channel` + manager self-written. Personal: webhook 004 + QR surface; **không** protocol in-process.
- Secrets box key `channel:<name>:<kind>`; env thắng.
- Tests-first: policy, pairing 60m, health webhook-without-URL, OA verify matrix, Slack two env names, 7 catalog names, no-leak.
- CP: `ChannelsPage.tsx`, i18n vi+en.
- Không đọc goclaw `.go`.
- Implement commit tương lai: `feat(channels): live mvp telegram ws zalo` — **không** làm ở task SPEC này.
- Nhánh: `admatrixmdp/spec084-channels-mvp-live`. Không merge.

---

STATUS: DRAFT — awaiting human LOCK
