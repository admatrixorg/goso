# SPEC 087 — Local OTel Collector + Jaeger (DI-10)

> After 086. Clean-room. Do not kill `:8082` `:8091` `:3000` `:18080`.
> **No Grafana Cloud keys** (DI-18 stays parked).

Closes **O3** leftover: exporter already noops unless `GOSO_OTEL_ENDPOINT` (042). This SPEC is compose + docs + a compose smoke.

## GoClaw cite (docs only)

`/Users/mqglobal/Documents/goclaw/goclaw-source/README.md` optional OpenTelemetry OTLP export.

## goso plan

1. Compose **profile** `otel`:
   - `jaeger` all-in-one **or** `otel-collector` + jaeger. Prefer **Jaeger all-in-one** with OTLP HTTP **4318** and UI **16686** (not 8082/8091/18080/3000).
   - Gateway env when profile on: `GOSO_OTEL_ENDPOINT=http://jaeger:4318/v1/traces` (compose network). Host-run gateway: `http://127.0.0.1:4318/v1/traces`.
2. Existing `observe.HTTPExporter` stays. Optional: set User-Agent `goso-otel/1`, no Authorization header ever.
3. Docs SETUP/RUNBOOK: `docker compose --profile otel up -d jaeger` then export endpoint. Explicit **no Grafana Cloud**.
4. Test: exporter still noops when env empty; httptest collector accepts one span POST (already 042). New test: compose file contains `4318` and **does not** contain grafana-cloud / api-key names.
5. QA `docs/qa/087-otel-local.md`.

QC: go test, build, agpl, agpl-docs.
Commit `admatrixmdp/spec087-otel-local`. Do not merge.
