// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"sort"
	"strings"
	"time"
)

// KGDedupCandidate is one suspected duplicate pair.
type KGDedupCandidate struct {
	ID         string  `json:"id"`
	TenantID   string  `json:"tenant_id"`
	EntityAID  string  `json:"entity_a_id"`
	EntityBID  string  `json:"entity_b_id"`
	NameA      string  `json:"name_a,omitempty"`
	NameB      string  `json:"name_b,omitempty"`
	Similarity float64 `json:"similarity"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"created_at"`
}

// KGMergeResult reports what a merge changed.
type KGMergeResult struct {
	KeptID           string `json:"kept_id"`
	MergedID         string `json:"merged_id"`
	RelationsMoved   int    `json:"relations_moved"`
	RelationsDropped int    `json:"relations_dropped"`
}

// KGDedupStore is the duplicate review capability.
type KGDedupStore interface {
	ScanKGDedupCandidates(tenant string) (int, error)
	ListKGDedupCandidates(tenant, status string) []*KGDedupCandidate
	MergeKGEntities(tenant, keepID, dropID string) (*KGMergeResult, error)
}

// KGDedup returns the duplicate review capability of st, or nil.
func KGDedup(st StoreIface) KGDedupStore {
	if x, ok := st.(KGDedupStore); ok {
		return x
	}
	return nil
}

func (s *SQLiteStore) migrateKGDedup() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS kg_dedup_candidates (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			entity_a_id TEXT NOT NULL,
			entity_b_id TEXT NOT NULL,
			similarity DOUBLE PRECISION NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_kg_dedup_pair ON kg_dedup_candidates(tenant_id, entity_a_id, entity_b_id)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// normalizeKGName folds a name for comparison: lowercase, trimmed, and
// without the punctuation that variants of the same entity tend to add.
func normalizeKGName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if r == ' ' || r == '-' || r == '_' || r == '.' || r == ',' || r == '(' || r == ')' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ScanKGDedupCandidates pairs entities whose folded names match and records a
// pending candidate for each new pair. It returns how many it added.
func (s *SQLiteStore) ScanKGDedupCandidates(tenant string) (int, error) {
	tenant = NormalizeTenant(tenant)
	rows, err := s.db.Query(`SELECT id, name FROM kg_entities WHERE tenant_id=? AND valid_until IS NULL ORDER BY name`, tenant)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	groups := map[string][]string{}
	for rows.Next() {
		var id, name string
		if rows.Scan(&id, &name) != nil {
			continue
		}
		key := normalizeKGName(name)
		if key == "" {
			continue
		}
		groups[key] = append(groups[key], id)
	}
	added := 0
	now := formatTime(time.Now().UTC())
	for _, ids := range groups {
		if len(ids) < 2 {
			continue
		}
		sort.Strings(ids)
		for i := 0; i < len(ids); i++ {
			for k := i + 1; k < len(ids); k++ {
				res, ierr := s.db.Exec(`INSERT INTO kg_dedup_candidates(id, tenant_id, entity_a_id, entity_b_id, similarity, status, created_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`,
					"kgd_"+newID(), tenant, ids[i], ids[k], 1.0, "pending", now)
				if ierr != nil {
					continue
				}
				if n, _ := res.RowsAffected(); n > 0 {
					added++
				}
			}
		}
	}
	return added, nil
}

func (s *SQLiteStore) ListKGDedupCandidates(tenant, status string) []*KGDedupCandidate {
	q := `SELECT c.id, c.tenant_id, c.entity_a_id, c.entity_b_id, coalesce(a.name,''), coalesce(b.name,''), c.similarity, c.status, c.created_at FROM kg_dedup_candidates c LEFT JOIN kg_entities a ON a.id=c.entity_a_id LEFT JOIN kg_entities b ON b.id=c.entity_b_id WHERE c.tenant_id=?`
	args := []any{NormalizeTenant(tenant)}
	if strings.TrimSpace(status) != "" {
		q += " AND c.status=?"
		args = append(args, strings.TrimSpace(status))
	}
	q += " ORDER BY c.similarity DESC, c.created_at LIMIT 500"
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return []*KGDedupCandidate{}
	}
	defer rows.Close()
	out := []*KGDedupCandidate{}
	for rows.Next() {
		var c KGDedupCandidate
		if rows.Scan(&c.ID, &c.TenantID, &c.EntityAID, &c.EntityBID, &c.NameA, &c.NameB, &c.Similarity, &c.Status, &c.CreatedAt) != nil {
			continue
		}
		out = append(out, &c)
	}
	return out
}

// MergeKGEntities keeps one entity, moves every edge onto it, drops edges that
// would become self loops, and retires the other entity.
func (s *SQLiteStore) MergeKGEntities(tenant, keepID, dropID string) (*KGMergeResult, error) {
	tenant = NormalizeTenant(tenant)
	keepID = strings.TrimSpace(keepID)
	dropID = strings.TrimSpace(dropID)
	if keepID == "" || dropID == "" || keepID == dropID {
		return nil, ErrNotFound
	}
	res := &KGMergeResult{KeptID: keepID, MergedID: dropID}
	now := formatTime(time.Now().UTC())
	for _, stmt := range []string{
		`UPDATE kg_relations SET from_id=? WHERE tenant_id=? AND from_id=?`,
		`UPDATE kg_relations SET to_id=? WHERE tenant_id=? AND to_id=?`,
	} {
		r, err := s.db.Exec(stmt, keepID, tenant, dropID)
		if err != nil {
			return nil, err
		}
		if n, _ := r.RowsAffected(); n > 0 {
			res.RelationsMoved += int(n)
		}
	}
	if r, err := s.db.Exec(`DELETE FROM kg_relations WHERE from_id=to_id`); err == nil {
		if n, _ := r.RowsAffected(); n > 0 {
			res.RelationsDropped = int(n)
		}
	}
	if _, err := s.db.Exec(`DELETE FROM kg_entities WHERE tenant_id=? AND id=?`, tenant, dropID); err != nil {
		return nil, err
	}
	_, _ = s.db.Exec(`UPDATE kg_dedup_candidates SET status='merged' WHERE tenant_id=? AND ((entity_a_id=? AND entity_b_id=?) OR (entity_a_id=? AND entity_b_id=?))`,
		tenant, keepID, dropID, dropID, keepID)
	// Mark the rest of the retired entity's pairs stale, but leave the pair that
	// was just merged reported as merged.
	_, _ = s.db.Exec(`UPDATE kg_dedup_candidates SET status='stale' WHERE tenant_id=?
		AND (entity_a_id=? OR entity_b_id=?)
		AND NOT ((entity_a_id=? AND entity_b_id=?) OR (entity_a_id=? AND entity_b_id=?))`,
		tenant, dropID, dropID, keepID, dropID, dropID, keepID)
	_ = now
	return res, nil
}
