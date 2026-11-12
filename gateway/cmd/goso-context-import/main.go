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
		fmt.Fprintln(stderr, "usage: goso-context-import --export file.json --source-tenant id --tenant id --db path --deny-file path [--agent-map file.json] [--customer-map file.json] [--apply]")
		fmt.Fprintln(stderr, "export is JSON or JSONL with the GoClaw columns agent_id, file_name, content, and user_id for a user row. There is no database connection.")
		fmt.Fprintln(stderr, "default: dry-run, writes nothing. --apply writes. --apply together with --dry-run is an error.")
		fmt.Fprintln(stderr, "--deny-file is required (or GOSO_TENANT_DENYLIST). A missing, empty, or malformed file stops before any work.")
		fmt.Fprintln(stderr, "--rollback counts mapped files for --tenant. --rollback --apply deletes only those whose body is unchanged.")
	}
	exportPath := fs.String("export", "", "JSON or JSONL export of agent_context_files and user_context_files")
	source := fs.String("source-tenant", "", "export tenant_id to keep; other tenants are rejected")
	dest := fs.String("tenant", "", "goso tenant_id to write")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	agentMapPath := fs.String("agent-map", "", "JSON object of GoClaw agent_id to goso agent_key")
	customerMapPath := fs.String("customer-map", "", "JSON array of channel, sender_id, user_id")
	denyFlag := fs.String("deny-file", "", "local tenant denylist; required")
	apply := fs.Bool("apply", false, "write context files")
	dry := fs.Bool("dry-run", false, "report only; this is the default when --apply is absent")
	rollback := fs.Bool("rollback", false, "remove mapped files whose body is unchanged since import")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *apply && *dry {
		fmt.Fprintln(stderr, "--apply and --dry-run cannot be combined")
		return 2
	}
	deny := denyPath(*denyFlag)
	if *rollback {
		if strings.TrimSpace(*dest) == "" || strings.TrimSpace(*dbPath) == "" {
			fs.Usage()
			return 2
		}
		if code := stopIfDenied(stdout, stderr, deny, "", *dest); code != 0 {
			return code
		}
		return runRollback(stdout, stderr, *dest, *dbPath, deny, *apply)
	}
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*source) == "" || strings.TrimSpace(*dest) == "" || strings.TrimSpace(*dbPath) == "" {
		fs.Usage()
		return 2
	}
	if code := stopIfDenied(stdout, stderr, deny, *source, *dest); code != 0 {
		return code
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
	var agentMap map[string]string
	if strings.TrimSpace(*agentMapPath) != "" {
		mf, err := os.Open(*agentMapPath)
		if err != nil {
			fmt.Fprintln(stderr, "open agent map failed")
			return 1
		}
		agentMap, err = contextimport.ReadAgentMap(mf)
		mf.Close()
		if err != nil {
			fmt.Fprintln(stderr, "read agent map failed")
			return 1
		}
	}
	var customers map[string]string
	if strings.TrimSpace(*customerMapPath) != "" {
		cf, err := os.Open(*customerMapPath)
		if err != nil {
			fmt.Fprintln(stderr, "open customer map failed")
			return 1
		}
		customers, err = contextimport.ReadCustomerMap(cf)
		cf.Close()
		if err != nil {
			fmt.Fprintln(stderr, "read customer map failed")
			return 1
		}
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
		AgentMap:     agentMap,
		Customers:    customers,
		DenyFile:     deny,
		Log:          stderrLog{w: stderr},
	})
	if err != nil {
		if writeDeny(stdout, stderr, err) {
			return 1
		}
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

func denyPath(flag string) string {
	if s := strings.TrimSpace(flag); s != "" {
		return s
	}
	return strings.TrimSpace(os.Getenv("GOSO_TENANT_DENYLIST"))
}

func stopIfDenied(stdout, stderr io.Writer, path, source, dest string) int {
	n, err := contextimport.CheckDeny(path, source, dest)
	if err != nil {
		var fileErr *contextimport.FileError
		if errors.As(err, &fileErr) {
			fmt.Fprintln(stderr, fileErr.Error())
		} else {
			fmt.Fprintln(stderr, "denylist is malformed")
		}
		return 1
	}
	if n > 0 {
		writeCount(stdout, n)
		return 1
	}
	return 0
}

func writeDeny(stdout, stderr io.Writer, err error) bool {
	var denied *contextimport.DenyError
	if errors.As(err, &denied) {
		writeCount(stdout, denied.Count)
		return true
	}
	var fileErr *contextimport.FileError
	if errors.As(err, &fileErr) {
		fmt.Fprintln(stderr, fileErr.Error())
		return true
	}
	return false
}

func writeCount(w io.Writer, n int) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(struct {
		DeniedTenants int `json:"denied_tenants"`
	}{DeniedTenants: n})
}

func runRollback(stdout, stderr io.Writer, dest, dbPath, deny string, apply bool) int {
	st, closeDB, err := openDB(dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	defer closeDB()
	rep, err := contextimport.Rollback(st, dest, deny, apply)
	if err != nil {
		if writeDeny(stdout, stderr, err) {
			return 1
		}
		var cfg *contextimport.ConfigError
		if errors.As(err, &cfg) {
			fmt.Fprintln(stderr, cfg.Error())
		} else {
			fmt.Fprintln(stderr, "rollback failed")
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

type stderrLog struct{ w io.Writer }

func (l stderrLog) Printf(format string, args ...any) {
	fmt.Fprintf(l.w, format+"\n", args...)
}
