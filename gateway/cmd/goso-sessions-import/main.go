// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-sessions-import loads one tenant from an offline JSONL export
// of GoClaw sessions. Messages are the sessions.messages array. Dry-run is
// the default and prints per-tenant counts. --apply writes sessions and
// messages only. It does not open GOSO_DATABASE_URL, and it refuses a
// non-local Postgres host.
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
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/sessionimport"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("goso-sessions-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	exportPath := fs.String("export", "", "JSONL export of GoClaw sessions, one session per line")
	tenant := fs.String("tenant", "", "goso tenant_id to write (required)")
	source := fs.String("source-tenant", "", "export tenant_id to keep (required); other tenants are not written")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	denyPath := fs.String("deny-tenants", "", "local denylist file, one tenant id per line; or set GOSO_IMPORT_DENY_TENANTS")
	rollback := fs.Bool("rollback", false, "remove rows this importer recorded for --tenant; dry-run unless --apply")
	apply := fs.Bool("apply", false, "write; omit to dry-run")
	dry := fs.Bool("dry-run", true, "print counts and write nothing (default)")
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
	if strings.TrimSpace(*tenant) == "" || strings.TrimSpace(*dbPath) == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if !*rollback && (strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*source) == "") {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if !store.TenantOK(strings.TrimSpace(*tenant)) {
		fmt.Fprintln(stderr, "tenant is invalid")
		return 2
	}
	if !*rollback && badToken(*source) {
		fmt.Fprintln(stderr, "source tenant is invalid")
		return 2
	}
	denyFile := strings.TrimSpace(*denyPath)
	if denyFile == "" {
		denyFile = strings.TrimSpace(os.Getenv("GOSO_IMPORT_DENY_TENANTS"))
	}
	gateSource := strings.TrimSpace(*source)
	if *rollback {
		gateSource = strings.TrimSpace(*tenant)
	}
	if rep, err := sessionimport.DenyGate(denyFile, *tenant, gateSource); err != nil {
		writeReport(stdout, rep)
		var cfg *sessionimport.ConfigError
		if errors.As(err, &cfg) {
			fmt.Fprintln(stderr, cfg.Error())
		} else {
			fmt.Fprintln(stderr, "import failed")
		}
		return 1
	}
	if *rollback {
		st, closeDB, err := openDB(*dbPath)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		defer closeDB()
		rep, err := sessionimport.Rollback(st, sessionimport.Options{Tenant: *tenant, DenyFile: denyFile, DryRun: dryRun})
		if err != nil {
			writeReport(stdout, rep)
			fmt.Fprintln(stderr, "import failed")
			return 1
		}
		writeReport(stdout, rep)
		return 0
	}
	f, err := os.Open(*exportPath)
	if err != nil {
		fmt.Fprintln(stderr, "open export failed")
		return 1
	}
	defer f.Close()
	st, closeDB, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	defer closeDB()
	rep, err := sessionimport.Import(st, f, sessionimport.Options{
		Tenant:       *tenant,
		SourceTenant: *source,
		DenyFile:     denyFile,
		DryRun:       dryRun,
	})
	if err != nil {
		writeReport(stdout, rep)
		var cfg *sessionimport.ConfigError
		if errors.As(err, &cfg) {
			fmt.Fprintln(stderr, cfg.Error())
		} else {
			fmt.Fprintln(stderr, "import failed")
		}
		return 1
	}
	writeReport(stdout, rep)
	return 0
}

func writeReport(w io.Writer, rep any) {
	if rep == nil {
		return
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(rep)
}

const usage = "usage: goso-sessions-import --export file.jsonl --tenant id --source-tenant id --db path --deny-tenants file [--apply]\n       goso-sessions-import --rollback --tenant id --db path --deny-tenants file [--apply]\ndry-run is the default and writes nothing. --apply writes. Do not pass --apply and --dry-run together.\nThe denylist file is required. A missing, empty, or malformed file stops before any write. A listed tenant is refused. Reports include a denied count and not the listed names."

func badToken(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func openDB(path string) (sessionimport.SQLDB, func() error, error) {
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
