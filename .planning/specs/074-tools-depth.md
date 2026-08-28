# SPEC 074 — Tools depth: filesystem, web, media fail-closed

> LOCKED after SPEC 073 merge. Clean-room. Do not kill `:8082` `:8091`.
> Do **not** spawn a real browser or sandbox (DI-12/13 stay parked). `exec`/`browser` remain fail-closed stubs.

Closes matrix **K1, K3, K5**. K2 exec/browser stay PARTIAL on purpose.

## GoClaw (cite, no copy)

| Behavior | Cite |
|----------|------|
| Filesystem group: `read_file`, `write_file`, `list_files`, `edit`, `send_file` (read-only vs write) | `docs/03-tools-system.md` (fs row + `read_file` description) |
| `web_search` is a real tool with fail-closed when unconfigured | same tools table; goso already has DDG Instant fail-closed |
| Media tools fail closed when not configured | README tools / `docs/03-tools-system.md` |

## goso today

- 050: `read_file` / `write_file` jailed to `GOSO_WORKSPACE`.
- 044: `web_search` DDG Instant fail-closed; `media` stub `not_configured`; `exec`/`browser` stubs.

## goso plan (self-written)

1. Filesystem tools (jail `GOSO_WORKSPACE`, path `..` denied): add `list_files`, `edit` (search/replace one occurrence in a file), `send_file` (return `{path, bytes, mime}` metadata only — **do not** upload off-box). Keep `read_file`/`write_file`. Tests use a temp workspace.
2. `web_search`: keep fail-closed when no network / empty query; if DDG (or existing provider) returns JSON, map to `{title, url, snippet}[]`. Tests use httptest, **no live web**. Empty key/config → `not_configured` (not a panic).
3. Media: `image_gen` / `tts` (or the existing stub names) remain **fail-closed** `not_configured` unless `GOSO_MEDIA_*=1` **and** a test double is injected. Never call a paid API. Tests assert  the public error string, no crash.
4. `exec` / `browser`: still `not_configured` (DI-12/13). Document in QA.
5. CP Functions/tools list shows the new names + configured flag. i18n.

## Tests

- list_files / edit / send_file jail + happy path.
- web_search httptest fixture; empty → not_configured or empty list (pick one, document).
- media stub fail-closed.
- Existing read/write tests still pass.

QC: typecheck, go test, build, agpl, agpl-docs.
`docs/qa/074-tools-depth.md` with cite table.
Commit `admatrixmdp/spec074-tools-depth`. Do not merge. Do not start 075.
