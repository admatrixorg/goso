// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-contacts-import loads one tenant from an offline contacts
// JSON file into an existing goso database. It writes the contacts tables
// only. Dry-run is the default. --apply writes. It does not open
// GOSO_DATABASE_URL, and it refuses a non-local Postgres host.
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

	"github.com/mqglobal/goso/gateway/internal/contactsimport"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("goso-contacts-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	exportPath := fs.String("export", "", "JSON file with a contacts array")
	tenant := fs.String("tenant", "", "goso tenant_id to write (required)")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	apply := fs.Bool("apply", false, "write contacts; omit to dry-run")
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
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*tenant) == "" || strings.TrimSpace(*dbPath) == "" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if !store.TenantOK(strings.TrimSpace(*tenant)) {
		fmt.Fprintln(stderr, "tenant is invalid")
		return 2
	}
	f, err := os.Open(*exportPath)
	if err != nil {
		fmt.Fprintln(stderr, "open export failed")
		return 1
	}
	defer f.Close()
	ex, err := contactsimport.ReadExport(f)
	if err != nil {
		if errors.Is(err, contactsimport.ErrExportTooLarge) {
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
	rep, err := contactsimport.Import(st, ex, contactsimport.Options{
		Tenant: *tenant,
		DryRun: dryRun,
	})
	if err != nil {
		var cfg *contactsimport.ConfigError
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

const usage = "usage: goso-contacts-import --export file.json --tenant id --db path [--apply]\ndry-run is the default; --apply writes. Do not pass --apply and --dry-run together."

func openDB(path string) (contactsimport.Store, func() error, error) {
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
