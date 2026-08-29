# QA — SPEC 087 Local OTel Collector + Jaeger (DI-10)

Date: 2026-08-29. Clean-room Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not merge.

Closes matrix **O3** leftover: exporter already noops unless `GOSO_OTEL_ENDPOINT` (042). This SPEC is compose profile `otel` + SETUP/RUNBOOK + a compose smoke. **DI-18** Grafana Cloud keys stay parked.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Optional OpenTelemetry OTLP export | `/Users/mqglobal/Documents/goclaw/goclaw-source/README.md` (Observability — optional OpenTelemetry OTLP export) |
| Optional Jaeger tracing UI | `/Users/mqglobal/Documents/goclaw/goclaw-source/README.md` (`WITH_OTEL=1` Jaeger — OpenTelemetry tracing UI) |

goso mapping (self-written): compose **profile** `otel` (not default `up`, not `WITH_OTEL=1`) runs Jaeger all-in-one with OTLP HTTP **4318** and UI **16686**. Gateway env on the compose network: `GOSO_OTEL_ENDPOINT=http://jaeger:4318/v1/traces`. Host-run gateway: `http://127.0.0.1:4318/v1/traces`. Empty env stays `NoopExporter` (042). `HTTPExporter` sets `User-Agent: goso-otel/1` and never `Authorization`. No Grafana Cloud keys.

## What changed

- `compose.yml` service `jaeger` behind profile `otel`, image `jaegertracing/all-in-one:1.66.0`, host **4318** (OTLP HTTP) and **16686** (UI). Not 8082/8091/18080/3000.
- Gateway compose env pass-through `GOSO_OTEL_ENDPOINT` (empty default = noop). Documented compose-network URL `http://jaeger:4318/v1/traces`.
- `observe.HTTPExporter` User-Agent `goso-otel/1`. Never an Authorization header. Does not read Grafana Cloud keys.
- Tests: empty env still noop; httptest collector still accepts one span POST; compose.yml contains `4318` and does not contain grafana-cloud / api-key vendor names.
- SETUP / RUNBOOK: `docker compose --profile otel up -d jaeger` then export the endpoint. Explicit no Grafana Cloud.

## Commands

```
go test ./...
gofmt -l gateway desktop
go vet ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Optional (not default CI; does not bind demo ports):

```
docker compose --profile otel up -d jaeger
export GOSO_OTEL_ENDPOINT=http://127.0.0.1:4318/v1/traces
# UI: http://127.0.0.1:16686
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge.

## Proof

- `go test ./... -count=1` OK; `gofmt -l gateway desktop` empty; `go vet ./...` OK; `go build -o bin/goso-gateway ./gateway/cmd/goso-gateway` OK
- `agpl-check.sh` OK; `./scripts/agpl-check-docs.sh` OK
- Empty `GOSO_OTEL_ENDPOINT` is noop (`TestExporterFromEnv_EmptyIsNoop`, `TestNew_UsesNoopWhenEndpointEmpty`).
- HTTP JSON POST has User-Agent `goso-otel/1` and no Authorization / Grafana header (`TestHTTPExporter_PostsJSONNoGrafanaHeader`).
- `compose.yml` contains `4318` and the compose-network URL; rejects grafana-cloud / api-key names (`TestComposeYml_OTelPortNoGrafanaCloud`).
- Default `docker compose up` does not start jaeger. This SPEC did not bind or kill demo ports.

## Non-goals

Grafana Cloud / SaaS keys (DI-18). Protobuf OTLP SDK / `go.opentelemetry.io`. Extra collector sidecar. Binding demo ports. Copying goclaw Go. Merge.
