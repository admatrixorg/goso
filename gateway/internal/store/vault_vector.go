// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"log"
	"strconv"
	"strings"
	"time"
)

// VaultChunk is one embedded piece of a vault doc body.
type VaultChunk struct {
	Index     int
	Body      string
	Embedding []float32
}

// VaultVectorHit is one chunk ranked by cosine distance (smaller is closer).
type VaultVectorHit struct {
	DocID      string
	ChunkIndex int
	Body       string
	Distance   float64
}

// VaultBacklogDoc is a served doc whose chunks are missing or stale.
type VaultBacklogDoc struct {
	ID       string
	TenantID string
}

// VaultVectorIndex is the optional chunk-embedding store. Only Postgres with
// pgvector implements it as ready; SQLite stays keyword-only.
type VaultVectorIndex interface {
	VaultVectorReady() bool
	// ReplaceVaultChunks swaps every chunk of docID for chunks of body sha.
	ReplaceVaultChunks(tenant, docID, sha, model string, dim int, chunks []VaultChunk) error
	// SearchVaultVector ranks chunks of current bodies visible to v.
	SearchVaultVector(tenant string, v VaultViewer, model string, vec []float32, limit int) ([]VaultVectorHit, error)
	// VaultEmbedBacklog lists served docs after cursor id whose chunks do not
	// match the current sha, model and dim. tenant "" means every tenant.
	VaultEmbedBacklog(tenant, model string, dim int, after string, limit int) ([]VaultBacklogDoc, error)
	// VaultEmbedCounts returns served docs and docs with current chunks.
	VaultEmbedCounts(tenant, model string, dim int) (docs, embedded int, err error)
}

var errVaultVectorOff = errors.New("vault vector index not available")

