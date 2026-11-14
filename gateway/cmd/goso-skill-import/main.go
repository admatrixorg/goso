// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-skill-import reads one tenant of skills from an offline
// GoClaw JSON or JSONL export. The default is a dry-run. --apply does not
// write skill files: a shared skills directory cannot keep tenants apart.
// --rollback counts, then with --apply removes only folders recorded for
// --tenant. A local denylist file is required.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/skillimport"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("goso-skill-import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: goso-skill-import --export file.json --source-tenant id --tenant id --skills-dir dir --src-dir dir --rename-map file.json --deny-tenants file [--rollback] [--apply]")
		fmt.Fprintln(stderr, "default: dry-run. --apply does not write skill files. --rollback counts mapped folders; --rollback --apply removes only that tenant.")
	}
	exportPath := fs.String("export", "", "JSON or JSONL export of skills, skill_versions, skill_agent_grants, skill_user_grants, skill_tenant_configs")
	source := fs.String("source-tenant", "", "export tenant_id to import; required")
	tenant := fs.String("tenant", "", "tenant id for this run; required")
	skillsDir := fs.String("skills-dir", "", "Goso skills directory (one folder per skill, SKILL.md inside)")
	srcDir := fs.String("src-dir", "", "directory the export file_path values are relative to")
	renameMap := fs.String("rename-map", "", "old-to-new name file written by a dry-run")
	deny := fs.String("deny-tenants", "", "local denylist file, one tenant id per line")
	rollback := fs.Bool("rollback", false, "count or remove folders recorded for --tenant")
	apply := fs.Bool("apply", false, "with --rollback, remove that tenant's mapped folders; import apply does not write")
	dry := fs.Bool("dry-run", false, "report only; this is the default when --apply is absent")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *apply && *dry {
		fmt.Fprintln(stderr, "--apply and --dry-run cannot be combined")
		return 2
	}
	denyPath := strings.TrimSpace(*deny)
	if denyPath == "" {
		denyPath = strings.TrimSpace(os.Getenv("GOSO_IMPORT_DENY_TENANTS"))
	}
	if *rollback {
		rep, err := skillimport.Rollback(skillimport.Options{
			Tenant:    *tenant,
			DenyFile:  denyPath,
			SkillsDir: *skillsDir,
			Apply:     *apply,
		})
		raw := mustRollback(rep)
		if err != nil && rep.DeniedTenants > 0 {
			stdout.Write(raw)
		}
		return finish(stdout, stderr, raw, err)
	}
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*source) == "" || strings.TrimSpace(*tenant) == "" || strings.TrimSpace(*skillsDir) == "" || strings.TrimSpace(*srcDir) == "" || strings.TrimSpace(*renameMap) == "" {
		fs.Usage()
		return 2
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
	if info.Size() > skillimport.MaxExportBytes {
		fmt.Fprintln(stderr, "export too large")
		return 1
	}
	ex, err := skillimport.ReadExport(f)
	if err != nil {
		if strings.Contains(err.Error(), "too large") {
			fmt.Fprintln(stderr, "export too large")
		} else {
			fmt.Fprintln(stderr, "read export failed")
		}
		return 1
	}
	rep, err := skillimport.Import(ex, skillimport.Options{
		Tenant:        *tenant,
		SourceTenant:  *source,
		DenyFile:      denyPath,
		SkillsDir:     *skillsDir,
		SrcDir:        *srcDir,
		RenameMapPath: *renameMap,
		Apply:         *apply,
		Log:           stderrLog{w: stderr},
	})
	raw, fmtErr := skillimport.FormatReport(rep)
	if fmtErr != nil {
		fmt.Fprintln(stderr, "write report failed")
		return 1
	}
	if err != nil && rep.DeniedTenants > 0 {
		stdout.Write(raw)
	}
	return finish(stdout, stderr, raw, err)
}

func finish(stdout, stderr io.Writer, raw []byte, err error) int {
	if err != nil {
		var cfg *skillimport.ConfigError
		if errors.As(err, &cfg) {
			fmt.Fprintln(stderr, skillimport.MaskIdentifiers(cfg.Error()))
		} else {
			fmt.Fprintln(stderr, "import failed")
		}
		return 1
	}
	if _, err := stdout.Write(raw); err != nil {
		fmt.Fprintln(stderr, "write report failed")
		return 1
	}
	return 0
}

func mustRollback(rep skillimport.RollbackReport) []byte {
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return []byte("{}\n")
	}
	return []byte(skillimport.MaskIdentifiers(string(append(raw, '\n'))))
}

type stderrLog struct{ w io.Writer }

func (l stderrLog) Printf(format string, args ...any) {
	fmt.Fprintf(l.w, "%s\n", skillimport.MaskIdentifiers(fmt.Sprintf(format, args...)))
}
