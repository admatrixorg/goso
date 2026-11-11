# SPEC 152 — Vault import file bodies

Status: implemented on this branch. One PR into `main`. Do not merge from this spec.

Sample data is Minh An only. This spec does not read customer data and does not copy GoClaw. It does not add a vault table, a scope column, or a memory change.

## Number

| Number | Where it is used |
|---|---|
| 150 | Open PR #35, branch `cursor/context-files-store-8978`: durable agent and user context files. |
| 151 | Open PR #36: durable tenant registry (`.planning/specs/151-durable-tenant-registry.md`). |
| 152 | This spec. Not a file on `main` at `acd85f1`. |

## Why

The offline importer (`gateway/cmd/goso-vault-import`, `gateway/internal/vaultimport`) copied a document body only from `vault_versions`. A real GoClaw export has no version rows. `--src-dir` only checked that the file existed, so a real import stored a title and an empty body.

## Behavior

1. When a document has no `vault_versions` row, the body is the file under `--src-dir`. The path is resolved the same way as the existing path-missing check: the export path is joined onto the source directory, `..` is rejected, and a symlink is not followed. The provenance trailer stays `<!-- goso-import src=<id> sum=<sha256> -->`. For that file, `sum` is sha256 of the raw file bytes. Newlines in the stored text are normalized; the sum is not. When a version row exists, the body and the sum stay as they are now: the latest version text, and sha256 of `title + "\n" + body prefix`.
2. A kept document that needs a file body and was given no `--src-dir` fails the run with `source dir is required when a document has no version` before any vault write. An empty file is `empty-body` and writes no row.
3. Existing guards are unchanged: symlink rejection, path / `..` / root / tenant filters, foreign-tenant, path-taken, owner-unclear, scope-personal-held, `--include-personal` default off, `--keep-prefix`, `--dry-run`, and no overwrite of a goso document this importer does not still own. Memory, the vault schema, and every other store are unchanged.
4. A file body over 1 MiB is `too-large` and writes no row. The cap is `security.MaxAPIBody`, the same 1 MiB as `storage.MaxFileBytes`. A file that is not valid UTF-8, or that contains a NUL byte, is `binary-skipped` and writes no row. Neither reason is applied to a document that already has a version row; that file is still only an existence check.
5. The report has `docs` and `links` sections. Each section is broken down per source tenant. For every tenant, `input = inserted + updated + skipped + rejected`, and the reason counts sum to `skipped + rejected`. Top-level inserted, updated, and skipped stay document counts. Top-level rejected counts document and link rejections. The importer returns an error if a tally does not balance. A personal document that shares a path with a team document, while `--include-personal` is off, is one `scope-personal-held` row. It is not also `scope-conflict`.
6. A later run still refreshes a version-sourced row when the marker names the same document and the stored title and prefix still match the version checksum. A file-sourced row is refreshed only when the marker sum equals sha256 of the current file, the title is unchanged, and the stored text still begins with that file. A hand edit of the file text, or a different file, is `path-taken` and is not overwritten.

## Reasons

| Code | When |
|---|---|
| `path-missing` | `--src-dir` is set and the resolved file is not a regular file |
| `too-large` | file body is over 1 MiB |
| `binary-skipped` | file body is not UTF-8 or contains NUL |
| `empty-body` | file body is empty |
| `scope-personal-held` | scope `personal` and `--include-personal` is off, counted once |
| `unchanged` | an importer-owned row or link already matches |
| `tenant-mismatch` | document tenant is not the source tenant and the path is otherwise safe |

Path, symlink, foreign-tenant, path-taken, owner-unclear, and scope-conflict stay as they were.

## Tests

`TestFileBodyMinhAn` on memory, SQLite, and Postgres (when `GOSO_TEST_POSTGRES_DSN` is set): a version row ignores different file bytes; a document with no version stores the file text and sha256 of the raw file; missing file; file over 1 MiB; invalid UTF-8; NUL; empty file; missing `--src-dir` writes nothing; per-tenant doc and link totals balance; a second run does not write; a hand edit of the file text is `path-taken`; a seeded memory row is unchanged. `TestPersonalScopeHeldUnlessEnabled` expects one `scope-personal-held` and no second `scope-conflict`.

## Non-goals

No control-plane change. No `vault_versions` table. No copy of GoClaw. No customer export in the tree.
