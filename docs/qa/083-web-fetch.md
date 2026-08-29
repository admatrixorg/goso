# QA — SPEC 083 web_fetch + SSRF (K3)

Date: 2026-08-29. Clean-room Go/React. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not merge. Do not start another SPEC.

Closes matrix **K3** leftover: `web_search` exists (074); **`web_fetch` was missing**. goso already had SSRF in 066 (`security.CheckURL` / `GuardClient`).

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Web group: `web_fetch` fetch a URL (HTML → Markdown in upstream; goso returns truncated bytes as string) | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/03-tools-system.md` (Web group row `web_fetch`; tenant config `policy` / `allowed_domains` / `blocked_domains`) |
| SSRF 3-step: blocked hostnames, private IP ranges, DNS pinning also applied to redirect targets | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/09-security.md` (Layer 3 Tool Security, SSRF protection) |

goso mapping (self-written): builtin `web_fetch` `{url}` HTTP GET. Always call `security.CheckURL(url)` first; on error public `status=error` and `error` contains `ssrf` (no dial). `http.Client` + `security.GuardClient` re-checks redirects. Timeout 10s. Cap body **1MiB** (read then stop). Response `{status, content_type, body}` as truncated string — no HTML-to-Markdown library, no extra vendor. Non-2xx still returns (no panic). **SSRF is the only network policy** (no extra env). `Configured("web_fetch")` is always true for advertisement; invoke stays SSRF-fail-closed. When `GOSO_SSRF` is off / demo, CheckURL allows loopback so httptest still works (066 contract).

## What changed

- Builtin `web_fetch` `{url}`. Empty url → `url is required`.
- Always `security.CheckURL` then GET with `GuardClient`. Catalog advertised; `GET /api/agents/{id}/tools` includes `web_fetch` with `configured: true`.
- Control-plane Functions list iterates the catalog; workspace note (vi+en) mentions `web_fetch` / `GOSO_SSRF`.
- No paid search. No HTML-to-Markdown library. No DI-01..07, exec, browser, or media.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
gofmt -l gateway desktop
go vet ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start another SPEC.

## Proof

- httptest 200 maps body with `GOSO_SSRF` off/demo (`TestInvoke_WebFetchHttptest200`). Non-2xx still returns (`TestInvoke_WebFetchNon2xxStillReturns`). Body cap 1MiB (`TestInvoke_WebFetchBodyCap`).
- `GOSO_SSRF=1` + `http://127.0.0.1` never dials (`TestInvoke_WebFetchSSRFNoDial`). Redirect to loopback blocked when SSRF on (`TestInvoke_WebFetchRedirectLoopbackBlocked`). Empty url (`TestInvoke_WebFetchEmptyURL`).
- Catalog length 16; `web_fetch` does not require approval (`TestCatalog_Tools`). `Configured` is always true (`TestConfigured_WebFetchAlways`).
- `GET /api/agents/{id}/tools` advertises `web_fetch` with `configured:true` (`TestAgentTools_ListAndPatchBuiltin`).
- Invoke result JSON-marshals a `text/plain` body (`TestInvoke_WebFetchHttptest200`). `Raw` is omitted so HTML/text is not treated as `json.RawMessage`.
- Functions `enabled` flag is persisted like other builtins; invoke is not env-gated. SSRF (`GOSO_SSRF` / production default) is the only network policy. The toggle does not add a second env gate.

## Non-goals

Paid search (DI-08). HTML-to-Markdown library. Live channels DI-01..07. exec / browser / media (DI-12/13/21). Extra env besides `GOSO_SSRF`. Merge. Another SPEC.
