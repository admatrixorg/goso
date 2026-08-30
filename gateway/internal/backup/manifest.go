// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package backup

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manifest is the sidecar metadata for a snapshot. No credentials.
type Manifest struct {
	Schema         string         `json:"schema"`
	SchemaVersion  int            `json:"schema_version"`
	SecretPolicy   string         `json:"secret_policy"`
	IncludeSecrets bool           `json:"include_secrets"`
	Scope          string         `json:"scope"`
	Tenant         string         `json:"tenant,omitempty"`
	File           string         `json:"file"`
	Engine         string         `json:"engine,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	Counts         map[string]int `json:"counts,omitempty"`
	Recovery       Recovery       `json:"recovery"`
}

// Recovery describes how a failed apply is rolled back.
type Recovery struct {
	Strategy         string `json:"strategy"`
	PreRestoreSuffix string `json:"pre_restore_suffix"`
	TempCleanup      bool   `json:"temp_cleanup"`
	LiveApplyCLIOnly bool   `json:"live_apply_cli_only"`
}

func manifestPath(dbPath string) string {
	return dbPath + ".meta.json"
}

// WriteManifest stores sidecar metadata next to the snapshot.
func WriteManifest(dbPath string, res SnapshotResult) error {
	engine := "sqlite"
	man := Manifest{
		Schema:         SchemaBackup,
		SchemaVersion:  SchemaVersion,
		SecretPolicy:   SecretExcluded,
		IncludeSecrets: false,
		Scope:          res.Scope,
		Tenant:         res.Tenant,
		File:           filepath.Base(dbPath),
		Engine:         engine,
		CreatedAt:      time.Now().UTC(),
		Counts:         res.Counts,
		Recovery: Recovery{
			Strategy:         "pre_restore_rename",
			PreRestoreSuffix: ".pre-restore",
			TempCleanup:      true,
			LiveApplyCLIOnly: true,
		},
	}
	raw, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath(dbPath), raw, 0o644)
}

// ReadManifest loads sidecar metadata. Missing sidecar is not fatal for callers.
func ReadManifest(dbPath string) (Manifest, error) {
	raw, err := os.ReadFile(manifestPath(dbPath))
	if err != nil {
		return Manifest{}, err
	}
	var man Manifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return Manifest{}, err
	}
	return man, nil
}

// TableCounts returns row counts for known tables. Missing tables are omitted.
func TableCounts(path string) (map[string]int, error) {
	db, err := openSQLite(path, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	names := []string{"agents", "sessions", "messages", "teams", "memories", "vault_docs", "webhooks", "llm_providers", "secrets"}
	out := map[string]int{}
	for _, name := range names {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + name).Scan(&n); err != nil {
			if missingTable(err) {
				continue
			}
			return out, err
		}
		out[name] = n
	}
	return out, nil
}

// ValidateArchive checks integrity, schema, and that credentials were stripped.
func ValidateArchive(name string) (Manifest, error) {
	src, err := Resolve(name)
	if err != nil {
		return Manifest{}, err
	}
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, ErrNotFound
		}
		return Manifest{}, err
	}
	if err := IntegrityCheck(src); err != nil {
		return Manifest{}, err
	}
	if err := assertNoSecrets(src); err != nil {
		return Manifest{}, err
	}
	man, err := ReadManifest(src)
	if err != nil {
		man = Manifest{
			Schema:         SchemaBackup,
			SchemaVersion:  SchemaVersion,
			SecretPolicy:   SecretExcluded,
			IncludeSecrets: false,
			File:           filepath.Base(src),
			Scope:          ScopeSystem,
			Recovery: Recovery{
				Strategy:         "pre_restore_rename",
				PreRestoreSuffix: ".pre-restore",
				TempCleanup:      true,
				LiveApplyCLIOnly: true,
			},
		}
		if strings.Contains(filepath.Base(src), "-t-") {
			man.Scope = ScopeTenant
		}
	}
	if man.Schema != "" && man.Schema != SchemaBackup {
		return man, ErrInvalidArchive
	}
	if man.IncludeSecrets {
		return man, ErrInvalidArchive
	}
	if man.SecretPolicy != "" && man.SecretPolicy != SecretExcluded {
		return man, ErrInvalidArchive
	}
	if counts, err := TableCounts(src); err == nil {
		man.Counts = counts
	}
	return man, nil
}

// OpenSanitized returns a snapshot path that has passed integrity and secret policy.
// If the stored file still has credential rows (pre-117 copies), a temp sanitized
// copy is used. Caller must invoke cleanup.
func OpenSanitized(name string) (path string, cleanup func(), err error) {
	src, err := Resolve(name)
	if err != nil {
		return "", func() {}, err
	}
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return "", func() {}, ErrNotFound
		}
		return "", func() {}, err
	}
	if err := IntegrityCheck(src); err != nil {
		return "", func() {}, err
	}
	if err := assertNoSecrets(src); err == nil {
		return src, func() {}, nil
	}
	tmp, err := os.CreateTemp("", "goso-dl-*.db")
	if err != nil {
		return "", func() {}, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	cleanup = func() { _ = os.Remove(tmpPath) }
	in, err := os.Open(src)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	out, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		_ = in.Close()
		cleanup()
		return "", func() {}, err
	}
	_, copyErr := io.Copy(out, in)
	_ = in.Close()
	_ = out.Close()
	if copyErr != nil {
		cleanup()
		return "", func() {}, copyErr
	}
	if err := Sanitize(tmpPath); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err := IntegrityCheck(tmpPath); err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err := assertNoSecrets(tmpPath); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tmpPath, cleanup, nil
}

func assertNoSecrets(path string) error {
	db, err := openSQLite(path, true)
	if err != nil {
		return err
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM secrets`).Scan(&n); err != nil {
		if missingTable(err) {
			return nil
		}
		return err
	}
	if n > 0 {
		return ErrInvalidArchive
	}
	return nil
}
