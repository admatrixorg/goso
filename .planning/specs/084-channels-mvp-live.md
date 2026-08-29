# SPEC 084 — Channels MVP Live (Telegram + WebSocket + Zalo OA + Zalo Personal)

> DRAFT: 2026-08-29 — Advisor soạn. **Không LOCK** trừ khi operator (Dat) xác nhận.
> Clean-room. Không copy GoClaw / ZaloCRM. Không invent production tokens.
> Số **002 đã dùng** (`002-gateway-http-session.md`) — SPEC này là **084**.
> Không bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`.
> DI còn lại không liên quan (DI-08 search, DI-09 pgvector, DI-12/13/21 spawn, DI-19 SSO, …) **giữ parked**.

Mở khóa **live connect có kiểm soát** cho 4 Channel MVP (Decision #06). Adapter fake/inject (003/004/040) và config depth không-secret (063/078) giữ nguyên; SPEC này thêm vận hành live: secrets tách config, policy, Channel Pairing, ChannelBinding, health, QR, runbook.

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

Goso mapping (self-written, không copy schema): xem §8.

---

## 1. Goal

Operator có thể **connect thật** bốn Channel MVP — **Telegram + WebSocket + Zalo OA + Zalo Personal** — với trải nghiệm vận hành gần GoClaw: credentials tách khỏi config không-secret, `dm_policy` / `group_policy` / `allow_from` / `require_mention`, Channel Pairing (approve), ChannelBinding (channel↔Agent), health `missing | stopped | running | failed` (Phase-2 thêm `parked`), QR login cho Zalo Personal kèm cảnh báo unofficial, **không lộ token** trên API/log/Control Plane.

Discord / Slack / Feishu / WhatsApp: **checklist + config + health Phase-2**, không bắt buộc live production parity trong 084.

Kết thúc 084, `docs/qa/084-channels-mvp-live.md` ghi evidence; unit fake vẫn xanh khi không có live env; live smoke **default OFF** trong CI.

---

## 2. User Stories

- **US-01 Operator — Telegram live.** Đặt `GOSO_TELEGRAM_BOT_TOKEN` (placeholder name only; giá trị do operator tự cấp từ BotFather). Gateway Start Telegram. `GET /api/channels` hàng `telegram` có `configured: true`, `missing: false`, `health: running` (hoặc `failed` + `last_error` đã redact). Gửi DM từ tài khoản đã allow/pair → Session `telegram:<chat_id>` → Agent bound → reply qua Bot API. Token không xuất hiện trong JSON, log, hay UI.
- **US-02 Operator — WebSocket / internal chat.** Kết nối `GET /ws` (auth admin Bearer; Origin theo `GOSO_WS_ORIGINS`) `{op:"chat", payload:{session_id, message}}` → reply LLM. Control Plane Chat dùng cùng hợp đồng. Health phản ánh WS như internal Channel (không thêm adapter vendor).
- **US-03 Operator — Zalo OA live.** Đặt env OA (xem checklist §7). Webhook Zalo gọi `POST /api/channels/zalo-oa/webhook`. Verify (nếu secret có) fail-closed. Inbound text → Session `zalo-oa:<user_id>` → gửi lại OA send API. Default DM `pairing`; group `disabled` (OA không group).
- **US-04 Operator — Zalo Personal QR.** Mở Control Plane Channels → Zalo Personal → **Đăng nhập QR** + banner rủi ro unofficial/ban. Quét QR, session credential được giữ **tách** config JSON (env hoặc secrets box — [NEEDS CLARIFICATION] §11). Health `running` khi listener sống. Default `dm_policy=allowlist`, `group_policy=allowlist`.
- **US-05 Operator — policy + pairing + binding.** PATCH không-secret: `dm_policy`, `group_policy`, `allow_from`, `require_mention`, `agent_id`. Sender lạ khi `pairing` nhận mã; admin `POST .../approve` một lần. ChannelBinding trỏ Channel instance → Agent có sẵn (không tự tạo agent `telegram` nếu đã bind).
- **US-06 Engineer — fake vẫn độc lập.** `go test ./...` / `make verify` **không** gọi mạng vendor. Live smoke chỉ khi flag env (vd. `GOSO_LIVE_TELEGRAM=1`) và token operator; CI không set flag.
- **US-07 End-user.** User đã pair/allow nhắn trên Telegram hoặc Zalo; nhận câu trả lời text từ Agent bound. User chưa pair khi policy `pairing` chỉ nhận hướng dẫn mã, **không** chạy LLM. User Zalo Personal thấy bot chỉ trả lời nếu ID nằm allowlist (trừ khi operator đổi policy).

---

## 3. Acceptance Criteria

Checkbox đo được. Implementation sau LOCK; SPEC này khóa hành vi.

### 3.1 Framework / runtime readiness

- [ ] **AC-F1 Manager.** Gateway có Channel manager (goso-shaped, không copy GoClaw): với mỗi adapter MVP, `Start` khi credential đủ **và** `enabled` (default true khi configured), `Stop` lúc shutdown. Discord/Slack/Feishu/WhatsApp **không** Start live trong 084 (webhook inject stub 040 giữ cho unit).
- [ ] **AC-F2 Policy.** Trước khi gọi LLM, inbound đi `CheckPolicy`:
  - `dm_policy`: `pairing` | `allowlist` | `open` | `disabled`
  - `group_policy`: `open` | `allowlist` | `disabled` (không `pairing` ở group trừ khi LOCK chọn thêm)
  - `allow_from`: danh sách sender/chat ID (non-secret)
  - `require_mention`: bool; Telegram group default `true`
  - Reject → **không** gọi LLM; pairing path có thể outbound hướng dẫn (debounce ≥ 30s, đề xuất 60s)
- [ ] **AC-F3 Channel Pairing.** Khác SPEC 077 (view-token). Khi `dm_policy=pairing` và sender chưa pair/allow: phát mã một lần, TTL đề xuất 60 phút, alphabet loại ký tự dễ nhầm. Admin list pending + `approve`. Sau approve, inbound sau đó vào pipeline. Store hashed; GET không trả raw code sau lần phát cho end-user.
- [ ] **AC-F4 ChannelBinding.** Mỗi Channel instance có `agent_id` (nullable). Null → hành vi cũ: Agent synthetic theo `agent_key` = tên channel (003/004). Non-null → Session thuộc Agent đó. PATCH `agent_id` 404 nếu Agent không tồn tại. Binding là config JSON, không chứa secret.
- [ ] **AC-F5 Secrets vs config.** Secret (bot token, OA access token, OA secret, Zalo Personal session cookie/IMEI/…) **không** nằm trong hàng config JSON/sqlite non-secret. Nguồn: process env `GOSO_*` (canonical 078). PATCH token/secret field → **400** `channel tokens are env-only` (giữ 078). GET không bao giờ echo giá trị. [NEEDS CLARIFICATION] overlay `secrets` AES-256-GCM (`GOSO_MASTER_KEY`) — nếu LOCK cho phép, vẫn `secret_set: true/false` only.
- [ ] **AC-F6 Health fields.** `GET /api/channels` (và `/v1/channels`) mỗi hàng **bổ sung** (không phá field 078):

  | Field | Ý nghĩa |
  |-------|---------|
  | `name` `configured` `missing` `env` `env_names` | Giữ 063/078 |
  | `health` | `missing` \| `stopped` \| `running` \| `failed` \| `parked` |
  | `transport` | `webhook` \| `poll` \| `ws` \| `qr` \| `none` |
  | `secret_set` | bool (không phải giá trị) |
  | `bound_agent_id` | string, có thể rỗng |
  | `dm_policy` `group_policy` `require_mention` | non-secret |
  | `allow_from_count` | số phần tử; optional `allow_from` IDs (non-secret) |
  | `phase` | `1` (MVP live) hoặc `2` (checklist) |
  | `last_error` | public, đã redact token-like |

  Mapping: env thiếu → `health=missing` + `configured=false`. Credential đủ, chưa Start / `enabled=false` / Lite → `stopped`. Listener/webhook path OK → `running`. Auth/network/protocol lỗi sau Start → `failed`. Adapter Phase-2 không Start live → `parked` (kể cả khi env đầy).
- [ ] **AC-F7 Lite.** `GOSO_LITE=1`: không Start live MVP; catalog vẫn list + `"lite": true` (055/078). UI giữ câu Lite off.
- [ ] **AC-F8 PATCH non-secret.** `PATCH /api/channels/{name}` chấp nhận subset `{enabled?, dm_policy?, group_policy?, allow_from?, require_mention?, agent_id?}`. Unknown name 404. Token-like keys 400. Persist **config JSON** (sqlite table `channel_config` hoặc tương đương goso) — **không** table token plaintext.

### 3.2 Live Telegram

- [ ] **AC-T1** Credential: `GOSO_TELEGRAM_BOT_TOKEN` (đã có catalog). Optional verify: `GOSO_TELEGRAM_WEBHOOK_SECRET` — nếu set, webhook thiếu/sai header secret → 401, không LLM. [NEEDS CLARIFICATION] tên header (goso-shaped, không copy vendor internals mù).
- [ ] **AC-T2 Transport.** Webhook `POST /api/channels/telegram/webhook` **giữ** (003). Live Start **thêm** long-poll **hoặc** `setWebhook` tùy LOCK §11. Unit test không đụng mạng Telegram.
- [ ] **AC-T3** Inbound text → policy → Session label `telegram:<chat_id>` → Agent bound → `sendMessage` Bot API khi token có. Sender injectable trong test (003 giữ).
- [ ] **AC-T4** Group: `require_mention=true` default; message không mention (và không phải reply-to-bot) → bỏ qua, không LLM. DM theo `dm_policy`.
- [ ] **AC-T5** Default đề xuất: `dm_policy=pairing`, `group_policy=allowlist`, `require_mention=true`. [NEEDS CLARIFICATION] nếu demo muốn `open`.
- [ ] **AC-T6** Không STT / forum topic / HTML pipeline / 10+ bot commands (non-goal). Text in/out đủ AC.
- [ ] **AC-T7** Health `running` sau Start thành công (getMe hoặc poll/webhook register — cách đo [NEEDS CLARIFICATION]); token sai → `failed`, `last_error` không chứa token.

### 3.3 Live WebSocket / internal chat

- [ ] **AC-W1** `GET /ws` JSON `{op, payload}`: `ping`→`pong`, `chat`→reply (040) **giữ**. Auth: Bearer admin (006); view-token **không** chat (077). Origin: `GOSO_WS_ORIGINS` (040).
- [ ] **AC-W2** Decision #06 đếm WebSocket là 1/4 MVP. **Không** thêm hàng catalog thứ 8 tên `websocket` trừ khi LOCK. Health nội bộ: field trên `GET /healthz` hoặc `GET /api/stats` (`ws_up: true`) + QA ghi “internal chat live”. Control Plane Chat là surface operator.
- [ ] **AC-W3** Session phải tồn tại / thuộc Agent; thiếu `session_id` → error frame, không echo raw. Không lộ admin token trên WS log.
- [ ] **AC-W4** Không yêu cầu public URL. Không vendor token.

### 3.4 Live Zalo OA

- [ ] **AC-Z1** Required env tối thiểu: `GOSO_ZALO_OA_ACCESS_TOKEN`. [NEEDS CLARIFICATION] `GOSO_ZALO_OA_SECRET` / app id — 004 US nhắc secret; catalog 078 chỉ một tên. Nếu LOCK thêm, cập nhật `env_names[]`.
- [ ] **AC-Z2** `POST /api/channels/zalo-oa/webhook` parse payload 004 (`event_name` + `sender.id` **hoặc** `{user_id, message.text}`). Verify MAC/secret nếu secret có — fail-closed.
- [ ] **AC-Z3** DM only: group inbound bỏ qua (`group_policy` hiệu lực `disabled`). Default `dm_policy=pairing`.
- [ ] **AC-Z4** Outbound: OA send API (adapter đã có URL public; test vẫn httptest/inject). Live smoke mới được gọi mạng khi flag bật.
- [ ] **AC-Z5** Public URL **cần** cho webhook (operator tunnel/reverse-proxy). Poll OA [NEEDS CLARIFICATION] — GoClaw docs nói long poll; Zalo OA phổ biến là webhook. 084 mặc định **webhook-first** (khớp 004) trừ khi LOCK chọn poll.
- [ ] **AC-Z6** Health: missing token → `missing`; webhook mounted + token set → `running` cho đến khi send/auth fail → `failed`.

### 3.5 Live Zalo Personal (QR + risk)

- [ ] **AC-P1** Surface QR: Control Plane Channels (vi+en) nút “QR login” + **cảnh báo cố định**: protocol unofficial, tài khoản có thể bị khóa/ban, không dùng account chính công ty cho đến khi operator chấp nhận rủi ro. Log một dòng public `zalo-personal unofficial` (goso-shaped; không copy chuỗi GoClaw).
- [ ] **AC-P2** API hành vi: `GET`/`POST` goso-shaped (đề xuất `GET /api/channels/zalo-personal/qr`) trả `{status: pending|scanned|confirmed|expired|failed, expires_at}` + ảnh QR (png/data-URL) **không** kèm cookie/imei/token. Poll hoặc WS op `channel.qr` để cập nhật. Credential sau confirm chỉ vào secret store/env, không vào GET.
- [ ] **AC-P3** Default `dm_policy=allowlist`, `group_policy=allowlist`. Open DMs trên Personal là opt-in rõ (PATCH).
- [ ] **AC-P4** Webhook inject 004 **giữ** cho unit. Live path: listener sau QR/credential. Protocol cụ thể = DI-06 [NEEDS CLARIFICATION] (clean-room gateway vs CRM sidecar). 084 **cấm** copy `zca-js` / goclaw-source / CRM internals.
- [ ] **AC-P5** Env hiện có `GOSO_ZALO_PERSONAL_TOKEN` = session credential **nếu** operator đã login sẵn (không QR). QR và env cùng lúc: env thắng đến khi logout [NEEDS CLARIFICATION].
- [ ] **AC-P6** Logout: xóa secret session, health → `missing`/`stopped`, không để cookie trên disk plaintext.
- [ ] **AC-P7** Không rich sticker/image. Text in/out.

### 3.6 Observability / security

- [ ] **AC-S1** GET/PATCH/list/logs/traces/`last_error`/QR payload: không chứa token, cookie, `access_token`, `imei`, webhook secret. Test grep token-like (078 pattern) mở rộng live handlers.
- [ ] **AC-S2** Access log chỉ path + status; không query token, không Authorization.
- [ ] **AC-S3** Health states §3.1 AC-F6; CP Channels: badge `running` / `failed` / `missing` / `parked` (i18n vi+en). Không input password cho token (078 giữ).
- [ ] **AC-S4** Channel Pairing approve = admin Bearer. View-token 403.
- [ ] **AC-S5** Production (`GOSO_ENV=production`): Channel `dm_policy=open` trên Zalo Personal **cảnh báo** (log + UI); không cấm nếu operator cố ý.
- [ ] **AC-S6** Không ghi secret vào git / `.env.example` (placeholder rỗng). `scripts` secret-scan vẫn 0.

### 3.7 Docs / runbook / QA evidence

- [ ] **AC-D1** `docs/qa/084-channels-mvp-live.md` — cite table (paths only), mapping goso, lệnh, proof tests, non-goals, live smoke **skipped** khi flag tắt.
- [ ] **AC-D2** `docs/SETUP.md` + `docs/RUNBOOK.md` + `.env.example`: checklist §7, warning Zalo Personal, live flags default empty, “điền token trên máy operator — không commit”.
- [ ] **AC-D3** QC sau implement (không làm ở SPEC-draft): `cd control-plane && npm run typecheck`; `go test ./...`; `go build`; `gofmt`; `go vet`; agpl + agpl-docs 0. Không merge. Không start SPEC khác.
- [ ] **AC-D4** Phase-2 Discord/Slack/Feishu/WhatsApp: một mục runbook “chưa live 084; env names + health `parked`”. Backlog một dòng (file này + `.planning/backlog.md`).

---

## 4. Non-Goals

- Copy code / schema / tên header GoClaw (`X-GoClaw-*`, `/v1/channels` GoClaw-shaped mới). Gateway giữ `/api/channels` + alias `/v1` **đã có** (052).
- Live **full production parity** Discord / Slack / Feishu / WhatsApp (DI-02, DI-03, DI-04, DI-01). Stub 040 + config 078 + health `parked` là đủ. Native WhatsApp stack vs Cloud API = **DI-01 vẫn parked**.
- Telegram STT, forum topics, Markdown→HTML pipeline, voice routing, 10+ bot commands, group file-writer.
- `chat_behavior` multi-bubble / debounce inbound (trừ khi đã có sẵn trong pipeline — 084 không thêm).
- CRM / ZCRM / ZaloCRM bridge, reuse `goso-crm` inbound drain như Channel GOSO — trừ SPEC riêng. X3 CRM QR **không** được gọi là parity Channel 084.
- Ghi secret vào sqlite plaintext; PATCH token (078 cấm, 084 giữ).
- OAuth / SSO (DI-19), Apple, Stripe, K8s, Grafana Cloud, Tailscale, Redis, pgvector (DI-09), sandbox/browser/media spawn (DI-12/13/21), paid search (DI-08).
- Đổi SPEC 002 (HTTP/session) hay tái số 002.
- Invent production bot tokens / OA tokens / QR sessions trong repo, QA, hay CI.
- Bind/kill demo ports; merge; tự LOCK.
- MCP `/v1/channels` toggle phía goso-mcp (C10) — ngoài 084 trừ khi LOCK mở.
- Media/image outbound, OA template/broadcast (đã backlog 2026-08-19).

---

## 5. Dependencies / prior SPECs

| SPEC | Quan hệ |
|------|---------|
| **002** Gateway HTTP + Session + `/ws` echo (nền). 084 **không** sửa số 002; WS RPC đã thay echo ở **040**. |
| **003** Telegram webhook + LLM + injectable Sender. Live Start/poll/setWebhook **đứng trên** 003. |
| **004** Zalo OA + Zalo Personal webhook stubs. Live OA + QR **đứng trên** 004. |
| **005** SQLite persist — `channel_config` / pairing rows nếu cần. |
| **006** Admin Bearer + rate limit. Channel webhooks platform: **không** đòi admin Bearer (giữ 003); verify bằng platform secret. |
| **040** 7 adapters + WS `{op,payload}` + origin allowlist. 084 không xóa stub Discord/Slack/Feishu/WhatsApp. |
| **041 / 066** SSRF, injection, production fail-closed. Outbound Channel dùng HTTP client có timeout; không mở SSRF. |
| **055** Lite caps; Channels lite-off. |
| **063** `env` help names. |
| **067** Durable webhooks **LLM** (`/api/webhooks`) — **khác** platform Channel webhook. Không trộn. |
| **069** Health chrome gateway — 084 thêm health **per-channel**, không thay `healthz` kind. |
| **077** View-token pairing (`POST /api/pairing`). **Không** tái sử dụng làm Channel DM pairing. Hai khái niệm, hai route. |
| **078** `configured` / `missing` / `env_names`; PATCH cấm secret. 084 **mở** PATCH non-secret; **không** lật cấm secret. |
| Decision **#06** MVP 4 Channel. Decision **#01** clean-room. |
| DI-05, DI-06, DI-07 = live MVP 084 (operator cung cấp credential). DI-01..04 = Phase-2. |

---

## 6. Credential & setup checklist

Placeholder **tên env** only. Giá trị để trống trong git.

| Channel | Required secrets (names) | Optional | Transport 084 | Public URL | Default policies (đề xuất) | Phase |
|---------|--------------------------|----------|---------------|------------|----------------------------|-------|
| **telegram** | `GOSO_TELEGRAM_BOT_TOKEN` | `GOSO_TELEGRAM_WEBHOOK_SECRET` | webhook **và/hoặc** long-poll — [NEEDS CLARIFICATION] default | **Cần** nếu webhook; **không** nếu chỉ poll | DM `pairing`; group `allowlist`; `require_mention=true` | 1 live |
| **websocket** (internal) | không vendor; `GOSO_ADMIN_TOKEN` cho `/ws` | `GOSO_WS_ORIGINS` | `GET /ws` | không | n/a (auth gateway) | 1 live |
| **zalo-oa** | `GOSO_ZALO_OA_ACCESS_TOKEN` | `GOSO_ZALO_OA_SECRET` (+ app id?) [NEEDS CLARIFICATION] | webhook (004); poll chỉ nếu LOCK | **Cần** (webhook) | DM `pairing`; group `disabled` | 1 live |
| **zalo-personal** | `GOSO_ZALO_PERSONAL_TOKEN` **hoặc** QR session secret | — | QR + listener unofficial | không (QR local/CP) | DM `allowlist`; group `allowlist` | 1 live + risk |
| **discord** | `GOSO_DISCORD_BOT_TOKEN` | intents / app id — Phase-2 | stub webhook 040 | n/a 084 | n/a | 2 parked |
| **slack** | `GOSO_SLACK_BOT_TOKEN` | `GOSO_SLACK_APP_TOKEN`, user token, signing secret — Phase-2 (GoClaw Socket Mode cần bot+app) | stub 040 | n/a 084 (Socket Mode không cần public URL) | n/a | 2 parked |
| **feishu** | `GOSO_FEISHU_APP_SECRET` | `GOSO_FEISHU_APP_ID`, encrypt key, domain China vs intl — Phase-2 | stub 040 | webhook mode cần; WS mode không | n/a | 2 parked |
| **whatsapp** | `GOSO_WHATSAPP_ACCESS_TOKEN` | phone number id, verify token; **native vs Cloud = DI-01** | Cloud-API stub 040 | Cloud webhook **cần**; native QR thì không | n/a | 2 parked |

Live smoke flags (default **empty/OFF**):

| Flag | Effect |
|------|--------|
| `GOSO_LIVE_TELEGRAM=1` | Smoke Telegram getMe + 1 inbound/outbound (operator token) |
| `GOSO_LIVE_ZALO_OA=1` | Smoke OA send to allowlisted test user |
| `GOSO_LIVE_ZALO_PERSONAL=1` | Smoke QR status endpoint (không bắt buộc quét trong CI) |
| `GOSO_LIVE_CHANNELS=1` | Bật cả ba trên (vẫn skip từng kênh nếu thiếu env) |

`make verify` **không** export các flag này.

---

## 7. Data model / API surface (hành vi — không copy schema GoClaw)

Thuật ngữ (bổ sung tại chỗ; glossary hiện có **Channel**, **Agent**, **Session**):

| Thuật ngữ | Định nghĩa GOSO |
|-----------|-----------------|
| **InboundMessage** | Bản tin đã chuẩn hoá sau adapter: `channel`, `sender_id`, `chat_id`, `peer_kind` (`direct`\|`group`), `text`, `mention`, `session_label`. |
| **ChannelBinding** | Liên kết `channel_name` → `agent_id`. |
| **Channel Pairing** | Approve sender trên Channel khi `dm_policy=pairing`. **Không** phải 077 view-token pairing. |

### 7.1 Config JSON (non-secret)

Hàng per catalog name (7 + không bắt buộc internal ws):

```
channel_config(
  name,                 -- PK, catalog name
  enabled,              -- bool
  agent_id,             -- nullable FK logical
  dm_policy,            -- pairing|allowlist|open|disabled
  group_policy,         -- open|allowlist|disabled
  require_mention,      -- bool
  allow_from,           -- JSON array of sender/chat ids
  updated_at
)
```

Không cột token. Migration ALTER-safe trên SQLite hiện có.

### 7.2 Credentials

- Đọc lúc Start: `os.Getenv` theo `env_names[]`.
- Optional overlay: `secrets` box 067/041 (`GOSO_MASTER_KEY`) keyed `channel:<name>:<kind>` — **chỉ nếu LOCK** §11. GET: `secret_set` only.

### 7.3 Channel Pairing rows

```
channel_pairing(
  id, channel, sender_id, code_hash, status, -- pending|approved|expired
  expires_at, created_at, approved_at
)
```

Code plaintext chỉ lúc tạo (gửi cho end-user qua Channel). Admin list: `id`, `channel`, `sender_id`, `status`, `expires_at` — **không** `code`.

Khác 077: alphabet/TTL có thể giống *hành vi* (8 ký tự, 60 phút) nhưng **bảng và route khác**.

### 7.4 HTTP (goso-shaped)

Giữ:

| Method | Path | 084 |
|--------|------|-----|
| GET | `/api/channels` | Thêm health/policy/binding fields (§3.1). Không secret. |
| PATCH | `/api/channels/{name}` | Non-secret only. |
| POST | `/api/channels/{telegram,zalo-oa,zalo-personal,discord,slack,feishu,whatsapp}/webhook` | Giữ inject; Telegram/OA thêm verify nếu secret set. |
| GET | `/ws` | Giữ 040. |
| POST | `/api/pairing` + `/exchange` | **077 không đổi.** |

Thêm (tên có thể chỉnh lúc LOCK, hành vi khóa):

| Method | Path | Ai | Body / response |
|--------|------|----|-----------------|
| GET | `/api/channels/{name}/health` | admin+view | Một hàng health (trùng field list). |
| GET | `/api/channel-pairing` | admin | `{items:[{id,channel,sender_id,status,expires_at}]}` |
| POST | `/api/channel-pairing/{id}/approve` | admin | `{ok:true}` — 404 unknown, 409 expired. |
| POST | `/api/channel-pairing/{id}/deny` | admin | optional; 084 tối thiểu approve. |
| GET | `/api/channels/zalo-personal/qr` | admin | `{status, expires_at, image_png_base64?}` — no session secret. |
| POST | `/api/channels/zalo-personal/logout` | admin | Xóa session secret. |

View-token: GET health/list OK; approve/QR/logout/PATCH **403**.

### 7.5 Session labels (giữ)

`telegram:<chat_id>`, `zalo-oa:<user_id>`, `zalo-personal:<thread_id>`, `discord:<channel_id>`, `slack:<channel_id>`, `feishu:<chat_id>`, `whatsapp:<from>`.

### 7.6 Control Plane

Channels page (078): thêm cột health, policy tóm tắt, bound agent picker, pending pairing count, Zalo Personal QR panel + risk banner. Không ô nhập token. i18n vi+en. StatusLine loading/empty/error (046).

---

## 8. Security & risk notes

1. **Zalo Personal unofficial.** Protocol reverse-engineered. Account **có thể bị lock/ban**. SPEC bắt buộc banner UI + log public + default allowlist. Không khuyến nghị dùng Zalo cá nhân lãnh đạo/công ty cho đến khi operator chấp nhận. Clean-room: tự viết adapter; không copy zca-js/goclaw/CRM.
2. **WhatsApp STT / E2E (Phase-2 note).** GoClaw docs: voice WhatsApp E2E; STT default OFF vì gửi audio ra STT phá E2E. Nếu Phase-2 native WhatsApp bao giờ live: STT **opt-in**, default `[Voice message]`, không bật ngầm. 084 **không** implement STT.
3. **Token leakage.** Mọi handler mới nằm trong test “JSON/log không khớp token fixture”. `last_error` cắt/redact.
4. **Webhook public.** Telegram/OA webhook là mặt public: verify secret, timeout, body cap, không tin `X-Forwarded-*` để bypass. Admin API khác platform webhook.
5. **Pairing vs 077.** Nhầm route = lỗ hổng (view grant vs DM allow). Test: `POST /api/pairing/exchange` không approve Channel sender; `POST /api/channel-pairing/.../approve` không mint `gv_`.
6. **Open DM.** `dm_policy=open` trên Telegram tiện demo, nguy hiểm production (LLM cost + prompt injection). Default pairing; production Personal không default open.
7. **DI còn lại.** 084 chỉ mở live **DI-05/06/07** (OA / Personal / Telegram) khi operator tự điền env. Không auto-fill. DI-01..04 parked Phase-2. Không đụng DI-08+.
8. **SSRF.** Channel outbound chỉ host vendor allow (Telegram API, `openapi.zalo.me`). Không lấy URL từ user để SSRF. `GOSO_SSRF` không chặn những host đó.
9. **Ban risk ≠ legal copy.** Cảnh báo vận hành không thay thế clean-room: vẫn cấm paste source GoClaw.

---

## 9. Test strategy

### 9.1 Unit / fake (bắt buộc, CI)

Giữ toàn bộ test 003/004/040/063/078. Thêm:

1. Policy matrix: mỗi `dm_policy`/`group_policy` × mention/no-mention → LLM gọi / không gọi (inject Sender, Echo).
2. Pairing: sender lạ → không LLM; approve → lần sau LLM; expired → 409; view-token approve 403; 077 routes untouched.
3. Binding: PATCH `agent_id` có thật → session.AgentID khớp; id lạ 404; null → synthetic key cũ.
4. PATCH token 400 + sqlite `secrets` không tăng (078). PATCH `dm_policy` persist, GET trả policy, không token.
5. Health: env empty `missing`; env set + Start fake OK `running`; Start error `failed` + `last_error` không chứa fixture token.
6. Catalog vẫn **7** tên; Phase-2 `phase=2`, `health=parked` khi không Start.
7. QR: GET qr không chứa cookie; logout xoá secret_set.
8. WS chat 040 + origin + view-token không upgrade chat (nếu chưa cover).
9. Grep GET `/api/channels` body: không khớp `123:ABC` / `xoxb-` / `imei` fixtures.

Không `net.Dial` tới `api.telegram.org` / `openapi.zalo.me` trong unit. Sender/HTTPClient injectable / httptest.

### 9.2 Contract / live smoke (gated, default OFF)

Script đề xuất `scripts/e2e-channels-live.sh` (tên không quan trọng):

- Nếu không flag → **exit 0 skip** (giống `e2e-router9.sh` 055).
- Nếu flag + thiếu token → skip kênh đó, không fail CI.
- Nếu flag + token: ephemeral gateway `--port 0` (không đụng demo ports); assert health `running` hoặc document `failed` thật; **không** commit transcript có token.
- Không phần `make verify`.

### 9.3 QC lệnh (sau implement, không phải lúc draft)

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

## 10. Open questions

Đánh dấu **không đoán**. LOCK phải trả lời trước khi cook.

1. **[NEEDS CLARIFICATION] Telegram default transport.** Webhook (003, cần public URL) hay long-poll (GoClaw docs, không cần URL) hay cả hai (poll fallback khi chưa set webhook)? Đề xuất SPEC: webhook giữ + long-poll Start khi token có **và** `GOSO_TELEGRAM_MODE=poll|webhook` (default `poll` cho local, `webhook` khi `GOSO_PUBLIC_URL` set) — **chưa khóa**.
2. **[NEEDS CLARIFICATION] `GOSO_PUBLIC_URL`.** Có cần env này để `setWebhook` không? Tên? Bắt buộc production?
3. **[NEEDS CLARIFICATION] Zalo OA secret/app id.** Catalog hiện chỉ `GOSO_ZALO_OA_ACCESS_TOKEN`. Có bắt buộc `GOSO_ZALO_OA_SECRET` (004 US-01) để verify webhook? App ID?
4. **[NEEDS CLARIFICATION] Zalo OA poll vs webhook.** 084 đề xuất webhook-first. Operator có muốn long-poll official API không?
5. **[NEEDS CLARIFICATION] DI-06 Zalo Personal protocol.** (a) Clean-room listener trong `gateway/internal/channel` — chậm, đúng clean-room; (b) Sidecar HTTP do operator tự host, GOSO chỉ QR+webhook inject — nhanh, phụ thuộc ngoài; (c) Trì hoãn live Personal, 084 chỉ QR **surface** + risk + allowlist, listener Phase-2. **Không** được copy CRM `zca-js`.
6. **[NEEDS CLARIFICATION] Secret overlay.** Env-only (khớp 078 tuyệt đối) hay cho phép `secrets` AES-256-GCM khi `GOSO_MASTER_KEY` có, để QR session sống qua restart mà không nhét cookie vào `.env`?
7. **[NEEDS CLARIFICATION] QR vs env precedence** khi cả `GOSO_ZALO_PERSONAL_TOKEN` và QR session cùng có.
8. **[NEEDS CLARIFICATION] Default policies production vs demo.** Đề xuất §6; demo `GOSO_ENV=demo` có được `open` Telegram DM không? Personal **không** open dù demo?
9. **[NEEDS CLARIFICATION] Channel Pairing TTL / alphabet / max pending.** Đề xuất 60 phút / 8 ký tự / max 3 pending / debounce 60s (hành vi công khai). Có đổi cho khớp 077 (10 phút view-code) để operator đỡ nhầm?
10. **[NEEDS CLARIFICATION] Route names** `/api/channel-pairing` vs lồng `/api/channels/pairing`. Tránh đụng 077.
11. **[NEEDS CLARIFICATION] WebSocket catalog row.** Decision #06 liệt kê WS là Channel MVP nhưng catalog 7 tên không có `websocket`. Giữ internal hay thêm hàng?
12. **[NEEDS CLARIFICATION] Health probe Telegram.** `getMe` lúc Start hay “poll loop sống”? Fail token revoke giữa chừng — interval?
13. **[NEEDS CLARIFICATION] Live smoke owner.** Bot/OA/Personal test account do Dat cấp ngoài git. Có skip-always cho đến khi Dat gửi token vào env local không-commit?
14. **[NEEDS CLARIFICATION] Phase-2 Slack env.** 078 một tên `GOSO_SLACK_BOT_TOKEN`; Socket Mode thực tế cần app token. 084 chỉ **ghi checklist** hay mở rộng `env_names[]` ngay (vẫn `parked`)?
15. **[NEEDS CLARIFICATION] WhatsApp Phase-2.** Giữ Cloud-API (040) hay reopen DI-01 native QR? 084 không implement; cần câu LOCK cho backlog.
16. **[NEEDS CLARIFICATION] GOSO_LITE + live.** Cấm Start (đề xuất) hay Lite chỉ ẩn UI?
17. **[NEEDS CLARIFICATION] Binding nhiều instance cùng platform** (hai bot Telegram). 084 đề xuất **một instance per catalog name** (đủ MVP). Nhiều instance = SPEC sau.

---

## 11. Phase-2 (backlog — không cook trong 084)

Discord, Slack, Feishu, WhatsApp:

- Điền env, `env_names[]` đủ field (Slack bot+app, Feishu app_id+secret, WhatsApp Cloud verify token / phone id).
- Health `parked` → khi SPEC sau LOCK: Start thật, policy, pairing, binding giống khung 084.
- WhatsApp native QR + STT E2E opt-in **chỉ** sau DI-01.
- Không claim parity GoClaw (Socket Mode, privileged intents, Feishu card stream, …) cho đến SPEC riêng.

Một dòng cũng ghi ở `.planning/backlog.md`.

---

## 12. Implementation notes (sau LOCK — không code lúc draft)

- Package goso: `gateway/internal/channel` (adapters đã có) + manager mới cùng package hoặc `channel/manager.go` self-written.
- Tests-first: policy + pairing + health + no-leak **trước** live HTTP.
- CP: `ChannelsPage.tsx` + `i18n/vi.ts` + `en.ts`.
- Không đọc goclaw `.go`. Cite docs path trong QA.
- Commit implement (tương lai): `feat(channels): live mvp telegram ws zalo` — **không** làm ở task SPEC.
- Nhánh hiện tại: `admatrixmdp/spec084-channels-mvp-live`. Không merge.

---

STATUS: DRAFT — awaiting human LOCK
