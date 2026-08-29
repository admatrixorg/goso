// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package backup

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"

	_ "modernc.org/sqlite"
)

// SnapshotResult is the admin backup JSON body.
type SnapshotResult struct {
	File      string `json:"file"`
	Bytes     int64  `json:"bytes"`
	Integrity string `json:"integrity"`
	Mtime     string `json:"mtime,omitempty"`
}

var (
	// ErrNoFile means GOSO_DB_PATH is empty or :memory:.
	ErrNoFile = errors.New("sqlite file required")
	// ErrCorrupt means PRAGMA integrity_check did not return ok.
	ErrCorrupt = errors.New("integrity check failed")
	// ErrEscape means the snapshot name is outside the backup directory.
	ErrEscape = errors.New("path escape")
	// ErrNotFound means the snapshot file is missing.
	ErrNotFound = errors.New("snapshot not found")
	// ErrPostgres means GOSO_DATABASE_URL is a postgres DSN; VACUUM INTO
	// would snapshot idle SQLite, not live PG rows.
	ErrPostgres   = errors.New("postgres backup not supported")
	errDestExists = errors.New("snapshot dest exists")
)

// Dir is GOSO_BACKUP_DIR, default ./var/backups.
func Dir() string {
	d := strings.TrimSpace(os.Getenv("GOSO_BACKUP_DIR"))
	if d == "" {
		return filepath.Join("var", "backups")
	}
	return d
}

// DBPath is the live SQLite file, or empty for memory.
func DBPath() string {
	p := strings.TrimSpace(os.Getenv("GOSO_DB_PATH"))
	if p == "" || p == ":memory:" {
		return ""
	}
	return p
}

// Snapshot writes a consistent copy of the live db via VACUUM INTO.
func Snapshot() (SnapshotResult, error) {
	if store.IsPostgresDSN(os.Getenv("GOSO_DATABASE_URL")) {
		return SnapshotResult{}, ErrPostgres
	}
	src := DBPath()
	if src == "" {
		return SnapshotResult{}, ErrNoFile
	}
	return SnapshotFile(src, Dir())
}

// SnapshotFile VACUUM INTOs src into dir as a timestamped file.
func SnapshotFile(src, dir string) (SnapshotResult, error) {
	src = strings.TrimSpace(src)
	if src == "" || src == ":memory:" {
		return SnapshotResult{}, ErrNoFile
	}
	if security.HasDotDot(src) || security.HasDotDot(dir) {
		return SnapshotResult{}, ErrEscape
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return SnapshotResult{}, err
	}
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return SnapshotResult{}, err
	}
	if _, err := os.Stat(absSrc); err != nil {
		return SnapshotResult{}, err
	}
	var dest string
	var last error
	for i := 0; i < 8; i++ {
		dest, err = uniqueDest(dir)
		if err != nil {
			return SnapshotResult{}, err
		}
		last = vacuumInto(absSrc, dest)
		if last == nil {
			break
		}
		if !errors.Is(last, errDestExists) {
			_ = os.Remove(dest)
			return SnapshotResult{}, last
		}
	}
	if last != nil {
		return SnapshotResult{}, last
	}
	if err := IntegrityCheck(dest); err != nil {
		_ = os.Remove(dest)
		return SnapshotResult{}, err
	}
	st, err := os.Stat(dest)
	if err != nil {
		return SnapshotResult{}, err
	}
	return SnapshotResult{
		File:      filepath.Base(dest),
		Bytes:     st.Size(),
		Integrity: "ok",
		Mtime:     st.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// List returns snapshots under Dir with integrity badges.
func List() ([]SnapshotResult, error) {
	dir := Dir()
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SnapshotResult{}, nil
		}
		return nil, err
	}
	out := []SnapshotResult{}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "goso-") || !strings.HasSuffix(strings.ToLower(name), ".db") {
			continue
		}
		p := filepath.Join(dir, name)
		info, err := e.Info()
		if err != nil {
			continue
		}
		item := SnapshotResult{File: name, Bytes: info.Size(), Mtime: info.ModTime().UTC().Format(time.RFC3339), Integrity: "fail"}
		if IntegrityCheck(p) == nil {
			item.Integrity = "ok"
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mtime > out[j].Mtime })
	return out, nil
}

// Resolve jails a snapshot name to Dir (basename only).
func Resolve(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || security.HasDotDot(name) {
		return "", ErrEscape
	}
	dir, err := filepath.Abs(Dir())
	if err != nil {
		return "", err
	}
	var candidate string
	if filepath.IsAbs(name) {
		candidate = name
	} else {
		candidate = filepath.Join(dir, filepath.Base(name))
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(dir, candidate)
	if err != nil {
		return "", ErrEscape
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || strings.ContainsRune(rel, os.PathSeparator) {
		return "", ErrEscape
	}
	if !strings.HasSuffix(strings.ToLower(candidate), ".db") {
		return "", ErrEscape
	}
	return candidate, nil
}

// IntegrityCheck runs PRAGMA integrity_check on path.
func IntegrityCheck(path string) error {
	db, err := openSQLite(path, true)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	defer db.Close()
	rows, err := db.Query(`PRAGMA integrity_check`)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	defer rows.Close()
	ok := false
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(s), "ok") {
			return ErrCorrupt
		}
		ok = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !ok {
		return ErrCorrupt
	}
	return nil
}

// RestoreToTemp copies a snapshot into a temp db after integrity_check.
// Caller must Close the cleanup func.
func RestoreToTemp(name string) (dest string, cleanup func(), err error) {
	src, err := Resolve(name)
	if err != nil {
		return "", nil, err
	}
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return "", nil, ErrNotFound
		}
		return "", nil, err
	}
	if err := IntegrityCheck(src); err != nil {
		return "", nil, err
	}
	tmpDir, err := os.MkdirTemp("", "goso-restore-*")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(tmpDir) }
	dest = filepath.Join(tmpDir, "restored.db")
	if err := vacuumInto(src, dest); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := IntegrityCheck(dest); err != nil {
		cleanup()
		return "", nil, err
	}
	return dest, cleanup, nil
}

// Apply replaces dest with a verified snapshot. Stop the gateway first.
func Apply(name, dest string) error {
	src, err := Resolve(name)
	if err != nil {
		return err
	}
	dest = strings.TrimSpace(dest)
	if dest == "" || dest == ":memory:" {
		return ErrNoFile
	}
	if security.HasDotDot(dest) {
		return ErrEscape
	}
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	if err := IntegrityCheck(src); err != nil {
		return err
	}
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absDest), 0o755); err != nil {
		return err
	}
	tmp := absDest + ".new"
	_ = os.Remove(tmp)
	if err := vacuumInto(src, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := IntegrityCheck(tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	bak := absDest + ".pre-restore"
	moved := false
	if _, err := os.Stat(absDest); err == nil {
		_ = os.Remove(bak)
		if err := os.Rename(absDest, bak); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		moved = true
	}
	if err := os.Rename(tmp, absDest); err != nil {
		if moved {
			_ = os.Rename(bak, absDest)
		}
		return err
	}
	return nil
}

func uniqueDest(dir string) (string, error) {
	ts := time.Now().UTC().Format("20060102T150405Z")
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	name := fmt.Sprintf("goso-%s-%s.db", ts, hex.EncodeToString(buf[:]))
	return filepath.Join(dir, name), nil
}

func vacuumInto(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return errDestExists
	}
	db, err := openSQLite(src, true)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("VACUUM INTO " + quoteLiteral(dest))
	return err
}

func openSQLite(path string, readOnly bool) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dsn := abs
	if readOnly {
		dsn = "file:" + abs + "?mode=ro"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
