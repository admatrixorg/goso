// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Tenant registry persistence.
//
// These tables are new. migrateTenantRegistry only creates them. It does not
// ALTER, rebuild, or backfill agents, memories, vault, contacts, or any other
// existing table. An empty tenant_registry means the process still starts
// with the in-memory master row and nothing else, which is the previous
// startup behavior, so there is no feature flag.
//
// Member order is pos, not a guess from id text. Subject is stored as the
// operator label the registry already accepted. No token column exists.

// tenantRegistryMigrateFault, when set, aborts migrateTenantRegistry after
// the named step. Tests use it to prove the migration rolls back.
// Production leaves it nil.
var tenantRegistryMigrateFault func(step string) error

// TenantRegistryMember is one access row. Subject is a label, not a credential.
type TenantRegistryMember struct {
	ID        string
	Subject   string
	Role      string
	CreatedAt time.Time
	Pos       int
}

// TenantRegistryRow is one tenant lifecycle row plus its members.
type TenantRegistryRow struct {
	ID        string
	Name      string
	Status    string
	Master    bool
	CreatedAt time.Time
	Members   []TenantRegistryMember
}

// TenantRegistryFault is a store failure whose text is safe to return.
// Class is a fixed token. The driver message is not retained, because it
// can echo a bound subject or slug.
type TenantRegistryFault struct {
	Op    string
	Class string
}

func (e *TenantRegistryFault) Error() string {
	return "tenant registry " + safeTenantRegistryOp(e.Op) + " failed"
}

