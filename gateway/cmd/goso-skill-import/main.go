// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Command goso-skill-import loads one tenant of skills from an offline
// GoClaw JSON or JSONL export into GOSO_SKILLS_DIR-style folders. The default
// is a dry-run. It writes the rename map and does not write skill files
// until --apply. It does not open a database and it does not execute skill files.
package main

import (
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
		fmt.Fprintln(stderr, "usage: goso-skill-import --export file.json --tenant id --skills-dir dir --src-dir dir --rename-map file.json [--apply]")
		fmt.Fprintln(stderr, "default: dry-run, writes no skill files. --apply writes new folders only. --apply together with --dry-run is an error.")
	}
	exportPath := fs.String("export", "", "JSON or JSONL export of skills, skill_versions, skill_agent_grants, skill_user_grants, skill_tenant_configs")
	tenant := fs.String("tenant", "", "export tenant_id to import; other tenants are counted and not written")
	skillsDir := fs.String("skills-dir", "", "Goso skills directory (one folder per skill, SKILL.md inside)")
	srcDir := fs.String("src-dir", "", "directory the export file_path values are relative to")
	renameMap := fs.String("rename-map", "", "old-to-new name file; written on dry-run and required to match on apply")
	apply := fs.Bool("apply", false, "write new skill folders")
	dry := fs.Bool("dry-run", false, "report only; this is the default when --apply is absent")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *apply && *dry {
		fmt.Fprintln(stderr, "--apply and --dry-run cannot be combined")
		return 2
	}
	if strings.TrimSpace(*exportPath) == "" || strings.TrimSpace(*tenant) == "" || strings.TrimSpace(*skillsDir) == "" || strings.TrimSpace(*srcDir) == "" || strings.TrimSpace(*renameMap) == "" {
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
		SkillsDir:     *skillsDir,
		SrcDir:        *srcDir,
		RenameMapPath: *renameMap,
		Apply:         *apply,
		Log:           stderrLog{w: stderr},
	})
	if err != nil {
		var cfg *skillimport.ConfigError
		if errors.As(err, &cfg) {
			fmt.Fprintln(stderr, cfg.Error())
		} else {
			fmt.Fprintln(stderr, "import failed")
		}
		return 1
	}
	raw, err := skillimport.FormatReport(rep)
	if err != nil {
		fmt.Fprintln(stderr, "write report failed")
		return 1
	}
	if _, err := stdout.Write(raw); err != nil {
		fmt.Fprintln(stderr, "write report failed")
		return 1
	}
	return 0
}

type stderrLog struct{ w io.Writer }

func (l stderrLog) Printf(format string, args ...any) {
	fmt.Fprintf(l.w, format+"\n", args...)
}
