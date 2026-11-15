// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-provider-import loads one tenant's GoClaw llm_providers rows
// into goso's llm_providers table. The default is a dry-run; writing needs
// --apply. It does not read GOSO_DATABASE_URL and refuses a non-local host,
// so an operator cannot point it at a live server by accident.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/providerimport"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("goso-provider-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: goso-provider-import --export file.json --source-tenant id --tenant id --db path --deny-file path [--apply]")
		fmt.Fprintln(stderr, "default: dry-run, writes nothing. --apply writes.")
		fmt.Fprintln(stderr, "--deny-file is required. A missing, empty, or malformed file stops before any work.")
	}
	exportPath := fs.String("export", "", "JSON or JSONL export of GoClaw llm_providers")
	source := fs.String("source-tenant", "", "export tenant_id to keep")
	dest := fs.String("tenant", "", "goso tenant_id to write")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	denyPath := fs.String("deny-file", "", "local denylist, one tenant id per line")
	apply := fs.Bool("apply", false, "write rows")
	dry := fs.Bool("dry-run", false, "report only; this is the default when --apply is absent")
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
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*source) == "" || strings.TrimSpace(*dest) == "" || strings.TrimSpace(*dbPath) == "" || strings.TrimSpace(*denyPath) == "" {
		fs.Usage()
		return 2
	}

	deny, err := readDeny(*denyPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}

	f, err := os.Open(*exportPath)
	if err != nil {
		fmt.Fprintln(stderr, "open export failed")
		return 1
	}
	defer f.Close()
	ex, err := providerimport.ReadExport(f)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}

	st, closeDB, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	defer closeDB()

	rep, err := providerimport.Import(st, ex, providerimport.Options{
		SourceTenant: *source,
		DestTenant:   *dest,
		Apply:        *apply,
		Deny:         deny,
	})
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fmt.Fprintln(stderr, "report failed")
		return 1
	}
	fmt.Fprintln(stdout, string(raw))
	return 0
}

func readDeny(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("denylist is missing")
	}
	defer f.Close()
	out := map[string]struct{}{}
	buf := make([]byte, 64<<10)
	var acc []byte
	for {
		n, err := f.Read(buf)
		acc = append(acc, buf[:n]...)
		if err != nil {
			break
		}
	}
	for _, line := range strings.Split(string(acc), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.ContainsAny(line, " \t") {
			return nil, errors.New("denylist is malformed")
		}
		out[strings.ToLower(line)] = struct{}{}
	}
	if len(out) == 0 {
		return nil, errors.New("denylist is empty")
	}
	return out, nil
}

func openDB(path string) (store.StoreIface, func(), error) {
	path = strings.TrimSpace(path)
	if store.IsPostgresDSN(path) {
		if err := requireLoopback(path); err != nil {
			return nil, nil, err
		}
		s, err := store.OpenPostgres(path)
		if err != nil {
			return nil, nil, errors.New("open database failed")
		}
		return s, func() { _ = s.Close() }, nil
	}
	s, err := store.OpenSQLite(path)
	if errors.Is(err, store.ErrPostgresUnsupported) {
		return nil, nil, errors.New("refusing GOSO_DATABASE_URL; this tool only opens --db")
	}
	if err != nil {
		return nil, nil, errors.New("open database failed")
	}
	return s, func() { _ = s.Close() }, nil
}

func requireLoopback(dsn string) error {
	u, err := url.Parse(dsn)
	if err != nil {
		return errors.New("refusing non-local database host")
	}
	switch strings.ToLower(strings.TrimSpace(u.Hostname())) {
	case "localhost", "127.0.0.1", "::1":
		return nil
	default:
		return errors.New("refusing non-local database host")
	}
}