func (s *SQLiteStore) migrateTenantRegistry() error {
	if s == nil || s.db == nil {
		return &TenantRegistryFault{Op: "load", Class: "store"}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return tenantRegistryCause("load", err)
	}
	defer func() { _ = tx.Rollback() }()
	steps := []struct {
		name string
		sql  string
	}{
		{"tenant_registry", `CREATE TABLE IF NOT EXISTS tenant_registry (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			master INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`},
		{"tenant_registry_members", `CREATE TABLE IF NOT EXISTS tenant_registry_members (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			subject TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at TEXT NOT NULL,
			pos INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (tenant_id) REFERENCES tenant_registry(id)
		)`},
		{"idx_tenant_registry_members_tenant", `CREATE INDEX IF NOT EXISTS idx_tenant_registry_members_tenant ON tenant_registry_members(tenant_id, pos)`},
		{"uq_tenant_registry_members_subject", `CREATE UNIQUE INDEX IF NOT EXISTS uq_tenant_registry_members_subject ON tenant_registry_members(tenant_id, subject)`},
	}
	for _, step := range steps {
		if _, err := tx.Exec(step.sql); err != nil {
			return fmt.Errorf("migrate tenant registry %s: %w", step.name, err)
		}
		if tenantRegistryMigrateFault != nil {
			if err := tenantRegistryMigrateFault(step.name); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// ListTenantRegistry returns every persisted tenant and its members.
// Member slices follow pos. A driver failure is an opaque fault.
func (s *SQLiteStore) ListTenantRegistry() ([]TenantRegistryRow, error) {
	if s == nil || s.db == nil {
		return nil, &TenantRegistryFault{Op: "load", Class: "store"}
	}
	rows, err := s.db.Query(`SELECT id, name, status, master, created_at FROM tenant_registry ORDER BY id`)
	if err != nil {
		return nil, tenantRegistryCause("load", err)
	}
	defer rows.Close()
	out := []TenantRegistryRow{}
	for rows.Next() {
		row, err := scanTenantRegistry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, tenantRegistryCause("load", err)
	}
	if err := rows.Close(); err != nil {
		return nil, tenantRegistryCause("load", err)
	}
	byID := make(map[string]*TenantRegistryRow, len(out))
	for i := range out {
		byID[out[i].ID] = &out[i]
	}
	members, err := s.db.Query(`SELECT id, tenant_id, subject, role, created_at, pos
		FROM tenant_registry_members ORDER BY tenant_id, pos, id`)
	if err != nil {
		return nil, tenantRegistryCause("load", err)
	}
	defer members.Close()
	for members.Next() {
		var mem TenantRegistryMember
		var tenantID, created string
		if err := members.Scan(&mem.ID, &tenantID, &mem.Subject, &mem.Role, &created, &mem.Pos); err != nil {
			return nil, tenantRegistryCause("load", err)
		}
		at, err := parseTenantRegistryTime(created)
		if err != nil {
			return nil, err
		}
		mem.CreatedAt = at
		owner := byID[tenantID]
		if owner == nil {
			return nil, &TenantRegistryFault{Op: "load", Class: "schema"}
		}
		owner.Members = append(owner.Members, mem)
	}
	if err := members.Err(); err != nil {
		return nil, tenantRegistryCause("load", err)
	}
	return out, nil
}

// PutTenantRegistry inserts or replaces one tenant and its full member list
// in one transaction. A failure leaves the previous rows in place.
func (s *SQLiteStore) PutTenantRegistry(row TenantRegistryRow) error {
	if s == nil || s.db == nil {
		return &TenantRegistryFault{Op: "save", Class: "store"}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return tenantRegistryCause("save", err)
	}
	defer func() { _ = tx.Rollback() }()
	master := 0
	if row.Master {
		master = 1
	}
	if _, err := tx.Exec(`INSERT INTO tenant_registry(id, name, status, master, created_at)
		VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			status=excluded.status,
			master=excluded.master,
			created_at=excluded.created_at`,
		row.ID, row.Name, row.Status, master, formatTime(row.CreatedAt)); err != nil {
		return tenantRegistryCause("save", err)
	}
	if _, err := tx.Exec(`DELETE FROM tenant_registry_members WHERE tenant_id=?`, row.ID); err != nil {
		return tenantRegistryCause("save", err)
	}
	for i, mem := range row.Members {
		if _, err := tx.Exec(`INSERT INTO tenant_registry_members(id, tenant_id, subject, role, created_at, pos)
			VALUES(?,?,?,?,?,?)`,
			mem.ID, row.ID, mem.Subject, mem.Role, formatTime(mem.CreatedAt), i); err != nil {
			return tenantRegistryCause("save", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return tenantRegistryCause("save", err)
	}
	return nil
}

func scanTenantRegistry(sc scanner) (TenantRegistryRow, error) {
	var row TenantRegistryRow
	var master int
	var created string
	if err := sc.Scan(&row.ID, &row.Name, &row.Status, &master, &created); err != nil {
		return TenantRegistryRow{}, tenantRegistryCause("load", err)
	}
	row.Master = master != 0
	at, err := parseTenantRegistryTime(created)
	if err != nil {
		return TenantRegistryRow{}, err
	}
	row.CreatedAt = at
	row.Members = []TenantRegistryMember{}
	return row, nil
}

func parseTenantRegistryTime(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, &TenantRegistryFault{Op: "load", Class: "schema"}
}

func tenantRegistryCause(op string, cause error) error {
	if cause == nil {
		return &TenantRegistryFault{Op: op, Class: "driver"}
	}
	var inner *TenantRegistryFault
	if errors.As(cause, &inner) && inner != nil {
		class := safeTenantRegistryClass(inner.Class)
		if class == "" {
			class = "driver"
		}
		return &TenantRegistryFault{Op: op, Class: class}
	}
	if errors.Is(cause, sql.ErrConnDone) {
		return &TenantRegistryFault{Op: op, Class: "closed"}
	}
	return &TenantRegistryFault{Op: op, Class: tenantRegistryClass(cause)}
}

func tenantRegistryClass(err error) string {
	if err == nil {
		return "driver"
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "closed"), strings.Contains(s, "conn done"):
		return "closed"
	case strings.Contains(s, "unique"), strings.Contains(s, "duplicate"):
		return "conflict"
	case strings.Contains(s, "no such"), strings.Contains(s, "syntax"), strings.Contains(s, "does not exist"):
		return "schema"
	default:
		return "driver"
	}
}

func safeTenantRegistryOp(op string) string {
	switch op {
	case "load", "save":
		return op
	default:
		return "op"
	}
}

func safeTenantRegistryClass(class string) string {
	switch class {
	case "closed", "conflict", "schema", "driver", "store":
		return class
	default:
		return "driver"
	}
}
