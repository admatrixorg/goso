# QA — SPEC 063 Channels: show env names (no secrets)

Date: 2026-08-28. Clean-room React/Go. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not break env `router9` + default `ocg/deepseek-v4-flash`. **No channel tokens in git, UI, or GET body.**

Today `GET /api/channels` returned `{channels:[{name,configured}], lite}`. `configured` is env-token non-empty (`gateway/internal/channel/catalog.go` `tokenEnv`). UI was a yes/no table. User could not see which env to set. No PATCH/sqlite overlay (parked — like 056 for LLM only).

## Wired

- `GET /api/channels` (and `/v1/channels` alias) each catalog row includes public `env` string — the **variable name** only (`GOSO_TELEGRAM_BOT_TOKEN`, `GOSO_ZALO_PERSONAL_TOKEN`, `GOSO_ZALO_OA_ACCESS_TOKEN`, `GOSO_DISCORD_BOT_TOKEN`, `GOSO_SLACK_BOT_TOKEN`, `GOSO_FEISHU_APP_SECRET`, `GOSO_WHATSAPP_ACCESS_TOKEN`). Never the value.
- `configured` still true only when `os.Getenv(tokenEnv[name]) != ""`.
- Control-plane table: name, configured (yes/no badge), env var (monospace). Copy copies the **name**, not a token. `StatusLine` still loading/error only.
- Note `channels.envOnly`: channels are env-only (variable names shown, never tokens). Lite still replaces the table with `channels.liteOff`.
- i18n vi+en.

## Not wired (no API / out of scope)

- PATCH channel tokens, SQLite overlay, UI save of secrets.
- Live Discord/Telegram/Slack/Feishu/WhatsApp apps.
- Deleting adapters.
- SPEC 064.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o /tmp/goso-gateway-063 ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Non-goals

Saving tokens from UI, SQLite overlay, live adapters, deleting adapters, SPEC 064, binding/killing demo ports.

## Proof (2026-08-28)

- `cd control-plane && npm run typecheck` → exit 0 (`tsc --noEmit`, no errors)
- `go test ./...` → exit 0 (channel + httpapi + all gateway pkgs). New: `TestCatalog_SevenNamesUnconfigured` asserts `env` names; `TestCatalog_ConfiguredWhenEnvSet` marshals JSON and rejects token value; `TestChannelsAPI_ListsSeven` asserts 7 `env` names; `TestChannelsAPI_JSONOmitsTokenValue` sets a distinctive telegram token and fails if GET body contains it.
- `go build -o /tmp/goso-gateway-063 ./gateway/cmd/goso-gateway` → exit 0
- `GOSO_ROOT=$PWD …/agpl-check.sh` → exit 0 (`OK`; no banned author ids outside `.planning`)
- Diff: `gateway/internal/channel/catalog.go` (`Info.Env`), catalog + `handlers_webhooks_test.go`, `control-plane/src/pages/ChannelsPage.tsx`, `control-plane/src/api/channels.ts`, `control-plane/src/api/client.ts`, `control-plane/src/i18n/en.ts`, `control-plane/src/i18n/vi.ts`, this QA file. No DEMO page diffs. No vite/dev server. Ports `:8082` `:8091` `:3000` `:18080` `:18088` not bound/killed.
- Source (no live UI — demo ports reserved): table columns name / configured / env var (monospace). Copy writes `c.env` (variable name) via `navigator.clipboard`. Lite path still `channels.liteOff` (no table). `StatusLine` loading/error unchanged. No PATCH/sqlite overlay.
- i18n en+vi new keys: `channels.col.env`, `channels.envOnly`, `channels.copied`. Existing `channels.liteOff` unchanged.
- Env router9 + default `ocg/deepseek-v4-flash` untouched (no llm/provider files in diff).
- No channel token values in git or GET JSON (tests use `test-placeholder` / `must-not-appear-in-get-body` and assert they do not appear in marshaled catalog / GET body).
