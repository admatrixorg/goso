# QA — SPEC 050 Filesystem tools in GOSO_WORKSPACE jail

Date: 2026-08-27. Clean-room. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed.

K1 was missing gateway `read_file` / `write_file`. The `GOSO_WORKSPACE` write jail already existed (041). This SPEC adds the two builtins and fail-closes them when the env is empty.

## What changed

- Builtins `read_file` `{path}` and `write_file` `{path, content}` (`connector: "builtin"`). Always advertised on `GET /api/agents/{id}/tools`.
- **Fail-closed** when `GOSO_WORKSPACE` is empty/unset: both tools return `not_configured` and do not touch the filesystem.
- When set: resolve the argument inside the workspace only. Reject `..`, absolute paths outside the jail, and symlink escape (file or directory). Read cap **1MiB** (reject, not truncate). Write creates parent directories only after the jail check. **No exec**, no delete.
- `write_file` `requires_approval: true` (Functions approval badge). `read_file` does not require approval.
- Control-plane Functions: workspace jail note (vi+en). Existing approval column shows the write badge from the catalog.

## Commands

```
cd control-plane && npm run typecheck
go test ./...
go build -o bin/goso-gateway ./gateway/cmd/goso-gateway
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`.

## Proof

- `gateway/internal/builtin`: catalog length 7 includes `read_file` / `write_file`; `write_file` requires approval; empty env `not_configured` and no FS touch; tempdir write/read round-trip; `..` and absolute outside rejected; symlink (file + dir) escape rejected; 1MiB read cap.
- httptest `GET /api/agents/{id}/tools` advertises both builtins; `POST /api/tools/invoke` `write_file` without workspace → `not_configured`.

## Non-goals

`delete_file`, process spawn, sandbox/browser/media exec, live demo bind/kill, copying goclaw/ZaloCRM.
