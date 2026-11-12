// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-agent-import loads one tenant from an offline GoClaw agent
// export into an existing goso database. It writes agents, teams, team
// members, and agent links that already exist in goso. The default is a
// dry-run. Writing requires --apply. It does not open GOSO_DATABASE_URL,
// and it refuses a non-local Postgres host.
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

	"github.com/mqglobal/goso/gateway/internal/agentimport"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("goso-agent-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: goso-agent-import --denylist file --export file.json --source-tenant id --tenant id --db path [--apply] [--dry-run]")
		fmt.Fprintln(stderr, "       goso-agent-import --denylist file --rollback --tenant id --db path [--apply] [--confirm-rollback]")
		fmt.Fprintln(stderr, "default: dry-run, writes nothing. --apply writes. --apply together with --dry-run is an error.")
		fmt.Fprintln(stderr, "--denylist is required (or GOSO_IMPORT_DENYLIST). A missing, empty, or malformed file stops before any work.")
		fmt.Fprintln(stderr, "--rollback removes rows this tool mapped for --tenant. It does not read an export.")
	}
	exportPath := fs.String("export", "", "JSON or JSONL export with GoClaw agent column names")
	source := fs.String("source-tenant", "", "export tenant_id to keep; other tenants are counted and not written")
	dest := fs.String("tenant", "", "goso tenant_id to write")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	denyPath := fs.String("denylist", "", "local file of tenant ids to refuse; required")
	apply := fs.Bool("apply", false, "write rows, or delete mapped rows when used with --rollback")
	dry := fs.Bool("dry-run", false, "report only; this is the default when --apply is absent")
	rollback := fs.Bool("rollback", false, "remove rows previously imported into --tenant")
	confirm := fs.Bool("confirm-rollback", false, "allow rollback when sessions, tasks, or messages were added later")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}
	if *apply && *dry {
		fmt.Fprintln(stderr, "--apply and --dry-run cannot be combined")
		return 2
	}
	if *rollback && (strings.TrimSpace(*exportPath) != "" || strings.TrimSpace(*source) != "") {
		fmt.Fprintln(stderr, "--rollback does not take --export or --source-tenant")
		return 2
	}
	if *confirm && !*rollback {
		fmt.Fprintln(stderr, "--confirm-rollback requires --rollback")
		return 2
	}
	if strings.TrimSpace(*dest) == "" || strings.TrimSpace(*dbPath) == "" {
		fs.Usage()
		return 2
	}
	path := strings.TrimSpace(*denyPath)
	if path == "" {
		path = strings.TrimSpace(os.Getenv("GOSO_IMPORT_DENYLIST"))
	}
	deny, derr := agentimport.LoadDenylist(path)
	if derr != nil {
		var cfg *agentimport.ConfigError
		if errors.As(derr, &cfg) {
			fmt.Fprintln(stderr, cfg.Error())
		} else {
			fmt.Fprintln(stderr, "denylist is unreadable")
		}
		return 1
	}
	if agentimport.TenantDenied(deny, *dest) || (!*rollback && agentimport.TenantDenied(deny, *source)) {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(struct {
			DeniedTenants int `json:"denied_tenants"`
		}{DeniedTenants: len(deny)})
		fmt.Fprintln(stderr, "tenant is denied")
		return 1
	}
	st, closeDB, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	defer closeDB()
	var rep *agentimport.Report
	if *rollback {
		rep, err = agentimport.Rollback(st, *dest, agentimport.RollbackOptions{
			Apply: *apply, Confirm: *confirm, Deny: deny,
		})
	} else {
		if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*source) == "" {
			fs.Usage()
			return 2
		}
		f, oerr := os.Open(*exportPath)
		if oerr != nil {
			fmt.Fprintln(stderr, "open export failed")
			return 1
		}
		defer f.Close()
		info, serr := f.Stat()
		if serr != nil {
			fmt.Fprintln(stderr, "open export failed")
			return 1
		}
		if info.Size() > agentimport.MaxExportBytes {
			fmt.Fprintln(stderr, "export too large")
			return 1
		}
		ex, rerr := agentimport.ReadExport(f)
		if rerr != nil {
			var cfg *agentimport.ConfigError
			if errors.As(rerr, &cfg) && strings.Contains(cfg.Error(), "too large") {
				fmt.Fprintln(stderr, "export too large")
				return 1
			}
			fmt.Fprintln(stderr, "read export failed")
			return 1
		}
		rep, err = agentimport.Import(st, ex, agentimport.Options{
			SourceTenant: *source,
			DestTenant:   *dest,
			Apply:        *apply,
			Deny:         deny,
		})
	}
	if err != nil {
		var cfg *agentimport.ConfigError
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
