# SPEC 176 — GoClaw MCP and tool-config import

Offline importer. No control-plane tab changed, so prototype screenshots do not apply. Sample rows are Minh An only. The fixture `api_key`, header, env, query, and credential-table values are the placeholder prefix `placeholder-minh-an`. They are not real credentials. No MQ Capital rows. No pgvector.

The command is `goso-mcp-import`. Dry-run is the default. It prints per-tenant counts and `needs_credential` server names. It does not print secret values.

```bash
go test -count=1 -timeout 180s ./gateway/internal/mcpimport ./gateway/cmd/goso-mcp-import
go test -race -count=1 -timeout 180s ./gateway/internal/mcpimport ./gateway/cmd/goso-mcp-import
go build -o /tmp/goso-mcp-import ./gateway/cmd/goso-mcp-import
```

`GOSO_TEST_POSTGRES_DSN` was unset here, so `TestCLIPostgresApply` skipped. SQLite and the in-memory store ran. Logs: `go-test.txt`, `go-test-race.txt`.

## qa:tabs

Code under test: `755514888f0492031ddf4132982d9d29659346ba` (merge of `origin/main` SPEC 182).

[qa-tabs log](../176/qa-tabs-755514888f0492031ddf4132982d9d29659346ba.log)

Gateway from that commit, fresh SQLite database, seed `minh-an-sales` / `Cố vấn Minh An` / `echo`. `CHROME_PATH=/usr/local/bin/google-chrome`. Exit 0. Last lines: `flow chat` then `qa:tabs ok`. Earlier logs remain in the same directory.
