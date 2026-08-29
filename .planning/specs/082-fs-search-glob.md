# SPEC 082 — Filesystem search + glob (K1)

> After 081. Clean-room. Do not kill `:8082` `:8091`.
> Parked: live channels DI-01..07, exec/browser/media/sandbox DI-12/13/21, PG/pgvector DI-09.

Closes matrix **K1** (034 six-name fs toolkit: `read_file`, `write_file`, `edit_file`, `list_files`, `search`, `glob`). goso already has `read_file` / `write_file` / `list_files` / `edit` / `send_file` (074). This SPEC adds **`search`** and **`glob`** only. Do not rename `edit` → `edit_file`. Do not add exec.

## GoClaw cite (docs only — no Go paste)

`/Users/mqglobal/Documents/goclaw/goclaw-source/README.md` Core tools table — Filesystem: `read_file`, `write_file`, `edit_file`, `list_files`, `search`, `glob` (virtual FS routing).
`/Users/mqglobal/Documents/goclaw/goclaw-source/docs/03-tools-system.md` fs group + workspace path containment / path traversal prevention.

## goso plan (self-written)

1. Builtins `search` and `glob`, same jail as 050/074 (`GOSO_WORKSPACE`, `security.HasDotDot` / `confineUnder`, symlink escape). Empty workspace → `not_configured`, no FS walk.
2. `search`: args `{q, path?}`. Case-insensitive substring over **file contents** under the (optional) relative path, default workspace root. Skip dirs, skip files > 1MiB, skip likely-binary (NUL in first 512 bytes). Cap **50** hits `{path, line, snippet}`. Empty `q` → error `q is required` (no walk).
3. `glob`: args `{pattern}`. Match relative paths from workspace root using Go `filepath.Match` on slash-normalized relative paths (and basename). Recursive walk, cap **256** `{path}` like `list_files`. Empty pattern → error `pattern is required`. Pattern with `..` → `path escape`.
4. Read-only (no approval). Never exec, never write.
5. `Configured("search"|"glob")` same as other fs tools. Catalog + `GET /api/agents/{id}/tools`. CP Functions list picks up catalog names if it already iterates builtins; i18n only if a new label is required.
6. Tests in `gateway/internal/builtin` (temp workspace): happy path; jail `..` and absolute-outside; empty env no touch; cap; binary skip for search.

QC: typecheck if CP touched, `go test ./...`, gofmt, go vet, build, agpl, agpl-docs.
`docs/qa/082-fs-search-glob.md` with cite table.
Commit `admatrixmdp/spec082-fs-search-glob`. Do not merge. Do not start 083.
