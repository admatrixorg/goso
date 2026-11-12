// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"strings"
	"unicode"
)

// SPEC 154. Text search config is simple: no stemming and no unaccent
// (diacritics are SPEC 155). These statements only add columns and indexes.
//
// Rollback drops the new indexes and leaves every pre-existing column alone:
//
//	DROP INDEX IF EXISTS idx_memories_body_tsv;
//	DROP INDEX IF EXISTS idx_vault_docs_body_tsv;
//	DROP INDEX IF EXISTS idx_kg_entities_body_tsv;
//
// Recall still reads body_tsv after that, so a note older than recallScanCap
// is still found; the read is a sequential scan until the index is recreated.
// Restoring the newest-500 scan means reverting this spec's query code.
// Do not ALTER or DROP memories.body, vault_docs.body, kg_entities.body, or
// any other column that existed before SPEC 154.
const postgresTextSearchConfig = "simple"

const (
	idxMemoriesBodyTSV = "idx_memories_body_tsv"
	idxVaultBodyTSV    = "idx_vault_docs_body_tsv"
	idxKGBodyTSV       = "idx_kg_entities_body_tsv"
)

func (s *PostgresStore) migrateBodyTSV() error {
	expr := `to_tsvector('` + postgresTextSearchConfig + `', coalesce(body, ''))`
	stmts := []string{
		`ALTER TABLE memories ADD COLUMN IF NOT EXISTS body_tsv tsvector GENERATED ALWAYS AS (` + expr + `) STORED`,
		`CREATE INDEX IF NOT EXISTS ` + idxMemoriesBodyTSV + ` ON memories USING GIN (body_tsv)`,
		`ALTER TABLE vault_docs ADD COLUMN IF NOT EXISTS body_tsv tsvector GENERATED ALWAYS AS (` + expr + `) STORED`,
		`CREATE INDEX IF NOT EXISTS ` + idxVaultBodyTSV + ` ON vault_docs USING GIN (body_tsv)`,
		`ALTER TABLE kg_entities ADD COLUMN IF NOT EXISTS body_tsv tsvector GENERATED ALWAYS AS (` + expr + `) STORED`,
		`CREATE INDEX IF NOT EXISTS ` + idxKGBodyTSV + ` ON kg_entities USING GIN (body_tsv)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.db.Exec(stmt); err != nil {
			return err
		}
	}
	s.bodyTSV = true
	return nil
}

// tsOrQuery builds a simple-config to_tsquery OR. Tokens are letters and
// digits from RecallTokens, so the string is not raw user text.
func tsOrQuery(toks []string) string {
	parts := make([]string, 0, len(toks))
	for _, t := range toks {
		if t == "" || !safeTSToken(t) {
			continue
		}
		parts = append(parts, "'"+t+"'")
	}
	return strings.Join(parts, " | ")
}

func safeTSToken(t string) bool {
	for _, r := range t {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func tsPhraseSQL(column string) string {
	return column + " @@ phraseto_tsquery('" + postgresTextSearchConfig + "', ?)"
}

// recallTSV is the Postgres candidate set. It does not apply recallScanCap:
// a matching note older than the newest 500 rows is still returned. Scores
// stay token-overlap so admission matches the previous Postgres path.
func (s *SQLiteStore) recallTSV(q RecallQuery, tenant, agent string, toks []string, limit int) ([]RecallHit, error) {
	match := tsOrQuery(toks)
	if match == "" {
		return []RecallHit{}, nil
	}
	query := `SELECT m.id, m.session_id, m.kind, m.body, m.created_at, m.tenant_id, m.customer_key
		FROM memories m
		JOIN sessions sess ON sess.id = m.session_id
		WHERE m.body_tsv @@ to_tsquery('` + postgresTextSearchConfig + `', ?)
		AND m.tenant_id = ? AND sess.tenant_id = ?
		AND m.kind IN (?, ?)
		AND m.session_id <> ?`
	args := []any{match, tenant, tenant, KindEpisodic, KindDurable, q.ExcludeSession}
	query, args = recallScope(query, args, q, agent)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []RecallHit
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil || m == nil {
			continue
		}
		if sc := lexicalScore(m.Body, toks); sc > 0 {
			hits = append(hits, RecallHit{Memory: *m, Score: sc})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sortRecall(hits, limit), nil
}

func (s *SQLiteStore) searchMemoryTSV(q string) ([]SearchHit, error) {
	out := []SearchHit{}
	rows, err := s.db.Query(`SELECT id, session_id, kind, body FROM memories
		WHERE `+tsPhraseSQL("body_tsv")+`
		LIMIT 50`, q)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, sid, kind, body string
		if err := rows.Scan(&id, &sid, &kind, &body); err != nil {
			continue
		}
		out = append(out, SearchHit{ID: id, SessionID: sid, Kind: kind, Snippet: SnippetAround(body, q, 80)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	remain := 50 - len(out)
	if remain <= 0 {
		return out, nil
	}
	rows, err = s.db.Query(`SELECT id, session_id, content FROM messages
		WHERE instr(lower(content), lower(?)) > 0
		LIMIT ?`, q, remain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, sid, body string
		if err := rows.Scan(&id, &sid, &body); err != nil {
			continue
		}
		out = append(out, SearchHit{ID: id, SessionID: sid, Kind: KindMessage, Snippet: SnippetAround(body, q, 80)})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) searchVaultTSV(tenant, q string) ([]VaultSearchHit, error) {
	rows, err := s.db.Query(`SELECT id, title, path, body, sha256 FROM (
			SELECT id, title, path, body, sha256 FROM vault_docs
			WHERE tenant_id = ? AND owner_explicit<>0 AND `+tsPhraseSQL("body_tsv")+`
			UNION
			SELECT id, title, path, body, sha256 FROM vault_docs
			WHERE tenant_id = ? AND owner_explicit<>0 AND instr(lower(title), lower(?)) > 0
		) AS hits
		ORDER BY lower(title)
		LIMIT ?`, tenant, q, tenant, q, VaultSearchLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VaultSearchHit
	for rows.Next() {
		var id, title, path, body, sum string
		if err := rows.Scan(&id, &title, &path, &body, &sum); err != nil {
			continue
		}
		hay := body
		if !strings.Contains(strings.ToLower(body), strings.ToLower(q)) {
			hay = title
		}
		out = append(out, VaultSearchHit{ID: id, Title: title, Path: path, Snippet: SnippetAround(hay, q, 80), SHA256: sum})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []VaultSearchHit{}
	}
	return out, nil
}

func (s *SQLiteStore) searchProgressiveL1TSV(q, tenant string) ([]KGSearchHit, error) {
	out := []KGSearchHit{}
	rows, err := s.db.Query(`SELECT id, session_id, kind, body, tenant_id FROM memories
		WHERE tenant_id=? AND `+tsPhraseSQL("body_tsv")+`
		LIMIT ?`, tenant, q, progressivePerTier)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var h KGSearchHit
		var body string
		if err := rows.Scan(&h.ID, &h.SessionID, &h.Kind, &body, &h.TenantID); err != nil {
			continue
		}
		h.TenantID = NormalizeTenant(h.TenantID)
		h.Snippet = SnippetAround(body, q, 80)
		h.Tier = TierL1
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	remain := progressivePerTier - len(out)
	if remain <= 0 {
		return out, nil
	}
	rows, err = s.db.Query(`SELECT m.id, m.session_id, m.content, s.tenant_id FROM messages m
		JOIN sessions s ON s.id = m.session_id
		WHERE s.tenant_id=? AND instr(lower(m.content), lower(?)) > 0
		LIMIT ?`, tenant, q, remain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var h KGSearchHit
		var body string
		if err := rows.Scan(&h.ID, &h.SessionID, &body, &h.TenantID); err != nil {
			continue
		}
		h.Kind = KindMessage
		h.TenantID = NormalizeTenant(h.TenantID)
		h.Snippet = SnippetAround(body, q, 80)
		h.Tier = TierL1
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) searchProgressiveL2TSV(q, tenant string) ([]KGSearchHit, error) {
	rows, err := s.db.Query(`SELECT `+kgEntityCols+` FROM (
			SELECT `+kgEntityCols+` FROM kg_entities
			WHERE tenant_id=? AND `+tsPhraseSQL("body_tsv")+`
			UNION
			SELECT `+kgEntityCols+` FROM kg_entities
			WHERE tenant_id=? AND instr(lower(name), lower(?)) > 0
		) AS hits LIMIT ?`, tenant, q, tenant, q, progressivePerTier)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KGSearchHit
	for rows.Next() {
		e, err := scanKGEntity(rows)
		if err != nil || e == nil || !kgCurrent(e.ValidFrom, e.ValidUntil) {
			continue
		}
		out = append(out, KGSearchHit{
			ID: e.ID, TenantID: e.TenantID, Kind: e.Kind, Name: e.Name,
			Snippet: SnippetAround(e.Name+" "+e.Body, q, 80), Tier: TierL2,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []KGSearchHit{}
	}
	return out, nil
}
