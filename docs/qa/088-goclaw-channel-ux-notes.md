# QA notes — GoClaw dashboard channel credential UX (behavior only)

Date: 2026-08-29. Source: live Dewee Dashboard at `http://127.0.0.1:18791` (process `goclaw listen 18791`). **Behavior study only.** Do not copy GoClaw / dashboard source (CC-BY-NC). No password, gateway token, or bot token is recorded here.

## 1. Login (operator credentials)

- URL: `/login`. Title: **Dewee Dashboard**.
- Two modes: **Token** vs **Pairing**.
- Token mode fields: **User ID** + **Gateway Token**. Token input is `type=password` (masked). Copy on the form: “Use system for full system access”.
- Pairing mode: request access; an admin approves. **No gateway token field.** This is **device / dashboard pairing**, not channel DM pairing.
- File `/tmp/goclaw-orb-creds.txt` (`user` + `password`) is **not** the dashboard login. That pair returned “Invalid credentials”. Login succeeded with User ID `system` + the process `GOCLAW_GATEWAY_TOKEN` (value not recorded).
- After login: origin `/overview`. Sidebar groups CORE / CONVERSATIONS / CONNECTIVITY / CAPABILITIES / DATA / MONITORING / SYSTEM. Channels sits under CONNECTIVITY; Providers and API Keys under SYSTEM.

## 2. Channels list

- Heading **Channels**, subtitle **Manage channel instances**.
- Actions: **Add Channel**, **Refresh**, search box `Search channels...`.
- List is **instances**, not a fixed 7-name catalog. This host showed two ZaloCRM Personal-bridge rows (Disabled / Running + Attention). Each row has a **NEXT STEP** (Inspect issue) and opens a detail page.
- Health chrome: Running / Disabled / Attention, last-checked relative time, a one-line diagnosis. No plaintext secrets on the list.

## 3. Add Channel — Telegram (primary)

Dialog **Create Channel Instance**:

| Field | Behavior |
|-------|----------|
| Key * | slug, placeholder `my-telegram-bot` |
| Display Name | optional |
| Channel Type * | combobox; default **Telegram** |
| Agent * | required bind |
| **Credentials → Bot Token *** | `input type=password` `id=ci-cred-token`, placeholder `123456:ABC-DEF...`. Helper: “From @BotFather”. **No reveal/eye control.** |
| Helper under the group | **“Encrypted server-side. Never returned in API responses.”** |
| Configuration | API Server URL, HTTP Proxy, DM Policy default **Pairing (require code)**, Group Policy default **Pairing (require approval)**, mention switch, streaming, media, allow-from, human-like delivery… |
| Enabled | switch, default on |
| Actions | Cancel / Create / Close |

Operator flow: paste Bot Token → Create. Token never comes back on later GET.

Channel types in the combobox (this build): Bitrix24, Discord, Facebook, Feishu / Lark, Pancake (pages.fm), Slack, Telegram, WhatsApp, Zalo OA, Zalo Personal, ZaloCRM (Zalo Personal bridge).

## 4. Other create forms (credentials)

- **Discord:** Credentials group, **Bot Token *** `type=password`, same “Encrypted server-side. Never returned…” line.
- **Zalo OA:** Credentials: **Bot Token *** + **Webhook Secret**. Same encrypted/never-returned helper. Configuration includes DM Policy (Pairing require code) and Webhook URL.
- **Zalo Personal:** **no Credentials group**. Policy + delivery only. Login is a later QR / unofficial-protocol path, not a bot-token field.

## 5. Existing instance — Credentials tab (write-only update)

URL pattern `/channels/<uuid>`. Tabs: General / **Credentials** / Contexts / Managers.

Credentials panel copy (ZaloCRM bridge instance):

- “Leave fields blank to keep current values. Credentials are encrypted server-side and never returned in API responses.”
- Password field empty (no current value, no mask of stored secret, no reveal).
- Label pattern: “\<secret name\> (leave blank to keep current)”.
- Submit: **Update Credentials**.

Health on the same page: Running / Attention, “What happened”, recommended action, last checked / last healthy. That is the **connect/health** surface — not a separate “show me the token” control.

## 6. Providers / API Keys (same write-only pattern)

- Providers list: **API key set** or **Connected** badges. Never the key.
- Add Provider: **API Key** `type=password` placeholder `sk-...`. Create stores it; list still says “API key set”.
- Channel secrets and LLM API keys share the same operator contract: write in a password field, persist encrypted, GET only a boolean.

## 7. Pairing vs device pairing (do not mix)

| Kind | Where | What the operator does |
|------|--------|-------------------------|
| Dashboard / device pairing | `/login` Pairing tab | Request access; admin approves. No gateway token. |
| Channel DM pairing | Channel Configuration **DM Policy = Pairing (require code)** | End-user must send a code; admin approves the sender. Independent of dashboard login. |

goso already split these (077 device/view-token vs 084 channel pairing). UI must keep them separate.

## 8. What goso Channels lacked (pre-088)

Control plane listed env var **names** and said “no secret fields”. Operator had nowhere to type a Telegram bot token. GoClaw’s operator path is: **Add Channel → Credentials → Bot Token (password) → Create**, then later **Credentials tab → empty password → Update** (blank = keep). GET never returns plaintext.

## 9. goso mapping (self-written; no schema copy)

- Write-only form on Channels for **telegram** (bot token) and **zalo-oa** (access token + app secret). **zalo-personal** stays QR (084); no bot-token field.
- Persist AES-GCM secrets-box `channel:<name>:<kind>` when `GOSO_MASTER_KEY` is set. Process env still wins if set.
- `GET /api/channels` remains names + `secret_set` / `from_env` / health. No token keys.
- `PATCH` keeps rejecting token fields (078). New `PUT /api/channels/{name}/secrets` is the write path.
- Connect/test: Telegram `getMe`; health badge updates. Phase-2 channels stay parked (no secret form).
