// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-cron-channel-import loads cron_jobs and channel_instances from
// an offline GoClaw JSON or JSONL export into an existing goso database.
// Every imported cron job and channel is stored disabled. Credentials are not
// copied. The default is a dry-run. --apply writes. It does not open
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

	"github.com/mqglobal/goso/gateway/internal/scheduleimport"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("goso-cron-channel-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	exportPath := fs.String("export", "", "JSON or JSONL export with cron_jobs and channel_instances")
	source := fs.String("source-tenant", "", "export tenant_id to import")
	dest := fs.String("tenant", "", "goso tenant to write")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	apply := fs.Bool("apply", false, "write disabled rows; without this flag the run only prints counts")
	dry := fs.Bool("dry-run", false, "print per-tenant counts and write nothing")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*source) == "" || strings.TrimSpace(*dest) == "" || strings.TrimSpace(*dbPath) == "" {
		fmt.Fprintln(stderr, "usage: goso-cron-channel-import --export file.json --source-tenant id --tenant id --db path [--dry-run] [--apply]")
		return 2
	}
	if *apply && *dry {
		fmt.Fprintln(stderr, "--apply and --dry-run cannot be used together")
		return 2
	}
	f, err := os.Open(*exportPath)
	if err != nil {
		fmt.Fprintln(stderr, "open export failed")
		return 1
	}
	defer f.Close()
	ex, err := scheduleimport.ReadExport(f)
	if err != nil {
		fmt.Fprintln(stderr, "read export failed")
		return 1
	}
	st, closeDB, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	defer closeDB()
	rep, err := scheduleimport.Import(st, ex, scheduleimport.Options{
		SourceTenant: *source,
		DestTenant:   *dest,
		DryRun:       !*apply,
		Log:          stderrLog{w: stderr},
	})
	if err != nil {
		var cfg *scheduleimport.ConfigError
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
