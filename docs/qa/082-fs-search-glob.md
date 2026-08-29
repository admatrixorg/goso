# QA — SPEC 082 Filesystem search + glob (K1)

Date: 2026-08-29. Clean-room Go/React. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:3000` `:18080` `:18088` — not bound or killed. Do not merge. Do not start SPEC 083.

Closes matrix **K1** leftover (`search`, `glob`). goso already had `read_file` / `write_file` / `list_files` / `edit` / `send_file` (050/074). This SPEC adds the two remaining names only. `edit` is not renamed to `edit_file`.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from docs, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Filesystem six names: `read_file`, `write_file`, `edit_file`, `list_files`, `search`, `glob` (virtual FS routing) | `/Users/mqglobal/Documents/goclaw/goclaw-source/README.md` (Core tools table, Filesystem row) |
| fs group + workspace path containment / path traversal prevention | `/Users/mqglobal/Documents/goclaw/goclaw-source/docs/03-tools-system.md` (Filesystem inventory, Path Security `resolvePath()`, group `fs` membership, read-only vs mutating) |

goso mapping (self-written): keep `edit` as the write-replace name (074). Add read-only `search` and `glob` under the same `GOSO_WORKSPACE` jail (`security.HasDotDot` / `confineUnder`, symlink escape). Empty workspace → `not_configured`, no FS walk. `search` is case-insensitive substring over file contents (optional relative start path, default workspace root). `glob` uses Go `filepath.Match` on slash-normalized relative paths from the workspace root and on the basename. Never exec, never write.

## What changed

- Builtin `search` `{q, path?}`. Empty `q` → `q is required` (no walk). Skip directories, skip files larger than 1MiB, skip likely-binary (NUL in first 512 bytes). Cap **50** hits `{path, line, snippet}`. `truncated: true` when the cap is hit.
- Builtin `glob` `{pattern}`. Empty pattern → `pattern is required` (no walk). Pattern containing `..` → `path escape`. Recursive walk, cap **256** `{path}` (same numeric cap as `list_files`). `truncated: true` when the cap is hit.
- Same jail as 074: reject `..`, absolute paths outside the workspace, and symlink escape. Read-only, no approval.
- Catalog + `Configured("search"|"glob")` follow the other filesystem tools. `GET /api/agents/{id}/tools` includes the names. Control-plane Functions list iterates the catalog (no new i18n key; workspace note lists `search` / `glob`).

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

Do not bind or kill demo ports `:8082` `:8091` `:3000` `:18080` `:18088`. Do not merge. Do not start SPEC 083.

## Proof

- Happy path + jail `..` and absolute-outside (`TestInvoke_SearchHappyAndJail`, `TestInvoke_GlobHappyAndJail`). Basename and slash-relative glob (`*.md` matches `notes/a.md`).
- Empty `q` / empty pattern do not walk (`TestInvoke_SearchEmptyQNoWalk`, `TestInvoke_GlobEmptyPatternNoWalk`). Empty `GOSO_WORKSPACE` → `not_configured` and no FS touch (`TestInvoke_SearchEmptyEnvNoTouch`, `TestInvoke_GlobEmptyEnvNoTouch`).
- Search cap 50, binary NUL skip, files >1MiB skipped (`TestInvoke_SearchCapBinaryAndLargeSkip`). Glob cap 256 (`TestInvoke_GlobCap`). Missing search path is `not_found` (`TestInvoke_SearchMissingPathNotFound`). FIFO/non-regular skipped (`TestInvoke_SearchSkipsFifo`). Invalid glob pattern (`TestInvoke_GlobInvalidPattern`).
- Symlink escape denied (`TestInvoke_SearchSymlinkEscape`, `TestInvoke_GlobSymlinkNoEscape`). Glob may include directory paths (same listing style as `list_files`; search still skips dirs).
- Catalog length 15; `search` / `glob` do not require approval (`TestCatalog_Tools`). `Configured` tracks `GOSO_WORKSPACE` (`TestConfigured_SearchGlobWorkspace`).
- `GET /api/agents/{id}/tools` advertises `search` and `glob` with `configured:false` when env empty (`TestAgentTools_ListAndPatchBuiltin`).

## Non-goals

Rename `edit` → `edit_file`. exec / browser / media (DI-12/13/21). Live channels DI-01..07. PG/pgvector DI-09. Merge. SPEC 083 (`web_fetch`).
