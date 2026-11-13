// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-memdoc-import loads memory_documents from an offline JSONL
// export into an existing goso database. It does not open GOSO_DATABASE_URL,
// and it refuses a non-local Postgres host. memory_chunks are not imported.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/memdocimport"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("goso-memdoc-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	exportPath := fs.String("export", "", "JSONL export of memory_documents (one object per line)")
	mapPath := fs.String("map", "", "JSON tenant and agent mapping")
	tenant := fs.String("tenant", "", "GoClaw source tenant id or slug for this run")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	vaultDir := fs.String("vault-dir", "", "vault root; files are written under t/<tenant>/")
	dry := fs.Bool("dry-run", false, "report counts without writing")
	remove := fs.String("remove-tenant", "", "delete this importer's gc-doc, gc-docsess, and gc-vdoc rows for this goso tenant")
	denyPath := fs.String("deny-file", "", "required local denylist, one tenant id per line (default $GOSO_TENANT_DENYLIST)")
	customerMap := fs.String("customer-map", "", "channel_contacts JSON or JSONL; optional customer_key map")
	systemNames := fs.String("system-names-out", "", "write _system file names to this file (mode 0600); stdout stays counts")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	deny, err := openDeny(*denyPath)
	if err != nil {
		return fail(stderr, err)
	}
	if strings.TrimSpace(*remove) != "" {
		if *dry || strings.TrimSpace(*exportPath) != "" || strings.TrimSpace(*mapPath) != "" || strings.TrimSpace(*tenant) != "" {
			fmt.Fprintln(stderr, "usage: goso-memdoc-import --remove-tenant id --db path --vault-dir dir")
			return 2
		}
		if strings.TrimSpace(*dbPath) == "" || strings.TrimSpace(*vaultDir) == "" {
			fmt.Fprintln(stderr, "usage: goso-memdoc-import --remove-tenant id --db path --vault-dir dir")
			return 2
		}
		if deny.Hit(*remove) {
			return denyExit(stdout, stderr, 1)
		}
		return runRemove(stdout, stderr, *dbPath, *vaultDir, *remove)
	}
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*mapPath) == "" || strings.TrimSpace(*tenant) == "" || strings.TrimSpace(*dbPath) == "" || strings.TrimSpace(*vaultDir) == "" {
		fmt.Fprintln(stderr, "usage: goso-memdoc-import --export file.jsonl --map map.json --tenant id --db path --vault-dir dir [--customer-map file] [--deny-file file] [--dry-run]")
		return 2
	}
	var customers *memdocimport.CustomerIndex
	if strings.TrimSpace(*customerMap) != "" {
		cf, err := os.Open(*customerMap)
		if err != nil {
			fmt.Fprintln(stderr, "open customer map failed")
			return 1
		}
		defer cf.Close()
		customers, err = memdocimport.ReadCustomerMap(cf)
		if err != nil {
			return fail(stderr, err)
		}
	}
	mf, err := os.Open(*mapPath)
	if err != nil {
		fmt.Fprintln(stderr, "open mapping failed")
		return 1
	}
	defer mf.Close()
	mapping, err := memdocimport.ReadMapping(mf)
	if err != nil {
		return fail(stderr, err)
	}
	rawExport, err := os.ReadFile(*exportPath)
	if err != nil {
		fmt.Fprintln(stderr, "open export failed")
		return 1
	}
	if n, err := memdocimport.CountDenied(deny, *tenant, mapping, bytes.NewReader(rawExport)); err != nil {
		return fail(stderr, err)
	} else if n > 0 {
		return denyExit(stdout, stderr, n)
	}
	st, closeDB, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	defer closeDB()
	rep, err := memdocimport.Import(st, vault.New(st, *vaultDir), bytes.NewReader(rawExport), memdocimport.Options{
		Mapping:      mapping,
		SourceTenant: *tenant,
		Deny:         deny,
		Customers:    customers,
		DryRun:       *dry,
	})
	if err != nil {
		var denied *memdocimport.DeniedError
		if errors.As(err, &denied) {
			return denyExit(stdout, stderr, denied.Count)
		}
		return fail(stderr, err)
	}
	if err := writeSystemNames(*systemNames, systemNameList(rep)); err != nil {
		fmt.Fprintln(stderr, "system names file failed")
		return 1
	}
	return writeJSON(stdout, stderr, rep)
}

func runRemove(stdout, stderr io.Writer, dbPath, vaultDir, tenant string) int {
	st, closeDB, err := openDB(dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	defer closeDB()
	res, err := memdocimport.RemoveTenant(st, vaultDir, tenant)
	if err != nil {
		return fail(stderr, err)
	}
	return writeJSON(stdout, stderr, res)
}

func fail(stderr io.Writer, err error) int {
	var cfg *memdocimport.ConfigError
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

func denyExit(stdout, stderr io.Writer, n int) int {
	if n < 1 {
		n = 1
	}
	if writeJSON(stdout, stderr, map[string]int{"denied": n}) != 0 {
		return 1
	}
	fmt.Fprintln(stderr, "tenant is denied")
	return 2
}

func systemNameList(rep *memdocimport.Report) []string {
	if rep == nil {
		return nil
	}
	var names []string
	for _, sec := range rep.Tenants {
		names = append(names, sec.SystemNames...)
	}
	return names
}

func writeSystemNames(path string, names []string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	var b strings.Builder
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, "\r\n") {
			continue
		}
		b.WriteString(name)
		b.WriteByte('\n')
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	_, err = io.WriteString(f, b.String())
	return err
}

func openDeny(path string) (*memdocimport.DenyList, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = strings.TrimSpace(os.Getenv("GOSO_TENANT_DENYLIST"))
	}
	if path == "" {
		return nil, errConfigMissing()
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errConfigMissing()
	}
	defer f.Close()
	return memdocimport.ReadDenyList(f)
}

func errConfigMissing() error {
	return &memdocimport.ConfigError{Msg: "denylist is missing"}
}

func openDB(path string) (memdocimport.Store, func() error, error) {
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
