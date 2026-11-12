// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-episodic-import loads episodic_summaries from a JSONL export
// into an existing goso database. It does not open a GoClaw database and it
// refuses a non-local Postgres host.
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
	"time"

	"github.com/mqglobal/goso/gateway/internal/episodicimport"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("goso-episodic-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	exportPath := fs.String("export", "", "JSONL export of episodic_summaries (one object per line)")
	tenantPath := fs.String("tenants", "", "JSONL SPEC 152 tenant map")
	agentPath := fs.String("agents", "", "JSONL GoClaw agent id to goso agent id map")
	customerPath := fs.String("customers", "", "customer map JSON (user_id, customer_key) or JSONL")
	source := fs.String("source-tenant", "", "GoClaw tenant id or slug for this run")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	dry := fs.Bool("dry-run", false, "print counts and create nothing")
	remove := fs.Bool("remove", false, "delete this tenant's gc-ep memories and gc-sess sessions")
	tenant := fs.String("tenant", "", "goso tenant_id to write, or to remove")
	denyPath := fs.String("denylist", "", "local denylist file, one tenant id per line (or GOSO_TENANT_DENYLIST)")
	nowFlag := fs.String("now", "", "expiry clock, RFC3339 (default: current time)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *remove {
		if strings.TrimSpace(*tenant) == "" || strings.TrimSpace(*dbPath) == "" || strings.TrimSpace(*exportPath) != "" {
			fmt.Fprintln(stderr, "usage: goso-episodic-import --remove --tenant id --db path [--dry-run]")
			return 2
		}
		return runRemove(stdout, stderr, strings.TrimSpace(*tenant), strings.TrimSpace(*dbPath), *dry)
	}
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*tenantPath) == "" || strings.TrimSpace(*agentPath) == "" || strings.TrimSpace(*dbPath) == "" || strings.TrimSpace(*source) == "" || strings.TrimSpace(*tenant) == "" {
		fmt.Fprintln(stderr, "usage: goso-episodic-import --export file.jsonl --source-tenant id --tenant id --tenants tenants.jsonl --agents agents.jsonl --denylist file --db path [--customers file] [--now RFC3339] [--dry-run]")
		return 2
	}
	denyFile := strings.TrimSpace(*denyPath)
	if denyFile == "" {
		denyFile = strings.TrimSpace(os.Getenv("GOSO_TENANT_DENYLIST"))
	}
	if denyFile == "" {
		fmt.Fprintln(stderr, "denylist is missing")
		return 2
	}
	deny, err := readDenylist(denyFile)
	if err != nil {
		return fail(stderr, err)
	}
	rows, err := readExport(*exportPath)
	if err != nil {
		return fail(stderr, err)
	}
	tenants, err := readTenants(*tenantPath)
	if err != nil {
		return fail(stderr, err)
	}
	agents, err := readAgents(*agentPath)
	if err != nil {
		return fail(stderr, err)
	}
	var customers map[string]string
	if strings.TrimSpace(*customerPath) != "" {
		customers, err = readCustomers(*customerPath)
		if err != nil {
			return fail(stderr, err)
		}
	}
	now, err := parseNow(*nowFlag)
	if err != nil {
		fmt.Fprintln(stderr, "now must be RFC3339")
		return 2
	}
	db, closeDB, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	defer closeDB()
	rep, err := episodicimport.Import(db, rows, episodicimport.Options{
		Tenants:      tenants,
		Agents:       agents,
		Customers:    customers,
		Deny:         deny,
		SourceTenant: strings.TrimSpace(*source),
		DestTenant:   strings.TrimSpace(*tenant),
		Now:          now,
		DryRun:       *dry,
	})
	if errors.Is(err, episodicimport.ErrDenied) {
		_ = writeJSON(stdout, stderr, map[string]int{"denied_tenants": rep.DeniedTenants})
		return 1
	}
	if err != nil {
		return fail(stderr, err)
	}
	return writeJSON(stdout, stderr, rep)
}

func runRemove(stdout, stderr io.Writer, tenant, dbPath string, dry bool) int {
	db, closeDB, err := openDB(dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	defer closeDB()
	mem, sess, err := db.CountEpisodicGC(tenant)
	if err != nil {
		fmt.Fprintln(stderr, "remove failed")
		return 1
	}
	if !dry {
		mem, sess, err = db.DeleteEpisodicGC(tenant)
		if err != nil {
			fmt.Fprintln(stderr, "remove failed")
			return 1
		}
	}
	return writeJSON(stdout, stderr, map[string]any{
		"dry_run":  dry,
		"remove":   true,
		"tenant":   tenant,
		"memories": mem,
		"sessions": sess,
	})
}

func readExport(path string) ([]episodicimport.Parsed, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errOpen
	}
	defer f.Close()
	return episodicimport.ReadExport(f)
}

func readTenants(path string) (episodicimport.TenantMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return episodicimport.TenantMap{}, errOpen
	}
	defer f.Close()
	return episodicimport.LoadTenants(f)
}

func readDenylist(path string) (episodicimport.Denylist, error) {
	f, err := os.Open(path)
	if err != nil {
		return episodicimport.Denylist{}, errDenylistMissing
	}
	defer f.Close()
	return episodicimport.LoadDenylist(f)
}

func readCustomers(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errOpen
	}
	defer f.Close()
	return episodicimport.LoadCustomers(f)
}

func readAgents(path string) (episodicimport.AgentMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return episodicimport.AgentMap{}, errOpen
	}
	defer f.Close()
	return episodicimport.LoadAgents(f)
}

var (
	errOpen            = errors.New("open failed")
	errDenylistMissing = errors.New("denylist is missing")
)

type gcStore interface {
	episodicimport.DB
	CountEpisodicGC(string) (int, int, error)
	DeleteEpisodicGC(string) (int, int, error)
}

func openDB(path string) (gcStore, func() error, error) {
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

func parseNow(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
	}
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
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

func fail(stderr io.Writer, err error) int {
	var cfg *episodicimport.ConfigError
	if errors.As(err, &cfg) {
		fmt.Fprintln(stderr, cfg.Error())
		return 2
	}
	if errors.Is(err, errDenylistMissing) {
		fmt.Fprintln(stderr, "denylist is missing")
		return 2
	}
	if errors.Is(err, errOpen) {
		fmt.Fprintln(stderr, "open failed")
		return 1
	}
	fmt.Fprintln(stderr, "import failed")
	return 1
}

func writeJSON(stdout, stderr io.Writer, v any) int {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(stderr, "write report failed")
		return 1
	}
	return 0
}
