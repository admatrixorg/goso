# QA — SPEC 074 Tools depth: filesystem, web, media fail-closed

Date: 2026-08-29. Clean-room Go/React. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not merge. Do not start SPEC 075.

Closes matrix **K1, K3, K5**. **K2** exec/browser stay PARTIAL on purpose (DI-12/13).

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Filesystem group: `read_file`, `write_file`, `list_files`, `edit`, `send_file` (read-only vs write) | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/03-tools-system.md` (fs row + `read_file` description; group `fs` membership) |
| `web_search` is a real tool with fail-closed when unconfigured | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/03-tools-system.md` (web row; tenant config / provider chain) |
| Media tools fail closed when not configured | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/03-tools-system.md` (Media Generation / Media Reading) |

goso mapping (self-written): jail all five filesystem builtins under `GOSO_WORKSPACE` (keep the 050 `read_file`/`write_file` pair). `edit` replaces **one** occurrence. `send_file` returns `{path, bytes, mime}` only — it does not upload. `web_search` maps DuckDuckGo Instant Answer JSON to `{title, url, snippet}[]`. Empty env, empty query, or empty provider base → public error `not_configured` (no panic, no live network in tests). `media` / `image_gen` / `tts` stay `not_configured` unless `GOSO_MEDIA*=1` **and** a process-injected test double. `sandbox` (exec/DI-12) and `browser` (DI-13) stay `not_configured` always.

## What changed

- Builtins added: `list_files` `{path?}`, `edit` `{path, old, new}` (one replace; Functions approval **badge** like 050 `write_file`, same builtin invoke path), `send_file` `{path}` metadata only. Same jail as 050: reject `..`, absolute outside, symlink escape. Empty `GOSO_WORKSPACE` → `not_configured`, no FS touch. Empty directories return `ok` + `entries: []`.
- `web_search` result contract is `{results:[{title,url,snippet}]}`. httptest fixtures only. Empty `GOSO_WEB_SEARCH`, empty query, or empty `InstantAnswerBase` → `not_configured` (documented choice: not an empty list). Empty Instant Answer JSON with env on → `ok` + empty `results`.
- Media stubs `media`, `image_gen`, `tts`: public error string `not_configured` unless env **and** `MediaInvoke` double. Never ffmpeg, never a paid vendor.
- `sandbox` / `browser` remain stubs (DI-12/13). No Docker, no Chrome.
- `GET /api/agents/{id}/tools` includes the new names and a `configured` boolean. Functions page column + workspace note (vi+en).

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 075.

## Proof

- list_files / edit / send_file jail + happy path (`TestInvoke_ListFilesHappyAndJail`, `TestInvoke_EditOneReplaceAndJail`, `TestInvoke_SendFileMetadataOnlyAndJail`). Empty dir (`TestInvoke_ListFilesEmptyDir`). Symlink escape (`TestInvoke_ListEditSendSymlinkEscape`).
- Empty workspace: new FS tools `not_configured` and no FS touch (`TestInvoke_ListFilesEmptyEnvNoTouch`, `TestInvoke_EditEmptyEnvNoTouch`, `TestInvoke_SendFileEmptyEnvNoTouch`). Existing read/write tests still pass.
- web_search httptest fixture maps to `{title,url,snippet}` (`TestInvoke_WebSearchDDGHttptest`). Empty query / empty base / unconfigured env do not network (`TestInvoke_WebSearchEmptyQueryNoNetwork`, `TestInvoke_WebSearchEmptyBaseNoNetwork`, `TestInvoke_UnconfiguredNoNetwork`). Empty JSON → empty list (`TestInvoke_WebSearchEmptyJSON`).
- media stub fail-closed; env-only or double-only still `not_configured`; both required (`TestInvoke_MediaFailClosedUnlessDouble`).
- sandbox/browser still `not_configured` (`TestInvoke_SandboxNeverSpawns`).
- CP list advertises `list_files` `edit` `send_file` `image_gen` `tts` + `configured:false` when env empty (`TestAgentTools_ListAndPatchBuiltin`).

## Non-goals

Live DDG. Paid search (DI-08). Paid media vendors (DI-21). Docker sandbox spawn (DI-12). Headless Chrome (DI-13). `web_fetch`. Off-box `send_file` upload. Merge. SPEC 075.
