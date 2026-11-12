// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-context-import loads one tenant from an offline context-file
// export into an existing goso database. It writes context_files only.
// The default is a dry-run. Writing requires --apply. It does not open
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

	"github.com/mqglobal/goso/gateway/internal/contextimport"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("goso-context-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: goso-context-import --export file.json --source-tenant id --tenant id --db path [--apply]")
		fmt.Fprintln(stderr, "default: dry-run, writes nothing. --apply writes. --apply together with --dry-run is an error.")
	}
	exportPath := fs.String("export", "", "JSON export with agent_context_files and user_context_files")
	source := fs.String("source-tenant", "", "export tenant_id to keep; other tenants are rejected")
	dest := fs.String("tenant", "", "goso tenant_id to write")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	apply := fs.Bool("apply", false, "write context files")
	dry := fs.Bool("dry-run", false, "report only; this is the default when --apply is absent")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *apply && *dry {
		fmt.Fprintln(stderr, "--apply and --dry-run cannot be combined")
		return 2
	}
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*source) == "" || strings.TrimSpace(*dest) == "" || strings.TrimSpace(*dbPath) == "" {
		fs.Usage()
		return 2
	}
	f, err := os.Open(*exportPath)
	if err != nil {
		fmt.Fprintln(stderr, "open export failed")
		return 1
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		fmt.Fprintln(stderr, "open export failed")
		return 1
	}
	if info.Size() > contextimport.MaxExportBytes {
		fmt.Fprintln(stderr, "export too large")
		return 1
	}
	ex, err := contextimport.ReadExport(f)
	if err != nil {
		if strings.Contains(err.Error(), "too large") {
			fmt.Fprintln(stderr, "export too large")
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
	rep, err := contextimport.Import(st, ex, contextimport.Options{
		SourceTenant: *source,
		DestTenant:   *dest,
		Apply:        *apply,
		Log:          stderrLog{w: stderr},
	})
	if err != nil {
		var cfg *contextimport.ConfigError
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

func openDB(path string) (store.StoreIface, func() error, error) {
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

type stderrLog struct{ w io.Writer }

func (l stderrLog) Printf(format string, args ...any) {
	fmt.Fprintf(l.w, format+"\n", args...)
}
