// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-kg-import loads kg_entities and kg_relations from a JSONL
// export into an existing goso database. It does not open a GoClaw
// database, does not follow GOSO_DATABASE_URL, and refuses a non-local
// Postgres host.
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

	"github.com/mqglobal/goso/gateway/internal/kgimport"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("goso-kg-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	exportPath := fs.String("export", "", "JSONL file of kg_entities and kg_relations rows (not a database)")
	tenantMapPath := fs.String("tenant-map", "", "SPEC 152 file: source_tenant, dest_tenant, slug")
	agentMapPath := fs.String("agent-map", "", "SPEC 174 file: goclaw_agent_id, goso_agent_id, source_tenant")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	dry := fs.Bool("dry-run", false, "print per-tenant counts and write nothing")
	remove := fs.Bool("remove", false, "delete gc- kg rows for --dest-tenant")
	dest := fs.String("dest-tenant", "", "goso tenant id for --remove")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*dbPath) == "" {
		fmt.Fprintln(stderr, "usage: goso-kg-import --export file.jsonl --tenant-map tenants.json --agent-map agents.json --db path [--dry-run]")
		fmt.Fprintln(stderr, "       goso-kg-import --remove --dest-tenant id --db path [--dry-run]")
		return 2
	}
	if *remove && (strings.TrimSpace(*exportPath) != "" || strings.TrimSpace(*tenantMapPath) != "" || strings.TrimSpace(*agentMapPath) != "") {
		fmt.Fprintln(stderr, "--remove does not take an export")
		return 2
	}
	st, closeDB, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	defer closeDB()
	if *remove {
		if strings.TrimSpace(*dest) == "" {
			fmt.Fprintln(stderr, "--dest-tenant is required with --remove")
			return 2
		}
		rep, err := kgimport.Remove(st, *dest, *dry)
		if err != nil {
			return fail(stderr, err)
		}
		return writeJSON(stdout, stderr, rep)
	}
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*tenantMapPath) == "" || strings.TrimSpace(*agentMapPath) == "" {
		fmt.Fprintln(stderr, "usage: goso-kg-import --export file.jsonl --tenant-map tenants.json --agent-map agents.json --db path [--dry-run]")
		return 2
	}
	if store.IsPostgresDSN(*exportPath) {
		fmt.Fprintln(stderr, "export is a JSONL file, not a database")
		return 2
	}
	ex, err := readExport(*exportPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	tenants, err := readTenantMap(*tenantMapPath)
	if err != nil {
		return fail(stderr, err)
	}
	agents, err := readAgentMap(*agentMapPath)
	if err != nil {
		return fail(stderr, err)
	}
	rep, err := kgimport.Import(st, ex, kgimport.Options{Tenants: tenants, Agents: agents, DryRun: *dry})
	if err != nil {
		return fail(stderr, err)
	}
	return writeJSON(stdout, stderr, rep)
}

func readExport(path string) (*kgimport.Export, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("open export failed")
	}
	defer f.Close()
	ex, err := kgimport.ReadExport(f)
	if err != nil {
		return nil, err
	}
	return ex, nil
}

func readTenantMap(path string) (kgimport.TenantMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return kgimport.TenantMap{}, errConfigOpen("tenant map")
	}
	defer f.Close()
	return kgimport.ReadTenantMap(f)
}

func readAgentMap(path string) (kgimport.AgentMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return kgimport.AgentMap{}, errConfigOpen("agent map")
	}
	defer f.Close()
	return kgimport.ReadAgentMap(f)
}

func errConfigOpen(kind string) error {
	return &kgimport.ConfigError{Msg: "open " + kind + " failed"}
}

func fail(stderr io.Writer, err error) int {
	var cfg *kgimport.ConfigError
	if errors.As(err, &cfg) {
		fmt.Fprintln(stderr, cfg.Error())
		return 2
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

func openDB(path string) (kgimport.Target, func() error, error) {
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
