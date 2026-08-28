# QA — SPEC 069 Health chrome + polish + CTO-09

Date: 2026-08-28. Clean-room Go/React. No ZaloCRM / goclaw copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Closes **CTO-08**, **CTO-09**, and **CTO-11** leftover English in `control-plane/src/i18n/vi.ts` (locked polish scope). Hardcoded Connectors placeholders stay out of this spec. Do not merge. Do not start SPEC 070.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Mode 1 personal dashboard is the live operational surface (login + gateway as truth) | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/23-multi-tenant-architecture.md` (Mode 1 dashboard/health) |
| System health is an unauthenticated HTTP probe, not a static chrome decoration | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/18-http-api.md` (`GET /health` system endpoint) |

goso mapping (self-written): control-plane chrome probes `GET /healthz` on load and on a 2s→15s backoff interval. `healthKind(status, ok)` maps 200+`ok` → connected, non-200 body → degraded, network/timeout (`status<=0`) → offline, 401/403 → unauthorized. The pill is not green unless the probe is connected. i18n vi+en.

## What changed

- Chrome: `GatewayStatus` calls `probeHealthz()` immediately, then retries with backoff 2s→15s (2/4/8/15 on failure; 15s while connected). Dot color: green / orange / red / gray-before-first-result. Pulse only when connected.
- `healthKind` (TS chrome) + equivalent `health.Kind` (Go, same mapping, covered by `go test`): 200/ok, non-200, 401/403, network.
- CTO-11: real leftover English in `control-plane/src/i18n/vi.ts` (crm empty/offline, cron session/message labels, teams links, channels lite-off, connector pick labels). Product terms (Functions, Memory, Traces, column codes) left as-is.
- CTO-09: `docs/qa/014-qa.md` banned author ids replaced with `upstream-author`. `scripts/agpl-check-docs.sh` scans `docs/qa`. Product Go unchanged.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 070.

## Proof

- `healthKind(200, true)` → connected; `(502, false)` and `(200, false)` → degraded; `(401|403)` → unauthorized; `(0)` → offline (`control-plane/src/api/health.test.ts`, `TestKind`).
- Chrome starts without a green pulse (`data-health="checking"` until the first probe). Gateway down → offline or degraded, never connected-green.
- Banned author identifiers absent from `docs/qa`. `./scripts/agpl-check-docs.sh` exit 0.

## Non-goals

SPEC 070 backup/TLS. SPEC 071 tenant-lite. Binding/killing demo ports. Merge. Copying goclaw Go. Secrets in docs.
