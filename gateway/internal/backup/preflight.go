// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package backup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Check is one preflight row. Blocking failures disable Start Backup.
type Check struct {
	ID       string `json:"id"`
	OK       bool   `json:"ok"`
	Blocking bool   `json:"blocking,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// PreflightResult is GET /api/system/backup/preflight.
type PreflightResult struct {
	Engine     string  `json:"engine"`
	CanBackup  bool    `json:"can_backup"`
	CanRestore bool    `json:"can_restore"`
	Blocking   string  `json:"blocking,omitempty"`
	Checks     []Check `json:"checks"`
}

var lookPath = exec.LookPath

var pgDumpVersion = func() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pg_dump", "--version")
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// Preflight reports database/tool compatibility for backup and restore.
func Preflight() PreflightResult {
	engine := "sqlite"
	if store.IsPostgresDSN(os.Getenv("GOSO_DATABASE_URL")) {
		engine = "postgres"
	}
	checks := []Check{}
	checks = append(checks, Check{ID: "engine", OK: true, Detail: engine})

	if engine == "postgres" {
		path, err := lookPath("pg_dump")
		if err != nil || strings.TrimSpace(path) == "" {
			checks = append(checks, Check{ID: "pg_dump", OK: false, Blocking: true, Detail: "missing pg_dump"})
		} else if ver, err := pgDumpVersion(); err != nil {
			checks = append(checks, Check{ID: "pg_dump", OK: false, Blocking: true, Detail: "incompatible pg_dump"})
		} else {
			checks = append(checks, Check{ID: "pg_dump", OK: true, Detail: ver})
		}
		checks = append(checks, Check{ID: "postgres_dump", OK: false, Blocking: true, Detail: ErrPostgres.Error()})
	} else {
		src := DBPath()
		if src == "" {
			checks = append(checks, Check{ID: "sqlite_file", OK: false, Blocking: true, Detail: ErrNoFile.Error()})
		} else if st, err := os.Stat(src); err != nil {
			checks = append(checks, Check{ID: "sqlite_file", OK: false, Blocking: true, Detail: err.Error()})
		} else if st.IsDir() {
			checks = append(checks, Check{ID: "sqlite_file", OK: false, Blocking: true, Detail: "not a file"})
		} else {
			checks = append(checks, Check{ID: "sqlite_file", OK: true, Detail: filepath.Base(src)})
		}
		checks = append(checks, Check{ID: "sqlite_vacuum", OK: true, Detail: "VACUUM INTO"})
	}

	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		checks = append(checks, Check{ID: "backup_dir", OK: false, Blocking: true, Detail: err.Error()})
	} else {
		probe := filepath.Join(dir, ".goso-backup-write")
		if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
			checks = append(checks, Check{ID: "backup_dir", OK: false, Blocking: true, Detail: err.Error()})
		} else {
			_ = os.Remove(probe)
			checks = append(checks, Check{ID: "backup_dir", OK: true, Detail: dir})
		}
	}

	if free, err := diskFree(dir); err != nil {
		checks = append(checks, Check{ID: "disk", OK: true, Detail: "unavailable"})
	} else {
		ok := free > 8<<20
		checks = append(checks, Check{ID: "disk", OK: ok, Blocking: !ok, Detail: formatBytes(free)})
	}

	out := PreflightResult{Engine: engine, CanBackup: true, CanRestore: true, Checks: checks}
	for _, c := range checks {
		if c.Blocking && !c.OK {
			out.CanBackup = false
			if out.Blocking == "" {
				out.Blocking = c.ID + ": " + c.Detail
			}
		}
	}
	ents, err := os.ReadDir(dir)
	if err != nil || len(ents) == 0 {
		out.CanRestore = false
	}
	return out
}

func diskFree(path string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}

func formatBytes(n int64) string {
	if n < 1024 {
		return "lt_1kib"
	}
	if n < 1<<20 {
		return "kib_free"
	}
	return "mib_free"
}
