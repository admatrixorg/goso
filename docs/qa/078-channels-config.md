# QA — SPEC 078 Channel config depth (no live tokens)

Date: 2026-08-29. Clean-room Go/React. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not fill or invent live channel tokens (DI-01..07 parked). Do not merge. Do not start SPEC 079.

Closes matrix **C1–C7** UI/config completeness (adapters already 040).

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Per-channel config field **names** (Slack `bot_token` / `app_token` / `user_token`; Feishu connection fields; Telegram topic `enabled`; seven platform adapters) | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/05-channels-messaging.md` |

goso mapping (self-written): `GET /api/channels` lists the same seven adapter names with `configured` (all required env non-empty), `missing: true` when any required env is empty, and public `env_names[]` (variable names only). `env` remains the first required name (063). `PATCH /api/channels/{name}` refuses token/secret fields and does not write sqlite (`secrets` stays empty; no channel table). Control-plane Channels shows the required env list and configured/missing badges. No secret inputs. Live tokens stay DI-01..07.

## What changed

- `GET /api/channels` (and `/v1/channels` alias) each row is `{name, configured, missing, env, env_names}`. Empty env → still listed, `configured: false`, `missing: true`. JSON never includes token values.
- `PATCH /api/channels/{name}`: unknown name 404; any token/secret field 400 `channel tokens are env-only`; `enabled` is **not** accepted (not already in catalog). Handler has no store handle — cannot persist secrets.
- Control-plane Channels: required env list (`env_names`), configured badge + missing badge, copy copies the **name**. No password/token inputs. i18n vi+en.
- Tests: seven names; empty env `configured=false` + `missing=true`; GET body grep rejects token-like values; PATCH does not insert sqlite `secrets`.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 079.

## Proof (2026-08-29)

- `cd control-plane && npm run typecheck` → exit 0 (`tsc --noEmit`)
- `go test ./...` → exit 0. New/updated: `TestCatalog_SevenNamesUnconfigured` (seven names, empty env `configured=false` `missing=true` `env_names[]`); `TestCatalog_ConfiguredWhenEnvSet` marshals JSON and rejects token value; `TestChannelsAPI_ListsSeven`; `TestChannelsAPI_JSONOmitsTokenValue` plus token-like grep; `TestChannelsAPI_PatchDoesNotWriteSecrets` (400, sqlite `secrets` count 0, no channel table).
- `go build -o bin/goso-gateway ./gateway/cmd/goso-gateway` → exit 0
- `gofmt -l gateway desktop` empty; `go vet ./...` exit 0
- `GOSO_ROOT=$PWD …/agpl-check.sh` → exit 0
- `./scripts/agpl-check-docs.sh` → exit 0
- Control-plane: `ChannelsPage` lists `env_names`, configured + missing badges, `channels.noSecrets`. No `<input>` for tokens. i18n en+vi.
- Ports `:8082` `:8091` `:3000` `:18080` `:18088` not bound/killed. No live tokens in git.

## Non-goals

Live Discord/Telegram/Slack/Feishu/WhatsApp apps (DI-01..07). SQLite overlay of tokens. `enabled` bool (not already in catalog). Copying goclaw Go. Merge. SPEC 079. Binding/killing demo ports. Product secrets in git.
