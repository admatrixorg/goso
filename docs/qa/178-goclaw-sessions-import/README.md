# SPEC 178 — session and message import

No control-plane tab changed. Sample data is the Minh An JSONL fixture only.

```bash
go test -race -count=1 -timeout 180s ./gateway/internal/sessionimport ./gateway/cmd/goso-sessions-import
go build -o /dev/null ./gateway/cmd/goso-sessions-import
```

`-race` passed (`sessionimport` 3.228s, `goso-sessions-import` 1.568s). `go build` of the command exited 0. Postgres was not running here, so these runs used SQLite. The writer uses the same `?` / `INSERT OR IGNORE` rewrite as the store for a loopback Postgres `--db`.

Dry-run of the fixture writes no session, message, memory, or link row. `--apply` inserts 4 sessions and 6 messages into `minh-an`, skips the duplicate lines, and leaves the `ban-khac` line unwritten. A second `--apply` inserts nothing. `memories` stays empty, and the imported transcript is not in `memory_fts`.

Message order follows the `messages` array. A repeated timestamp is stored one nanosecond later so `ORDER BY created_at` keeps that order. Other timestamps stay on the source instant.

Columns with no home are listed in the report `not_imported` and in `.planning/specs/178-goclaw-sessions-import.md`.

`qa:tabs` for code `ec929e8d7008783158f9474d3ad6a15dc3c1404d` is `docs/qa/178/qa-tabs-ec929e8d7008783158f9474d3ad6a15dc3c1404d.log`. Exit 0.
