// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestIngestJobTableAdditive is criterion 7: a new table on SQLite and
// Postgres, with no change to existing vault_docs columns.
func TestIngestJobTableAdditive(t *testing.T) {
	wantDocs := []string{"id", "title", "path", "sha256", "mtime", "body", "tenant_id", "owner_explicit"}
	wantJobs := []string{"id", "tenant_id", "sha256_original", "original_filename", "mime_detected", "bytes", "raw_path", "status", "error_class", "created_at", "updated_at", "existing_job_id"}

	t.Run("memory", func(t *testing.T) {
		s := New()
		assertIngestRoundTrip(t, s)
	})

	t.Run("sqlite", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "ingest.db")
		s, err := OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		assertColumns(t, s, false, "vault_docs", wantDocs)
		assertColumns(t, s, false, "vault_ingest_jobs", wantJobs)
		assertIngestRoundTrip(t, s)
		if _, err := s.PutVaultDoc(VaultDoc{Title: "Minh An", Path: "sources/inbox/hop-dong.md", Body: "keep", SHA256: "aa", OwnerExplicit: true, TenantID: "minh-an"}); err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		again, err := OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		defer again.Close()
		assertColumns(t, again, false, "vault_docs", wantDocs)
		assertColumns(t, again, false, "vault_ingest_jobs", wantJobs)
		doc, err := again.FindVaultDocByPath("minh-an", "sources/inbox/hop-dong.md")
		if err != nil || doc.Body != "keep" {
			t.Fatalf("vault_docs row after reopen: %v %#v", err, doc)
		}
	})

	t.Run("postgres", func(t *testing.T) {
		base := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
		if base == "" {
			base = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
		}
		if base == "" {
			if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
				t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
			}
			t.Skip("GOSO_TEST_POSTGRES_DSN unset")
		}
		s := openIngestPostgres(t, base, true)
		// SPEC 154 adds generated body_tsv. The SPEC 170 columns above are unchanged.
		assertColumns(t, s, true, "vault_docs", append(append([]string{}, wantDocs...), "body_tsv"))
		assertColumns(t, s, true, "vault_ingest_jobs", wantJobs)
		assertIngestRoundTrip(t, s)
		if _, err := s.PutVaultDoc(VaultDoc{Title: "Minh An", Path: "sources/inbox/hop-dong.md", Body: "keep", SHA256: "aa", OwnerExplicit: true, TenantID: "minh-an"}); err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		again := openIngestPostgres(t, base, false)
		defer again.Close()
		assertColumns(t, again, true, "vault_docs", append(append([]string{}, wantDocs...), "body_tsv"))
		doc, err := again.FindVaultDocByPath("minh-an", "sources/inbox/hop-dong.md")
		if err != nil || doc.Body != "keep" {
			t.Fatalf("postgres vault_docs: %v %#v", err, doc)
		}
	})
}

func assertIngestRoundTrip(t *testing.T, s StoreIface) {
	t.Helper()
	when := time.Date(2026, 11, 12, 1, 2, 3, 0, time.UTC)
	job, err := s.PutIngestJob(IngestJob{
		TenantID:         "minh-an",
		SHA256:           "abc",
		OriginalFilename: "hop-dong.pdf",
		MIME:             "application/pdf",
		Bytes:            12,
		RawPath:          "raw/abc.pdf",
		Status:           IngestUploaded,
		CreatedAt:        when,
		UpdatedAt:        when,
	})
	if err != nil || job.ID == "" || job.Status != IngestUploaded {
		t.Fatalf("put %#v %v", job, err)
	}
	got, err := s.GetIngestJob(job.ID)
	if err != nil || got.SHA256 != "abc" || got.TenantID != "minh-an" || got.Bytes != 12 || got.RawPath != "raw/abc.pdf" {
		t.Fatalf("get %#v %v", got, err)
	}
	dup, err := s.PutIngestJob(IngestJob{
		TenantID:         "minh-an",
		SHA256:           "abc",
		OriginalFilename: "hop-dong.pdf",
		MIME:             "application/pdf",
		Bytes:            12,
		RawPath:          "raw/abc.pdf",
		Status:           IngestDuplicate,
		ExistingJobID:    job.ID,
	})
	if err != nil || dup.Status != IngestDuplicate || dup.ExistingJobID != job.ID {
		t.Fatalf("dup %#v %v", dup, err)
	}
	found, err := s.FindUploadedIngestJob("minh-an", "abc")
	if err != nil || found.ID != job.ID || found.Status != IngestUploaded {
		t.Fatalf("find %#v %v", found, err)
	}
	if _, err := s.FindUploadedIngestJob("minh-an-legal", "abc"); err != ErrNotFound {
		t.Fatalf("other tenant must not see the job: %v", err)
	}
}

func assertColumns(t *testing.T, s *SQLiteStore, pg bool, table string, want []string) {
	t.Helper()
	got := map[string]struct{}{}
	var rows *sql.Rows
	var err error
	if pg {
		rows, err = s.db.db.Query(`SELECT column_name FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=$1`, table)
	} else {
		rows, err = s.db.db.Query(`PRAGMA table_info(` + table + `)`)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		if pg {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			got[name] = struct{}{}
			continue
		}
		var cid int
		var name, typ string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		got[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("%s columns %#v", table, got)
	}
	for _, name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("%s missing %s in %#v", table, name, got)
		}
	}
}

func openIngestPostgres(t *testing.T, base string, reset bool) *SQLiteStore {
	t.Helper()
	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Fatal(err)
	}
	const schema = "goso_ingest_170"
	if reset {
		if _, err := admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
		})
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	st, err := OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st.SQLiteStore
}
