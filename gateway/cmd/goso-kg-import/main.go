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
	customerMapPath := fs.String("customer-map", "", "PR 46 file: channel, sender_id, user_id. Optional. Fallback is goclaw:<user_id>")
	denyPath := fs.String("deny-file", "", "local denylist of tenant ids. Required. Also read from GOSO_TENANT_DENY_FILE")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	source := fs.String("source-tenant", "", "GoClaw tenant id for this run")
	tenant := fs.String("tenant", "", "goso tenant id to write")
	dry := fs.Bool("dry-run", false, "print per-tenant counts and write nothing")
	remove := fs.Bool("remove", false, "delete gc- kg rows for --tenant")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	denyFile := strings.TrimSpace(*denyPath)
	if denyFile == "" {
		denyFile = strings.TrimSpace(os.Getenv("GOSO_TENANT_DENY_FILE"))
	}
	if denyFile == "" {
		fmt.Fprintln(stderr, "deny file is required")
		return 1
	}
	deny, err := readDeny(denyFile)
	if err != nil {
		return fail(stderr, err)
	}
	if strings.TrimSpace(*dbPath) == "" {
		fmt.Fprintln(stderr, "usage: goso-kg-import --export file.jsonl --tenant-map tenants.json --agent-map agents.json --source-tenant id --tenant id --deny-file path --db path [--customer-map file.json] [--dry-run]")
		fmt.Fprintln(stderr, "       goso-kg-import --remove --tenant id --deny-file path --db path [--dry-run]")
		return 2
	}
	if *remove && (strings.TrimSpace(*exportPath) != "" || strings.TrimSpace(*tenantMapPath) != "" || strings.TrimSpace(*agentMapPath) != "") {
		fmt.Fprintln(stderr, "--remove does not take an export")
		return 2
	}
	dest := strings.TrimSpace(*tenant)
	if dest == "" {
		fmt.Fprintln(stderr, "--tenant is required")
		return 2
	}
	if deny.Has(dest) || deny.Has(*source) {
		n := 0
		if deny.Has(*source) && strings.TrimSpace(*source) != "" {
			n++
		}
		if deny.Has(dest) && !strings.EqualFold(strings.TrimSpace(*source), dest) {
			n++
		}
		if n == 0 {
			n = 1
		}
		_ = writeJSON(stdout, stderr, kgimport.Report{DryRun: *dry, DeniedTenants: n, Tables: []kgimport.Section{}})
		fmt.Fprintln(stderr, "tenant is denied")
		return 1
	}
	st, closeDB, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	defer closeDB()
	if *remove {
		rep, err := kgimport.Remove(st, dest, deny, *dry)
		if err != nil {
			return fail(stderr, err)
		}
		return writeJSON(stdout, stderr, rep)
	}
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*tenantMapPath) == "" || strings.TrimSpace(*agentMapPath) == "" || strings.TrimSpace(*source) == "" {
		fmt.Fprintln(stderr, "usage: goso-kg-import --export file.jsonl --tenant-map tenants.json --agent-map agents.json --source-tenant id --tenant id --deny-file path --db path [--customer-map file.json] [--dry-run]")
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
	var customers kgimport.CustomerMap
	if strings.TrimSpace(*customerMapPath) != "" {
		customers, err = readCustomerMap(*customerMapPath)
		if err != nil {
			return fail(stderr, err)
		}
	}
	rep, err := kgimport.Import(st, ex, kgimport.Options{
		SourceTenant: *source,
		DestTenant:   dest,
		Tenants:      tenants,
		Agents:       agents,
		Customers:    customers,
		Deny:         deny,
		DryRun:       *dry,
	})
	if rep != nil {
		if code := writeJSON(stdout, stderr, rep); code != 0 {
			return code
		}
	}
	if err != nil {
		return fail(stderr, err)
	}
	return 0
}

func readDeny(path string) (kgimport.DenyList, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return kgimport.DenyList{}, &kgimport.ConfigError{Msg: "deny file is missing"}
		}
		return kgimport.DenyList{}, &kgimport.ConfigError{Msg: "deny file is missing"}
	}
	defer f.Close()
	return kgimport.ReadDenyFile(f)
}

func readCustomerMap(path string) (kgimport.CustomerMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return kgimport.CustomerMap{}, errConfigOpen("customer map")
	}
	defer f.Close()
	return kgimport.ReadCustomerMap(f)
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
