# SPEC 083 — web_fetch + SSRF (K3)

> After 082. Clean-room. Do not kill `:8082` `:8091`.
> Parked: paid search DI-08, live channels DI-01..07, media DI-21.

Closes matrix **K3** leftover: `web_search` exists (074); **`web_fetch` missing**.

## GoClaw cite (docs only — no Go paste)

`/Users/mqglobal/Documents/goclaw/goclaw-source/docs/03-tools-system.md` Web group: `web_fetch` fetch a URL; domain allow/block.
`/Users/mqglobal/Documents/goclaw/goclaw-source/docs/09-security.md` SSRF (hostname / private IP / DNS pin) — goso already has this in 066 (`security.CheckURL` / `GuardClient`).

## goso plan (self-written)

1. Builtin `web_fetch` args `{url}`. Empty url → `url is required`.
2. **Always** call `security.CheckURL(url)` first. If it returns error → public `status=error` `error` contains `ssrf` (fail-closed, no dial). When `GOSO_SSRF` off / demo, CheckURL allows loopback so httptest still works (066 contract).
3. HTTP GET via `http.Client` with `security.GuardClient` (redirect re-check). Timeout 10s. Cap body **1MiB** (read then stop; do not stream unbounded).
4. Response `{status, content_type, body}` — body is the truncated bytes as string (no HTML-to-Markdown library; no extra vendor). Non-2xx still returns body + status (not a panic).
5. Network gate: same as web_search **or** always-on with SSRF as the only gate. Pick **SSRF as the only network policy** (no extra env). Tests must set `GOSO_SSRF=1` for block cases and use httptest for allow cases with SSRF off (demo).
6. Catalog + Configured: `true` always for advertisement; invoke still SSRF-fail-closed. CP list shows `web_fetch`.
7. Tests: httptest 200 maps body; `GOSO_SSRF=1` + `http://127.0.0.1/...` never dials (httptest listener must not see a request); redirect to loopback blocked when SSRF on; empty url.

QC: typecheck if CP touched, `go test ./...`, gofmt, go vet, build, agpl, agpl-docs.
`docs/qa/083-web-fetch.md` with cite table.
Commit `admatrixmdp/spec083-web-fetch`. Do not merge. Do not start another SPEC.
