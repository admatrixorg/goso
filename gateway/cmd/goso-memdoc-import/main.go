// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-memdoc-import loads memory_documents from an offline JSONL
// export into an existing goso database. It does not open GOSO_DATABASE_URL,
// and it refuses a non-local Postgres host. memory_chunks are not imported.
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
	mapPath := fs.String("map", "", "JSON tenant, agent, and optional customer mapping")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	vaultDir := fs.String("vault-dir", "", "vault root; files are written under t/<tenant>/")
	dry := fs.Bool("dry-run", false, "report counts and _system file names without writing")
	remove := fs.String("remove-tenant", "", "delete gc- rows and their vault files for this goso tenant")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*remove) != "" {
		if *dry || strings.TrimSpace(*exportPath) != "" || strings.TrimSpace(*mapPath) != "" {
			fmt.Fprintln(stderr, "usage: goso-memdoc-import --remove-tenant id --db path --vault-dir dir")
			return 2
		}
		if strings.TrimSpace(*dbPath) == "" || strings.TrimSpace(*vaultDir) == "" {
			fmt.Fprintln(stderr, "usage: goso-memdoc-import --remove-tenant id --db path --vault-dir dir")
			return 2
		}
		return runRemove(stdout, stderr, *dbPath, *vaultDir, *remove)
	}
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*mapPath) == "" || strings.TrimSpace(*dbPath) == "" || strings.TrimSpace(*vaultDir) == "" {
		fmt.Fprintln(stderr, "usage: goso-memdoc-import --export file.jsonl --map map.json --db path --vault-dir dir [--dry-run]")
		return 2
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
	ef, err := os.Open(*exportPath)
	if err != nil {
		fmt.Fprintln(stderr, "open export failed")
		return 1
	}
	defer ef.Close()
	st, closeDB, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	defer closeDB()
	rep, err := memdocimport.Import(st, vault.New(st, *vaultDir), ef, memdocimport.Options{
		Mapping: mapping,
		DryRun:  *dry,
	})
	if err != nil {
		return fail(stderr, err)
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
