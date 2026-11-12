// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agentimport

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/mqglobal/goso/gateway/internal/store"
)

const mapTable = "goclaw_agent_import_map"

const (
	kindAgent  = "agent"
	kindTeam   = "team"
	kindMember = "team_member"
	kindLink   = "agent_link"
)

type mapRow struct {
	DestTenant  string
	SourceTable string
	SourceKey   string
	Kind        string
	Ref         string
}

type mapper interface {
	get(dest, table, key string) (mapRow, bool, error)
	put(row mapRow) error
	list(dest string) ([]mapRow, error)
	del(dest, table, key string) error
}

type nopMapper struct{}

func (nopMapper) get(string, string, string) (mapRow, bool, error) {
	return mapRow{}, false, nil
}
func (nopMapper) put(mapRow) error { return fmt.Errorf("agentimport: map is closed") }
func (nopMapper) list(string) ([]mapRow, error) {
	return nil, nil
}
func (nopMapper) del(string, string, string) error { return fmt.Errorf("agentimport: map is closed") }

type sqlMapper struct {
	db *sql.DB
	pg bool
}

func (m *sqlMapper) q(query string) string {
	if m == nil || !m.pg {
		return query
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}

func (m *sqlMapper) get(dest, table, key string) (mapRow, bool, error) {
	var row mapRow
	err := m.db.QueryRow(m.q(`SELECT dest_tenant, source_table, source_key, goso_kind, goso_ref FROM `+mapTable+` WHERE dest_tenant=? AND source_table=? AND source_key=?`), dest, table, key).
		Scan(&row.DestTenant, &row.SourceTable, &row.SourceKey, &row.Kind, &row.Ref)
	if err == sql.ErrNoRows {
		return mapRow{}, false, nil
	}
	if err != nil {
		return mapRow{}, false, err
	}
	return row, true, nil
}

func (m *sqlMapper) put(row mapRow) error {
	_, err := m.db.Exec(m.q(`INSERT INTO `+mapTable+` (dest_tenant, source_table, source_key, goso_kind, goso_ref) VALUES (?,?,?,?,?)`),
		row.DestTenant, row.SourceTable, row.SourceKey, row.Kind, row.Ref)
	return err
}

func (m *sqlMapper) list(dest string) ([]mapRow, error) {
	rows, err := m.db.Query(m.q(`SELECT dest_tenant, source_table, source_key, goso_kind, goso_ref FROM `+mapTable+` WHERE dest_tenant=? ORDER BY source_table, source_key`), dest)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []mapRow
	for rows.Next() {
		var row mapRow
		if err := rows.Scan(&row.DestTenant, &row.SourceTable, &row.SourceKey, &row.Kind, &row.Ref); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (m *sqlMapper) del(dest, table, key string) error {
	_, err := m.db.Exec(m.q(`DELETE FROM `+mapTable+` WHERE dest_tenant=? AND source_table=? AND source_key=?`), dest, table, key)
	return err
}

type memMapper struct {
	mu   sync.Mutex
	rows map[string]mapRow
}

func (m *memMapper) key(dest, table, key string) string {
	return dest + "\n" + table + "\n" + key
}

func (m *memMapper) get(dest, table, key string) (mapRow, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.rows[m.key(dest, table, key)]
	return row, ok, nil
}

func (m *memMapper) put(row mapRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(row.DestTenant, row.SourceTable, row.SourceKey)
	if _, ok := m.rows[k]; ok {
		return fmt.Errorf("agentimport: map is closed")
	}
	if m.rows == nil {
		m.rows = map[string]mapRow{}
	}
	m.rows[k] = row
	return nil
}

func (m *memMapper) list(dest string) ([]mapRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []mapRow
	for _, row := range m.rows {
		if row.DestTenant == dest {
			out = append(out, row)
		}
	}
	sortMapRows(out)
	return out, nil
}

func (m *memMapper) del(dest, table, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, m.key(dest, table, key))
	return nil
}

func sortMapRows(rows []mapRow) {
	for i := 1; i < len(rows); i++ {
		j := i
		for j > 0 && (rows[j].SourceTable < rows[j-1].SourceTable || (rows[j].SourceTable == rows[j-1].SourceTable && rows[j].SourceKey < rows[j-1].SourceKey)) {
			rows[j], rows[j-1] = rows[j-1], rows[j]
			j--
		}
	}
}

var (
	storeLocks sync.Map
	memMaps    sync.Map
)

func withStoreLock(st store.StoreIface, fn func() error) error {
	key := fmt.Sprintf("%p", st)
	v, _ := storeLocks.LoadOrStore(key, new(sync.Mutex))
	mu := v.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	return fn()
}

type sqlPool interface {
	SQL() (*sql.DB, bool)
}

func openMapper(st store.StoreIface, write bool) (mapper, error) {
	if pool, ok := st.(sqlPool); ok {
		db, pg := pool.SQL()
		if db != nil {
			if !write {
				ok, err := mapTablePresent(db, pg)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nopMapper{}, nil
				}
				return &sqlMapper{db: db, pg: pg}, nil
			}
			if err := ensureMapTable(db, pg); err != nil {
				return nil, err
			}
			return &sqlMapper{db: db, pg: pg}, nil
		}
	}
	key := fmt.Sprintf("%p", st)
	v, _ := memMaps.LoadOrStore(key, &memMapper{rows: map[string]mapRow{}})
	return v.(*memMapper), nil
}

func mapTablePresent(db *sql.DB, pg bool) (bool, error) {
	var n int
	var err error
	if pg {
		err = db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1`, mapTable).Scan(&n)
	} else {
		err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, mapTable).Scan(&n)
	}
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func ensureMapTable(db *sql.DB, pg bool) error {
	q := `CREATE TABLE IF NOT EXISTS ` + mapTable + ` (
		dest_tenant TEXT NOT NULL,
		source_table TEXT NOT NULL,
		source_key TEXT NOT NULL,
		goso_kind TEXT NOT NULL,
		goso_ref TEXT NOT NULL,
		PRIMARY KEY (dest_tenant, source_table, source_key)
	)`
	if _, err := db.Exec(q); err != nil {
		return err
	}
	_ = pg
	return nil
}

func linkRef(edges [][2]string) string {
	parts := make([]string, 0, len(edges))
	for _, e := range edges {
		parts = append(parts, e[0]+"\t"+e[1])
	}
	return strings.Join(parts, "\n")
}

func parseLinkRef(ref string) [][2]string {
	if strings.TrimSpace(ref) == "" {
		return nil
	}
	var out [][2]string
	for _, line := range strings.Split(ref, "\n") {
		from, to, ok := strings.Cut(line, "\t")
		if !ok || from == "" || to == "" {
			continue
		}
		out = append(out, [2]string{from, to})
	}
	return out
}

func memberRef(teamID, agentID string) string {
	return teamID + "\t" + agentID
}

func parseMemberRef(ref string) (string, string, bool) {
	teamID, agentID, ok := strings.Cut(ref, "\t")
	if !ok || teamID == "" || agentID == "" {
		return "", "", false
	}
	return teamID, agentID, true
}