// migrateVaultVectors creates vault_chunks when pgvector is present. A
// failure is logged and leaves search keyword-only; startup continues.
func (s *PostgresStore) migrateVaultVectors() {
	if !s.vector {
		return
	}
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS vault_chunks (
			doc_id TEXT NOT NULL,
			chunk_index INTEGER NOT NULL,
			tenant_id TEXT NOT NULL,
			doc_sha256 TEXT NOT NULL,
			model TEXT NOT NULL,
			dim INTEGER NOT NULL,
			body TEXT NOT NULL,
			embedding ` + s.vecType() + ` NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY (doc_id, chunk_index)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_vault_chunks_tenant ON vault_chunks(tenant_id, model, dim)`,
	} {
		if _, err := s.db.Exec(ddl); err != nil {
			log.Printf("goso store: vault_chunks skipped: %v", err)
			return
		}
	}
	s.vaultChunks = true
}

// vecType is the pgvector type, qualified with the extension's schema.
func (s *SQLiteStore) vecType() string {
	if s.vecSchema == "" {
		return "vector"
	}
	return s.vecSchema + ".vector"
}

// VaultVectorReady reports that vault_chunks exists on pgvector.
func (s *SQLiteStore) VaultVectorReady() bool { return s.pg && s.vector && s.vaultChunks }

func (s *SQLiteStore) deleteVaultChunks(docID string) {
	if s.VaultVectorReady() {
		_, _ = s.db.Exec(`DELETE FROM vault_chunks WHERE doc_id=?`, docID)
	}
}

func vectorLiteral(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 10)
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func (s *SQLiteStore) ReplaceVaultChunks(tenant, docID, sha, model string, dim int, chunks []VaultChunk) error {
	if !s.VaultVectorReady() {
		return errVaultVectorOff
	}
	tenant = NormalizeTenant(tenant)
	owner, err := s.vaultTenant(docID)
	if err != nil {
		return err
	}
	if !SameTenant(owner, tenant) {
		return ErrNotFound
	}
	for _, c := range chunks {
		if len(c.Embedding) != dim {
			return errors.New("chunk vector size does not match dim")
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM vault_chunks WHERE doc_id=?`, docID); err != nil {
		return err
	}
	now := formatTime(time.Now().UTC())
	for _, c := range chunks {
		if _, err := tx.Exec(`INSERT INTO vault_chunks(doc_id, chunk_index, tenant_id, doc_sha256, model, dim, body, embedding, created_at)
			VALUES(?,?,?,?,?,?,?,?::`+s.vecType()+`,?)`,
			docID, c.Index, tenant, sha, model, dim, c.Body, vectorLiteral(c.Embedding), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) SearchVaultVector(tenant string, v VaultViewer, model string, vec []float32, limit int) ([]VaultVectorHit, error) {
	if !s.VaultVectorReady() {
		return nil, errVaultVectorOff
	}
	if len(vec) == 0 {
		return []VaultVectorHit{}, nil
	}
	if limit <= 0 || limit > VaultSearchLimit {
		limit = VaultSearchLimit
	}
	tenant = NormalizeTenant(tenant)
	where, extra := vaultViewerSQL("d.", v)
	args := []any{vectorLiteral(vec), tenant, tenant, model, len(vec)}
	args = append(args, extra...)
	args = append(args, limit)
	rows, err := s.db.Query(`SELECT c.doc_id, c.chunk_index, c.body, c.embedding OPERATOR(`+s.vecSchema+`.<=>) ?::`+s.vecType()+` AS dist
		FROM vault_chunks c
		JOIN vault_docs d ON d.id = c.doc_id
		WHERE c.tenant_id = ? AND d.tenant_id = ? AND d.owner_explicit<>0
			AND c.doc_sha256 = d.sha256 AND c.model = ? AND c.dim = ?`+where+`
		ORDER BY dist
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VaultVectorHit{}
	for rows.Next() {
		var h VaultVectorHit
		if err := rows.Scan(&h.DocID, &h.ChunkIndex, &h.Body, &h.Distance); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) VaultEmbedBacklog(tenant, model string, dim int, after string, limit int) ([]VaultBacklogDoc, error) {
	if !s.VaultVectorReady() {
		return nil, errVaultVectorOff
	}
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT d.id, d.tenant_id FROM vault_docs d
		WHERE d.owner_explicit<>0 AND d.id > ?`
	args := []any{after}
	if strings.TrimSpace(tenant) != "" {
		q += ` AND d.tenant_id = ?`
		args = append(args, NormalizeTenant(tenant))
	}
	q += ` AND NOT EXISTS (SELECT 1 FROM vault_chunks c
			WHERE c.doc_id = d.id AND c.doc_sha256 = d.sha256 AND c.model = ? AND c.dim = ?)
		ORDER BY d.id LIMIT ?`
	args = append(args, model, dim, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VaultBacklogDoc
	for rows.Next() {
		var d VaultBacklogDoc
		if err := rows.Scan(&d.ID, &d.TenantID); err != nil {
			return nil, err
		}
		d.TenantID = NormalizeTenant(d.TenantID)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) VaultEmbedCounts(tenant, model string, dim int) (int, int, error) {
	if !s.VaultVectorReady() {
		return 0, 0, errVaultVectorOff
	}
	filter, args := ``, []any{}
	if strings.TrimSpace(tenant) != "" {
		filter = ` AND d.tenant_id = ?`
		args = append(args, NormalizeTenant(tenant))
	}
	var docs, embedded int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM vault_docs d WHERE d.owner_explicit<>0`+filter, args...).Scan(&docs); err != nil {
		return 0, 0, err
	}
	args2 := append([]any{model, dim}, args...)
	if err := s.db.QueryRow(`SELECT COUNT(DISTINCT d.id) FROM vault_docs d
		JOIN vault_chunks c ON c.doc_id = d.id AND c.doc_sha256 = d.sha256 AND c.model = ? AND c.dim = ?
		WHERE d.owner_explicit<>0`+filter, args2...).Scan(&embedded); err != nil {
		return 0, 0, err
	}
	return docs, embedded, nil
}
