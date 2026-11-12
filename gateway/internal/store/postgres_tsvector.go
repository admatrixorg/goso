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
//	DROP INDEX IF EXISTS idx_memories_customer_created;
//
// Recall still reads body_tsv after that, so a note older than recallScanCap
// is still found; the read is a sequential scan until the index is recreated.
// Restoring the newest-500 scan means reverting this spec's query code.
// Do not ALTER or DROP memories.body, vault_docs.body, kg_entities.body, or
// any other column that existed before SPEC 154.
const postgresTextSearchConfig = "simple"

const (
	idxMemoriesBodyTSV         = "idx_memories_body_tsv"
	idxVaultBodyTSV            = "idx_vault_docs_body_tsv"
	idxKGBodyTSV               = "idx_kg_entities_body_tsv"
	idxMemoriesTenantCreated   = "idx_memories_tenant_created"
	idxMemoriesCustomerCreated = "idx_memories_customer_created"
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
		// Customer-scoped OR recall orders by created_at. idx_memories_customer
		// is (tenant_id, customer_key) only, so a large customer would be read
		// in full and sorted. This index stops after the LIMIT.
		`CREATE INDEX IF NOT EXISTS ` + idxMemoriesCustomerCreated + ` ON memories (tenant_id, customer_key, created_at DESC)`,
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

// recallORStableGUCs keeps the newest-window probe on an index scan.
// Sequential scans are what flipped on CI for the same indexes and the same rows.
var recallORStableGUCs = []string{
	`SET LOCAL enable_seqscan = off`,
	`SET LOCAL jit = off`,
	`SET LOCAL max_parallel_workers_per_gather = 0`,
}

// recallORBitmapGUCs forces BitmapAnd of the tenant btree and the GIN index.
// Index scans are off so the plan cannot walk idx_memories_tenant_created.
// Sequential scans are off so it cannot read the heap to filter body_tsv.
// cpu_tuple_cost makes a heap filter of every tenant row more expensive than
// intersecting the two bitmaps, which heap-fetches only the overlap.
// tenant_id stays an index condition. The GIN posting lists are still global;
// the heap fetch is not.
var recallORBitmapGUCs = []string{
	`SET LOCAL enable_seqscan = off`,
	`SET LOCAL enable_indexscan = off`,
	`SET LOCAL enable_indexonlyscan = off`,
	`SET LOCAL cpu_tuple_cost = 1`,
	`SET LOCAL jit = off`,
	`SET LOCAL max_parallel_workers_per_gather = 0`,
}

// recallORCustomerBoundGUCs keeps the large-customer window on
// idx_memories_customer_created. A bitmap of that index cannot stop after
// the newest rows, so bitmap scans are off.
var recallORCustomerBoundGUCs = []string{
	`SET LOCAL enable_seqscan = off`,
	`SET LOCAL enable_bitmapscan = off`,
	`SET LOCAL jit = off`,
	`SET LOCAL max_parallel_workers_per_gather = 0`,
}

// recallORAgentGUCs starts at the agent's sessions and nested-loops their
// memories. Hash and merge joins are off so the planner cannot scan every
// memory in the tenant and probe a tiny session hash.
var recallORAgentGUCs = []string{
	`SET LOCAL enable_seqscan = off`,
	`SET LOCAL enable_bitmapscan = off`,
	`SET LOCAL enable_hashjoin = off`,
	`SET LOCAL enable_mergejoin = off`,
	`SET LOCAL jit = off`,
	`SET LOCAL max_parallel_workers_per_gather = 0`,
}

const (
	recallPathTenantWindow  = "tenant-window"
	recallPathCustomerIndex = "customer-index"
	recallPathAgentNested   = "agent-nested-loop"
	recallPathBitmapAnd     = "bitmap-and"
)

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
	step2, n2, _, err := s.recallTSVORPath(q, tenant, agent, toks, orMatch, remain)
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

// recallTSVOR is step 2. A small customer or a small agent is read through
// its own index and returned whole, so a tenant full of other customers or
// agents is not materialized. Otherwise a newest-first window on
// idx_memories_tenant_created supplies the rows when it already holds lim
// hits. The remaining case is a BitmapAnd of the tenant btree and
// idx_memories_body_tsv, with the agent and customer filters inside the CTE.
// SQL does not call ts_rank.
func (s *SQLiteStore) recallTSVOR(q RecallQuery, tenant, agent string, toks []string, match string, lim int) ([]RecallHit, int, error) {
	hits, n, _, err := s.recallTSVORPath(q, tenant, agent, toks, match, lim)
	return hits, n, err
}

// recallTSVORPath is recallTSVOR plus the plan name (tenant-window,
// customer-index, agent-nested-loop, bitmap-and). The name is a return value
// so callers do not share mutable state.
func (s *SQLiteStore) recallTSVORPath(q RecallQuery, tenant, agent string, toks []string, match string, lim int) ([]RecallHit, int, string, error) {
	if match == "" || lim <= 0 {
		return nil, 0, "", nil
	}
	budget := orRecallProbeBudget(lim)
	// Customer scope is decided before the tenant window. A small customer is
	// the whole answer. A large one must not scan newer rows that belong to
	// other customers; the bitmap CTE already filters customer_key.
	if q.CustomerScoped {
		nCust, err := s.recallORCount(recallORCustomerCountSQL(), tenant, q.CustomerKey, KindEpisodic, KindDurable, budget)
		if err != nil {
			return nil, 0, "", err
		}
		if nCust < budget {
			query, args := recallTSVORCustomerQuery(q, tenant, agent, match, lim)
			hits, n, err := s.execRecallTSV(toks, query, args, recallORStableGUCs)
			if err != nil {
				return nil, n, "", err
			}
			return hits, n, recallPathCustomerIndex, nil
		}
		// Newest window of this customer. When it already holds lim hits, other
		// customers are never read. Otherwise the bitmap CTE still filters
		// customer_key, so the swamp is not materialized either.
		query, args := recallTSVORCustomerBoundQuery(q, tenant, agent, match, budget, lim)
		hits, n, err := s.execRecallTSV(toks, query, args, recallORCustomerBoundGUCs)
		if err != nil {
			return nil, n, "", err
		}
		if n >= lim {
			return hits, n, recallPathBitmapAnd, nil
		}
		bitQ, bitArgs := recallTSVORBitmapQuery(q, tenant, agent, match, lim)
		hits, n, err = s.execRecallTSV(toks, bitQ, bitArgs, recallORBitmapGUCs)
		if err != nil {
			return nil, n, "", err
		}
		return hits, n, recallPathBitmapAnd, nil
	}
	probeQ, probeArgs := recallTSVORProbeQuery(q, tenant, agent, match, budget, lim)
	hits, n, err := s.execRecallTSV(toks, probeQ, probeArgs, recallORStableGUCs)
	if err != nil {
		return nil, n, "", err
	}
	if n >= lim {
		return hits, n, recallPathTenantWindow, nil
	}
	if agent != "" {
		nAgent, err := s.recallORCountGUCs(recallORAgentGUCs, recallORAgentCountSQL(), agent, tenant, budget)
		if err != nil {
			return nil, n, "", err
		}
		if nAgent < budget {
			query, args := recallTSVORAgentQuery(q, tenant, agent, match, lim)
			hits, n, err = s.execRecallTSV(toks, query, args, recallORAgentGUCs)
			if err != nil {
				return nil, n, "", err
			}
			return hits, n, recallPathAgentNested, nil
		}
	}
	bitQ, bitArgs := recallTSVORBitmapQuery(q, tenant, agent, match, lim)
	hits, n, err = s.execRecallTSV(toks, bitQ, bitArgs, recallORBitmapGUCs)
	if err != nil {
		return nil, n, "", err
	}
	return hits, n, recallPathBitmapAnd, nil
}

func (s *SQLiteStore) recallORCount(query string, args ...any) (int, error) {
	var n int
	err := s.db.QueryRow(query, args...).Scan(&n)
	return n, err
}

func (s *SQLiteStore) recallORCountGUCs(gucs []string, query string, args ...any) (int, error) {
	if len(gucs) == 0 {
		return s.recallORCount(query, args...)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, g := range gucs {
		if _, err := tx.Exec(g); err != nil {
			return 0, err
		}
	}
	var n int
	if err := tx.QueryRow(query, args...).Scan(&n); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
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

// recallTSVORBitmapQuery loads the newest lim OR hits for one tenant.
// tenant_id = ? is an index condition. The agent filter is session_id IN
// that agent's sessions, inside the CTE, so other agents' matches are not
// materialized. customer_key is in the CTE when the recall is customer scoped.
// The statement runs under recallORBitmapGUCs: sequential scans and plain
// index scans are off, cpu_tuple_cost is 1, so the plan intersects the tenant
// btree with idx_memories_body_tsv instead of heap-fetching every match.
func recallTSVORBitmapQuery(q RecallQuery, tenant, agent, match string, lim int) (string, []any) {
	query := `WITH hits AS MATERIALIZED (
			SELECT id, session_id, kind, body, created_at, tenant_id, customer_key
			FROM memories
			WHERE tenant_id = ?
			AND body_tsv @@ to_tsquery('` + postgresTextSearchConfig + `', ?)
			AND kind IN (?, ?)
			AND session_id <> ?`
	args := []any{tenant, match, KindEpisodic, KindDurable, q.ExcludeSession}
	if agent != "" {
		query += ` AND session_id IN (SELECT id FROM sessions WHERE agent_id = ? AND tenant_id = ?)`
		args = append(args, agent, tenant)
	}
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

// recallTSVORCustomerBoundQuery reads at most lim newest rows of one
// customer through idx_memories_customer_created, then applies the OR and
// the agent filter. The inner LIMIT is the fence, so other customers are
// not read.
func recallTSVORCustomerBoundQuery(q RecallQuery, tenant, agent, match string, window, lim int) (string, []any) {
	query := `SELECT m.id, m.session_id, m.kind, m.body, m.created_at, m.tenant_id, m.customer_key
		FROM (
			SELECT id, session_id, kind, body, created_at, tenant_id, customer_key, body_tsv
			FROM memories
			WHERE tenant_id = ? AND customer_key = ?
			ORDER BY created_at DESC
			LIMIT ?
		) m
		JOIN sessions sess ON sess.id = m.session_id
		WHERE m.body_tsv @@ to_tsquery('` + postgresTextSearchConfig + `', ?)
		AND sess.tenant_id = ?
		AND m.kind IN (?, ?)
		AND m.session_id <> ?`
	args := []any{tenant, q.CustomerKey, window, match, tenant, KindEpisodic, KindDurable, q.ExcludeSession}
	if agent != "" {
		query += ` AND sess.agent_id = ?`
		args = append(args, agent)
	}
	query += ` ORDER BY m.created_at DESC LIMIT ?`
	args = append(args, lim)
	return query, args
}

func recallORCustomerCountSQL() string {
	return `SELECT count(*) FROM (
		SELECT 1 FROM memories
		WHERE tenant_id = ? AND customer_key = ? AND kind IN (?, ?)
		LIMIT ?
	) AS w`
}

func recallTSVORCustomerQuery(q RecallQuery, tenant, agent, match string, lim int) (string, []any) {
	query := `SELECT m.id, m.session_id, m.kind, m.body, m.created_at, m.tenant_id, m.customer_key
		FROM memories m
		JOIN sessions sess ON sess.id = m.session_id
		WHERE m.tenant_id = ? AND m.customer_key = ?
		AND sess.tenant_id = ?
		AND m.body_tsv @@ to_tsquery('` + postgresTextSearchConfig + `', ?)
		AND m.kind IN (?, ?)
		AND m.session_id <> ?`
	args := []any{tenant, q.CustomerKey, tenant, match, KindEpisodic, KindDurable, q.ExcludeSession}
	if agent != "" {
		query += ` AND sess.agent_id = ?`
		args = append(args, agent)
	}
	query += ` ORDER BY m.created_at DESC LIMIT ?`
	args = append(args, lim)
	return query, args
}

func recallORAgentCountSQL() string {
	// session_id IN, with no tenant predicate on memories, so the nested
	// loop uses idx_memories_session. A tenant predicate makes the planner
	// scan every row of the tenant instead.
	return `SELECT count(*) FROM (
		SELECT 1 FROM memories
		WHERE session_id IN (
			SELECT id FROM sessions WHERE agent_id = ? AND tenant_id = ?
		)
		LIMIT ?
	) AS w`
}

func recallTSVORAgentQuery(q RecallQuery, tenant, agent, match string, lim int) (string, []any) {
	query := `SELECT m.id, m.session_id, m.kind, m.body, m.created_at, m.tenant_id, m.customer_key
		FROM (
			SELECT id, session_id, kind, body, created_at, tenant_id, customer_key, body_tsv
			FROM memories
			WHERE session_id IN (
				SELECT id FROM sessions WHERE agent_id = ? AND tenant_id = ?
			)
			OFFSET 0
		) m
		WHERE m.tenant_id = ?
		AND m.body_tsv @@ to_tsquery('` + postgresTextSearchConfig + `', ?)
		AND m.kind IN (?, ?)
		AND m.session_id <> ?`
	args := []any{agent, tenant, tenant, match, KindEpisodic, KindDurable, q.ExcludeSession}
	if q.CustomerScoped {
		query += ` AND m.customer_key = ?`
		args = append(args, q.CustomerKey)
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
