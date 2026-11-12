// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-mcp-import loads MCP servers and tool config from an offline
// GoClaw export into an existing goso database. It writes connector rows and
// agent links only. Dry-run is the default. --apply writes. It does not open
// GOSO_DATABASE_URL, and it refuses a non-local Postgres host.
//
// A local deny file is required. If that file is missing, empty, or
// malformed, the command stops before it opens the database. The report
// counts denied tenants and does not name them.
//
// Secrets in mcp_servers.api_key, headers, env, URL userinfo, query strings,
// key-like path segments, and secret stdio arguments are not copied.
// Imported servers are stored disabled.
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
	source := fs.String("source-tenant", "", "export tenant_id to keep (required for import)")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	denyFile := fs.String("deny-file", "", "local file of tenant ids to refuse; else GOSO_IMPORT_DENY_FILE")
	rollback := fs.Bool("rollback", false, "delete import-map rows for --tenant; dry-run unless --apply")
	apply := fs.Bool("apply", false, "write; omit to dry-run")
	dry := fs.Bool("dry-run", true, "report counts and write nothing (default)")
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
	dest := strings.TrimSpace(*tenant)
	src := strings.TrimSpace(*source)
	if dest == "" || strings.TrimSpace(*dbPath) == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if !*rollback && (strings.TrimSpace(*exportPath) == "" || src == "") {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if *rollback && strings.TrimSpace(*exportPath) != "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if !store.TenantOK(dest) {
		fmt.Fprintln(stderr, "tenant is invalid")
		return 2
	}
	if !*rollback && !store.TenantOK(src) {
		fmt.Fprintln(stderr, "source tenant is invalid")
		return 2
	}
	denyPath := strings.TrimSpace(*denyFile)
	if denyPath == "" {
		denyPath = strings.TrimSpace(os.Getenv("GOSO_IMPORT_DENY_FILE"))
	}
	if denyPath == "" {
		fmt.Fprintln(stderr, "deny file")
		return 2
	}
	deny, err := mcpimport.LoadDenyFile(denyPath)
	if err != nil {
		fmt.Fprintln(stderr, "deny file")
		return 2
	}
	if *rollback {
		if n := deniedCount(deny, dest); n > 0 {
			writeReport(stdout, stderr, mcpimport.RollbackReport{Rollback: true, DryRun: true, DeniedTenants: n})
			fmt.Fprintln(stderr, "tenant is denied")
			return 1
		}
		st, closeDB, err := openDB(*dbPath)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		defer closeDB()
		rep, err := mcpimport.Rollback(st, mcpimport.Options{Tenant: dest, DryRun: dryRun, Deny: deny})
		if err != nil {
			return fail(stderr, err)
		}
		return writeReport(stdout, stderr, rep)
	}
	if n := deniedCount(deny, dest, src); n > 0 {
		writeReport(stdout, stderr, mcpimport.Report{
			DryRun:          true,
			Tenants:         []mcpimport.TenantReport{},
			NeedsCredential: []mcpimport.CredentialNeed{},
			NotImported:     []mcpimport.Decision{},
			DeniedTenants:   n,
		})
		fmt.Fprintln(stderr, "tenant is denied")
		return 1
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
		Tenant:       dest,
		SourceTenant: src,
		DryRun:       dryRun,
		Deny:         deny,
	})
	if err != nil {
		if rep.DeniedTenants > 0 {
			writeReport(stdout, stderr, rep)
			fmt.Fprintln(stderr, "tenant is denied")
			return 1
		}
		return fail(stderr, err)
	}
	return writeReport(stdout, stderr, rep)
}

const usage = "usage: goso-mcp-import --export file.json --tenant id --source-tenant id --deny-file path --db path [--apply]\n       goso-mcp-import --rollback --tenant id --deny-file path --db path [--apply]\ndry-run is the default; --apply writes. Do not pass --apply and --dry-run together.\nGOSO_IMPORT_DENY_FILE is used when --deny-file is omitted. A missing, empty, or malformed deny file stops the command."

func deniedCount(list map[string]struct{}, ids ...string) int {
	seen := map[string]struct{}{}
	for _, id := range ids {
		if mcpimport.Denied(list, id) {
			seen[strings.ToLower(strings.TrimSpace(id))] = struct{}{}
		}
	}
	return len(seen)
}

func fail(stderr io.Writer, err error) int {
	var cfg *mcpimport.ConfigError
	if errors.As(err, &cfg) {
		fmt.Fprintln(stderr, cfg.Error())
	} else {
		fmt.Fprintln(stderr, "import failed")
	}
	return 1
}

func writeReport(stdout, stderr io.Writer, v any) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(stderr, "write report failed")
		return 1
	}
	return 0
}

func openDB(path string) (interface {
	mcpimport.Deleter
}, func() error, error) {
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
