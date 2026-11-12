// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-mcp-import loads MCP servers and tool config from an offline
// GoClaw export into an existing goso database. It writes connector rows and
// agent links only. Dry-run is the default. --apply writes. It does not open
// GOSO_DATABASE_URL, and it refuses a non-local Postgres host.
//
// Secrets in mcp_servers.api_key, headers, env, and URL userinfo or query
// strings are not copied. Imported servers are stored disabled. The report
// lists servers that need a credential re-entered, without the secret.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/mcpimport"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("goso-mcp-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	exportPath := fs.String("export", "", "JSON or JSONL export of GoClaw MCP and tool tables")
	tenant := fs.String("tenant", "", "goso tenant_id to write (required)")
	source := fs.String("source-tenant", "", "optional export tenant_id to keep; other tenants are foreign-tenant")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	apply := fs.Bool("apply", false, "write connectors and links; omit to dry-run")
	dry := fs.Bool("dry-run", true, "report per-tenant counts and write nothing (default)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	applySet, drySet := false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "apply":
			applySet = true
		case "dry-run":
			drySet = true
		}
	})
	if applySet && drySet {
		fmt.Fprintln(stderr, "pass either --apply or --dry-run")
		return 2
	}
	dryRun := true
	switch {
	case applySet:
		if !*apply {
			fmt.Fprintln(stderr, "writing requires --apply")
			return 2
		}
		dryRun = false
	case drySet:
		if !*dry {
			fmt.Fprintln(stderr, "writing requires --apply")
			return 2
		}
	}
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*tenant) == "" || strings.TrimSpace(*dbPath) == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if !store.TenantOK(strings.TrimSpace(*tenant)) {
		fmt.Fprintln(stderr, "tenant is invalid")
		return 2
	}
	if s := strings.TrimSpace(*source); s != "" && !store.TenantOK(s) {
		fmt.Fprintln(stderr, "source tenant is invalid")
		return 2
	}
	f, err := os.Open(*exportPath)
	if err != nil {
		fmt.Fprintln(stderr, "open export failed")
		return 1
	}
	defer f.Close()
	ex, err := mcpimport.ReadExport(f)
	if err != nil {
		if errors.Is(err, mcpimport.ErrExportTooLarge) {
			fmt.Fprintln(stderr, "export is too large")
			return 1
		}
		fmt.Fprintln(stderr, "read export failed")
		return 1
	}
	st, closeDB, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	defer closeDB()
	rep, err := mcpimport.Import(st, ex, mcpimport.Options{
		Tenant:       strings.TrimSpace(*tenant),
		SourceTenant: strings.TrimSpace(*source),
		DryRun:       dryRun,
	})
	if err != nil {
		var cfg *mcpimport.ConfigError
		if errors.As(err, &cfg) {
			fmt.Fprintln(stderr, cfg.Error())
		} else {
			fmt.Fprintln(stderr, "import failed")
		}
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		fmt.Fprintln(stderr, "write report failed")
		return 1
	}
	return 0
}

const usage = "usage: goso-mcp-import --export file.json --tenant id --db path [--source-tenant id] [--apply]\ndry-run is the default; --apply writes. Do not pass --apply and --dry-run together."

func openDB(path string) (mcpimport.Store, func() error, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil, errors.New("--db is required")
	}
	if store.IsPostgresDSN(path) {
		if err := requireLoopback(path); err != nil {
			return nil, nil, err
		}
		s, err := store.OpenPostgres(path)
		if err != nil {
			return nil, nil, errors.New("open database failed")
		}
		return s, s.Close, nil
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		info, statErr := os.Lstat(dir)
		if errors.Is(statErr, os.ErrNotExist) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, nil, errors.New("open database failed")
			}
		} else if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, nil, errors.New("open database failed")
		}
	}
	s, err := store.OpenSQLite(path)
	if errors.Is(err, store.ErrPostgresUnsupported) {
		return nil, nil, errors.New("refusing GOSO_DATABASE_URL; this tool only opens --db")
	}
	if err != nil {
		return nil, nil, errors.New("open database failed")
	}
	return s, s.Close, nil
}

func requireLoopback(dsn string) error {
	u, err := url.Parse(dsn)
	if err != nil {
		return errors.New("refusing non-local database host")
	}
	host := strings.TrimSpace(u.Hostname())
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return nil
	default:
		return errors.New("refusing non-local database host")
	}
}
