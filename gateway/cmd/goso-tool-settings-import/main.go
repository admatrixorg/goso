// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-tool-settings-import loads one tenant's GoClaw media tool
// settings (builtin_tools + builtin_tool_tenant_configs provider chains for
// read_image, read_document, read_video, read_audio, create_image) into
// goso's builtin_tool_settings. Provider names are mapped to goso providers
// of the destination tenant; unmapped ones are reported and dropped. The
// default is a dry-run; writing needs --apply. It does not read
// GOSO_DATABASE_URL and refuses a non-local host.
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

	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/toolsettingsimport"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("goso-tool-settings-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: goso-tool-settings-import --export file.json --source-tenant id --tenant id --db path --deny-file path [--map goclaw=goso]... [--env-provider name]... [--overwrite] [--apply]")
		fmt.Fprintln(stderr, "default: dry-run, writes nothing. --apply writes. --overwrite replaces settings a tenant already has.")
		fmt.Fprintln(stderr, "--deny-file is required. A missing, empty, or malformed file stops before any work.")
	}
	exportPath := fs.String("export", "", `JSON export: {"builtin_tools":[...],"builtin_tool_tenant_configs":[...],"llm_providers":[...]}`)
	source := fs.String("source-tenant", "", "GoClaw tenant_id to read")
	dest := fs.String("tenant", "", "goso tenant_id to write")
	dbPath := fs.String("db", "", "local SQLite file or loopback Postgres DSN")
	denyPath := fs.String("deny-file", "", "local denylist, one tenant id per line")
	apply := fs.Bool("apply", false, "write settings")
	dry := fs.Bool("dry-run", false, "report only; this is the default when --apply is absent")
	overwrite := fs.Bool("overwrite", false, "replace existing goso settings of a tool")
	var maps, envs multiFlag
	fs.Var(&maps, "map", "GoClaw provider name = goso provider name (repeatable)")
	fs.Var(&envs, "env-provider", "goso env provider name that exists on the server (repeatable)")
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
	pmap := map[string]string{}
	for _, m := range maps {
		from, to, ok := strings.Cut(m, "=")
		from, to = strings.TrimSpace(from), strings.TrimSpace(to)
		if !ok || from == "" || to == "" {
			fmt.Fprintln(stderr, "--map must look like goclaw-name=goso-name")
			return 2
		}
		pmap[from] = to
	}
	envSet := map[string]struct{}{}
	for _, e := range envs {
		if e = strings.TrimSpace(e); e != "" {
			envSet[e] = struct{}{}
		}
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
	ex, err := toolsettingsimport.ReadExport(f)
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

	rep, err := toolsettingsimport.Import(st, ex, toolsettingsimport.Options{
		SourceTenant: *source,
		DestTenant:   *dest,
		Apply:        *apply,
		Overwrite:    *overwrite,
		Deny:         deny,
		ProviderMap:  pmap,
		EnvProviders: envSet,
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
