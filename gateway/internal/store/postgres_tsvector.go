// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"fmt"
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
//	DROP INDEX IF EXISTS idx_memories_tenant_created;
//
// Recall still reads body_tsv after that, so a note older than recallScanCap
// is still found; the read is a sequential scan until the index is recreated.
// Restoring the newest-500 scan means reverting this spec's query code.
// Do not ALTER or DROP memories.body, vault_docs.body, kg_entities.body, or
// any other column that existed before SPEC 154.
const postgresTextSearchConfig = "simple"

const (
	idxMemoriesBodyTSV       = "idx_memories_body_tsv"
	idxVaultBodyTSV          = "idx_vault_docs_body_tsv"
	idxKGBodyTSV             = "idx_kg_entities_body_tsv"
	idxMemoriesTenantCreated = "idx_memories_tenant_created"
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
		// Scope always filters tenant_id and orders by created_at. No existing
		// index covers that pair (idx_memories_tenant is tenant_id only).
		`CREATE INDEX IF NOT EXISTS ` + idxMemoriesTenantCreated + ` ON memories (tenant_id, created_at DESC)`,
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
// digits from RecallTokens, kept only when safeTSToken accepts them.
func tsOrQuery(toks []string) string {
	return tsJoinQuery(toks, " | ")
}

// tsAndQuery builds a simple-config to_tsquery AND. Same sanitizing as tsOrQuery.
func tsAndQuery(toks []string) string {
	return tsJoinQuery(toks, " & ")
}

func tsJoinQuery(toks []string, op string) string {
	parts := make([]string, 0, len(toks))
	for _, t := range toks {
		if t == "" || !safeTSToken(t) {
			continue
		}
		parts = append(parts, "'"+t+"'")
	}
	return strings.Join(parts, op)
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

// recallORStableGUCs is applied to both step-2 statements. Sequential scans
// are what flipped on CI for the same indexes and the same rows. The probe
// still index-scans idx_memories_tenant_created because its subquery is
// ORDER BY created_at DESC LIMIT. The bitmap statement's only indexable
// predicate is body_tsv @@, so the plan that remains is the GIN bitmap.
var recallORStableGUCs = []string{`SET LOCAL enable_seqscan = off`}

// recallTSV is the Postgres candidate set. It does not apply recallScanCap
// and it does not rank in SQL. Step 1 is the AND of every safe token, newest
// first, at most recallCandidateCap rows. Step 2 runs only when that returned
// fewer than limit rows: the OR of the same tokens, newest first, only for
// the unused part of the cap, skipping ids step 1 already returned. Go scores
// the union with lexicalScore.
//
// Step 2 does not ask the planner to pick between a sequential scan and an
// index walk. It reads a bounded newest-first window through
// idx_memories_tenant_created, and uses that window only when it already
// contains lim matches. Otherwise it runs a GIN bitmap of the OR. A walk of
// the whole tenant is the slow plan when almost nothing matches.
func (s *SQLiteStore) recallTSV(q RecallQuery, tenant, agent string, toks []string, limit int) ([]RecallHit, error) {
	hits, _, err := s.recallTSVFetch(q, tenant, agent, toks, limit)
	return hits, err
}

func (s *SQLiteStore) recallTSVFetch(q RecallQuery, tenant, agent string, toks []string, limit int) ([]RecallHit, int, error) {
	andMatch := tsAndQuery(toks)
	orMatch := tsOrQuery(toks)
	if orMatch == "" {
		return []RecallHit{}, 0, nil
	}
	if limit <= 0 {
		limit = recallDefaultLimit
	}
	step1, n1, err := s.recallTSVStep(q, tenant, agent, toks, andMatch, recallCandidateCap)
	if err != nil {
		return nil, n1, err
	}
	hits := step1
	fetched := n1
	if len(hits) >= limit || fetched >= recallCandidateCap {
		if fetched > recallCandidateCap {
			return nil, fetched, fmt.Errorf("recall fetched %d rows, cap is %d", fetched, recallCandidateCap)
		}
		return sortRecall(hits, limit), fetched, nil
	}
	seen := make(map[string]struct{}, len(hits))
	for _, h := range hits {
		seen[h.Memory.ID] = struct{}{}
	}
	remain := recallCandidateCap - fetched
	step2, n2, err := s.recallTSVOR(q, tenant, agent, toks, orMatch, remain)
	fetched += n2
	if err != nil {
		return nil, fetched, err
	}
	for _, h := range step2 {
		if _, ok := seen[h.Memory.ID]; ok {
			continue
		}
		hits = append(hits, h)
	}
	if fetched > recallCandidateCap {
		return nil, fetched, fmt.Errorf("recall fetched %d rows, cap is %d", fetched, recallCandidateCap)
	}
	return sortRecall(hits, limit), fetched, nil
}

func (s *SQLiteStore) recallTSVStep(q RecallQuery, tenant, agent string, toks []string, match string, lim int) ([]RecallHit, int, error) {
	if match == "" || lim <= 0 {
		return nil, 0, nil
	}
	query, args := recallTSVStepQuery(q, tenant, agent, match, lim)
	return s.execRecallTSV(toks, query, args, nil)
}

// orRecallProbeBudget is the most rows step 2 reads from
// idx_memories_tenant_created. It is not the candidate cap. Twice lim covers
// rows dropped by kind, session, or agent filters when nearly every body
// matches the OR. A larger budget would scan a sparse tenant end to end.
func orRecallProbeBudget(lim int) int {
	if lim <= 0 {
		return 0
	}
	b := lim * 2
	if b/2 != lim {
		return lim
	}
	return b
}

// recallTSVOR is step 2. The probe is an ordered index read with a hard row
// budget. When that window holds lim hits, they are the newest lim hits,
// because every row outside the window is older. When it does not, the probe
// rows are discarded and the GIN bitmap returns the newest lim hits. Either
// way the candidate count stays within lim, and SQL does not call ts_rank.
func (s *SQLiteStore) recallTSVOR(q RecallQuery, tenant, agent string, toks []string, match string, lim int) ([]RecallHit, int, error) {
	if match == "" || lim <= 0 {
		return nil, 0, nil
	}
	budget := orRecallProbeBudget(lim)
	probeQ, probeArgs := recallTSVORProbeQuery(q, tenant, agent, match, budget, lim)
	hits, n, err := s.execRecallTSV(toks, probeQ, probeArgs, recallORStableGUCs)
	if err != nil {
		return nil, n, err
	}
	if n >= lim {
		return hits, n, nil
	}
	bitQ, bitArgs := recallTSVORBitmapQuery(q, tenant, agent, match, lim)
	return s.execRecallTSV(toks, bitQ, bitArgs, recallORStableGUCs)
}

func (s *SQLiteStore) execRecallTSV(toks []string, query string, args []any, gucs []string) ([]RecallHit, int, error) {
	if len(gucs) == 0 {
		rows, err := s.db.Query(query, args...)
		if err != nil {
			return nil, 0, err
		}
		defer rows.Close()
		return scanRecallTSV(rows, toks)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	for _, g := range gucs {
		if _, err := tx.Exec(g); err != nil {
			return nil, 0, err
		}
	}
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	hits, n, scanErr := scanRecallTSV(rows, toks)
	closeErr := rows.Close()
	if scanErr != nil {
		return nil, n, scanErr
	}
	if closeErr != nil {
		return nil, n, closeErr
	}
	if err := tx.Commit(); err != nil {
		return nil, n, err
	}
	return hits, n, nil
}

func scanRecallTSV(rows *sql.Rows, toks []string) ([]RecallHit, int, error) {
	var hits []RecallHit
	fetched := 0
	for rows.Next() {
		fetched++
		m, err := scanMemory(rows)
		if err != nil || m == nil {
			continue
		}
		if sc := lexicalScore(m.Body, toks); sc > 0 {
			hits = append(hits, RecallHit{Memory: *m, Score: sc})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fetched, err
	}
	return hits, fetched, nil
}

// recallTSVStepQuery is the SQL for one AND or OR pass. match is a bound
// to_tsquery string, not interpolated SQL.
func recallTSVStepQuery(q RecallQuery, tenant, agent, match string, lim int) (string, []any) {
	query := `SELECT m.id, m.session_id, m.kind, m.body, m.created_at, m.tenant_id, m.customer_key
		FROM memories m
		JOIN sessions sess ON sess.id = m.session_id
		WHERE m.body_tsv @@ to_tsquery('` + postgresTextSearchConfig + `', ?)
		AND m.tenant_id = ? AND sess.tenant_id = ?
		AND m.kind IN (?, ?)
		AND m.session_id <> ?`
	args := []any{match, tenant, tenant, KindEpisodic, KindDurable, q.ExcludeSession}
	query, args = recallScope(query, args, q, agent)
	query += ` ORDER BY m.created_at DESC LIMIT ?`
	args = append(args, lim)
	return query, args
}

// recallTSVORProbeQuery reads at most budget newest rows for the tenant,
// then applies the OR and the rest of the scope. The inner LIMIT is a fence:
// the tsquery is not pushed into the index scan, so the plan does not follow
// the planner's row estimate for the OR.
func recallTSVORProbeQuery(q RecallQuery, tenant, agent, match string, budget, lim int) (string, []any) {
	query := `SELECT m.id, m.session_id, m.kind, m.body, m.created_at, m.tenant_id, m.customer_key
		FROM (
			SELECT id, session_id, kind, body, created_at, tenant_id, customer_key, body_tsv
			FROM memories
			WHERE tenant_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		) m
		JOIN sessions sess ON sess.id = m.session_id
		WHERE m.body_tsv @@ to_tsquery('` + postgresTextSearchConfig + `', ?)
		AND sess.tenant_id = ?
		AND m.kind IN (?, ?)
		AND m.session_id <> ?`
	args := []any{tenant, budget, match, tenant, KindEpisodic, KindDurable, q.ExcludeSession}
	query, args = recallScope(query, args, q, agent)
	query += ` ORDER BY m.created_at DESC LIMIT ?`
	args = append(args, lim)
	return query, args
}

// recallTSVORBitmapQuery loads the newest lim OR hits through the GIN index.
// tenant_id || ” is a filter, not an index condition, so idx_memories_tenant
// and idx_memories_tenant_created cannot answer the statement. With sequential
// scans disabled, the remaining plan is a bitmap index scan of
// idx_memories_body_tsv.
func recallTSVORBitmapQuery(q RecallQuery, tenant, agent, match string, lim int) (string, []any) {
	query := `WITH hits AS MATERIALIZED (
			SELECT id, session_id, kind, body, created_at, tenant_id, customer_key
			FROM memories
			WHERE body_tsv @@ to_tsquery('` + postgresTextSearchConfig + `', ?)
			AND tenant_id || '' = ?
			AND kind IN (?, ?)
			AND session_id <> ?`
	args := []any{match, tenant, KindEpisodic, KindDurable, q.ExcludeSession}
	if q.CustomerScoped {
		query += ` AND customer_key = ?`
		args = append(args, q.CustomerKey)
	}
	query += `)
		SELECT m.id, m.session_id, m.kind, m.body, m.created_at, m.tenant_id, m.customer_key
		FROM hits m
		JOIN sessions sess ON sess.id = m.session_id
		WHERE sess.tenant_id = ?`
	args = append(args, tenant)
	if agent != "" {
		query += ` AND sess.agent_id = ?`
		args = append(args, agent)
	}
	query += ` ORDER BY m.created_at DESC LIMIT ?`
	args = append(args, lim)
	return query, args
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
