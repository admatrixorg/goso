# Test Report — 2026-08-29 — SPEC 087 local OTel + Jaeger

Worktree: `/Users/mqglobal/orca/workspaces/goso/spec087-otel-local`
Branch: `admatrixmdp/spec087-otel-local`
Did **not** bind/kill `:8082` `:8091` `:3000` `:18080` `:18088`. Did **not** `docker compose up`.

## Diff-aware mapping

Diff-aware mode: analyzed 12 changed files (uncommitted)

Changed:
- `compose.yml`, `compose.prod.yml`
- `gateway/internal/observe/export.go`, `export_test.go`
- `gateway/internal/observe/compose_test.go` (new, untracked)
- `gateway/cmd/goso-gateway/main.go`
- `.env.example`, `CHANGELOG.md`, `docs/{ARCHITECTURE,DEPLOY,RUNBOOK,SETUP}.md`
- `docs/qa/087-otel-local.md` (untracked)

Mapped:
- `export.go` → `export_test.go` (Strategy A)
- `compose.yml` → `compose_test.go` (Strategy C / contract test)

Unmapped:
- [!] No tests for `gateway/cmd/goso-gateway/main.go` help text (`GOSO_OTEL_ENDPOINT` / Jaeger URLs)
- [!] No tests for `compose.prod.yml` (docs-only overlay; jaeger stays in `compose.yml` profile `otel`)
- [!] Docs / `.env.example` / CHANGELOG — not unit-tested (expected)

Ran 23/552 tests (observe package, verbose) then full `go test ./...`. Full suite all packages PASS.

## Test Results Overview

| Command | Result |
|---------|--------|
| `go test ./gateway/internal/observe -count=1` | **PASS** (23 tests, 0.444s) |
| `go test ./... -count=1` | **PASS** (27 packages ok, 2 no tests, ~12.5s) |
| `gofmt -l gateway desktop` | **PASS** (empty = all formatted) |
| `go vet ./...` | **PASS** |

- **Total**: 552 `func Test*` in repo; observe 23; all executed packages green
- **Passed**: all | **Failed**: 0 | **Skipped**: 0
- **Duration**: observe 0.444s; full suite ~12.5s

### Named SPEC 087 tests

| Test | Result |
|------|--------|
| `TestExporterFromEnv_EmptyIsNoop` | **PASS** |
| `TestNew_UsesNoopWhenEndpointEmpty` | **PASS** |
| `TestHTTPExporter_PostsJSONNoGrafanaHeader` | **PASS** — asserts `User-Agent == goso-otel/1`, `len(Authorization)==0`, no `grafana*` header, no `graf-secret` in body |
| `TestComposeYml_OTelPortNoGrafanaCloud` | **PASS** |
| `TestFakeExporter_Records` | **PASS** (042 leftover, still green) |

## Coverage Metrics

observe package (`-covermode=atomic`):

| Metric | Value | Threshold | Status |
|--------|-------|-----------|--------|
| Lines (observe pkg) | 72.3% | 80% | FAIL (pkg-wide; pre-existing hijack/span helpers) |
| `ExporterFromEnv` | 100% | 80% | PASS |
| `HTTPExporter.Export` | 69.6% | 80% | FAIL |
| `otlpPayload` | 58.8% | 80% | FAIL |
| `otlpKV` / `NoopExporter.Export` | 100% | 80% | PASS |

SPEC 087 happy path covered. Error/empty branches of `HTTPExporter.Export` and parent/error/status attrs in `otlpPayload` not hit.

## Confirmations

1. Empty `GOSO_OTEL_ENDPOINT` → `NoopExporter` (`TestExporterFromEnv_EmptyIsNoop`, `TestNew_UsesNoopWhenEndpointEmpty`) **PASS**.
2. HTTP JSON POST `User-Agent: goso-otel/1`, no `Authorization` (`TestHTTPExporter_PostsJSONNoGrafanaHeader`) **PASS**. Source `export.go:102` sets UA; never sets Authorization. Test also sets `GRAFANA_CLOUD_API_KEY` and rejects grafana headers + body leak.
3. `compose.yml` contains `4318` and `http://jaeger:4318/v1/traces`. Does **not** contain `grafana-cloud` / `grafana_cloud` / `api-key` / `apikey`. Jaeger service is `profiles: [otel]`, ports `4318` + `16686` only (host mapping). Control-plane still `3000:3000` (existing stack; not jaeger).
4. **No test binds reserved demo ports.** Tests use `httptest.NewServer` (ephemeral) / `httptest.NewRecorder`. Repo `*_test.go` has no `ListenAndServe` and no `:8082` `:8091` `:3000` `:18080` `:18088`. Only non-test bind is `gateway/cmd/goso-gateway/main.go` `net.Listen` (not executed by unit tests).

## Failed Tests

None.

## Performance Metrics

- observe: 0.444s
- full suite: ~12.5s
- slowest package: `gateway/internal/builtin` 10.352s (sandbox/browser, SPEC 086 — not 087)
- no flaky retries observed (`-count=1`)

## Build Status

- `go test` compile+run: PASS
- `gofmt`: PASS
- `go vet`: PASS
- `go build` not requested (skipped)
- docker compose smoke **not** run (reserved; control-plane `:3000`)

## Critical Issues

None blocking. SPEC 087 unit/contract gates green.

## Recommendations

1. **Medium** — `TestComposeYml_OTelPortNoGrafanaCloud` does not assert `profiles:` / `otel`. QA doc claim “default `docker compose up` does not start jaeger” is true in YAML (`compose.yml:98-99`) but unenforced. Add `strings.Contains(body, "otel")` near the jaeger service block (or yaml parse) so a future edit cannot drop the profile.
2. **Medium** — `HTTPExporter.Export` uncovered: empty endpoint/spans noop, nil ctx, default client, `client.Do` error, HTTP status >= 300 (`export.go:83-111`). Add httptest 500 + closed-server cases.
3. **Low** — `otlpPayload` uncovered: `Attributes`, `Error`, `ParentID`, `Status` (`export.go:122-145`). One span with parent+error would cover.
4. **Low** — `dispatchExport` HTTP async path (`observe.go:101-103`) not exercised; test calls `Export` directly. Optional: `RecordSpans` with `HTTPExporter` + wait.
5. **Low** — no test that compose jaeger ports are not 8082/8091/18080/3000 (comment-only today).

Did **not** edit product or test files (prefer report).

## Next Steps

1. Implementer may add compose profile assertion (rec 1) — optional, not blocking.
2. Reviewer: green for merge-readiness of unit gates; docker compose `--profile otel` smoke still human/optional (not CI, not this run).
3. Do not merge from this tester session (SPEC says do not merge).

## Unresolved Questions

- Team TaskList MCP not in connected servers (tasks MCP is automations). Could not claim/complete via TaskUpdate.
- Live Jaeger smoke (`docker compose --profile otel up -d jaeger`) not executed — out of tester port/compose constraints.
