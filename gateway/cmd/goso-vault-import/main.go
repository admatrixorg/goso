// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-vault-import loads one tenant from an offline GoClaw vault
// JSON export into an existing goso database. It writes vault_docs and
// vault_links only. It does not open GOSO_DATABASE_URL, and it refuses a
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

	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vaultimport"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("goso-vault-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	exportPath := fs.String("export", "", "JSON export with vault_documents, vault_links, and vault_versions")
	source := fs.String("source-tenant", "", "GoClaw tenant_id to import")
	root := fs.String("root", "", "tenant root folder; paths outside it are rejected")
	dest := fs.String("dest-tenant", "", "goso tenant_id to write")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	vaultDir := fs.String("vault-dir", "", "optional vault root; writes t/<tenant>/ files when set")
	keepPrefix := fs.Bool("keep-prefix", false, "store the path with the root prefix kept (default strips the root)")
	srcDir := fs.String("src-dir", "", "source workspace; a missing file is path-missing and is not imported")
	includePersonal := fs.Bool("include-personal", false, "import scope=personal docs (default holds them; the whole tenant can read them)")
	dry := fs.Bool("dry-run", false, "report counts and rejections without writing")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*source) == "" || strings.TrimSpace(*root) == "" || strings.TrimSpace(*dest) == "" || strings.TrimSpace(*dbPath) == "" {
		fmt.Fprintln(stderr, "usage: goso-vault-import --export file.json --source-tenant id --root tenants/<tenant>|/app/workspace|. --dest-tenant id --db path [--vault-dir dir] [--src-dir dir] [--keep-prefix] [--include-personal] [--dry-run]")
		return 2
	}
	f, err := os.Open(*exportPath)
	if err != nil {
		fmt.Fprintln(stderr, "open export failed")
		return 1
	}
	defer f.Close()
	ex, err := vaultimport.ReadExport(f)
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
	rep, err := vaultimport.Import(st, ex, vaultimport.Options{
		SourceTenant:    *source,
		Root:            *root,
		DestTenant:      *dest,
		VaultRoot:       *vaultDir,
		KeepPrefix:      *keepPrefix,
		SrcDir:          *srcDir,
		IncludePersonal: *includePersonal,
		DryRun:          *dry,
		Log:             stderrLog{w: stderr},
	})
	if err != nil {
		var cfg *vaultimport.ConfigError
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
