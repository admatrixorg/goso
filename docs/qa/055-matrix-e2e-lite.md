# QA — SPEC 055 matrix refresh + optional router9 e2e + Lite channel hide

Date: 2026-08-28. Clean-room. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed.

Last remaining-queue item. Historical 034 snapshot stays at `docs/qa/034-goclaw-parity-matrix.md`.

## What changed

1. **Matrix appendix** `docs/qa/034-goclaw-parity-update-054.md`: row id | original 034 status | **now** after 035–054 | evidence. Closed rows marked CÓ/PARTIAL honestly. Parked DIs stay parked.
2. **Optional e2e** `scripts/e2e-router9.sh`: if `GOSO_ROUTER9_BASE_URL` unset or `GET /v1/models` not 200 → **skip exit 0**. If up: ephemeral gateway (`--port 0`), `POST /api/chat` with admin Bearer + default model `ocg/deepseek-v4-flash`, assert HTTP **200** and non-empty `reply`. No secrets in the script. Documented in `docs/SETUP.md`. Not part of `make verify`.
3. **Lite channel hide:** `GOSO_LITE=1` → `GET /api/channels` `{channels:[…], lite:true}` (adapters still listed). Control-plane Channels page shows one-line i18n `channels.liteOff` = “Lite: channels off” and does not render the catalog. Adapters are not deleted.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
# optional; skip 0 when router down:
./scripts/e2e-router9.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- Appendix file exists; original 034 still present and not rewritten.
- `TestChannelsAPI_LiteFlag`: unset → `lite: false` and 7 names; `GOSO_LITE=1` → `lite: true` and 7 names still listed.
- i18n: `channels.liteOff` in `control-plane/src/i18n/en.ts` and `vi.ts` (`MsgKey` typecheck).
- `scripts/e2e-router9.sh` skip path: empty `GOSO_ROUTER9_BASE_URL` exits 0.

## Non-goals

Merging, live bind/kill of demo ports, copying goclaw/ZaloCRM, rewriting 034 in place, deleting channel adapters.
