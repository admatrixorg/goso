// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package tenant

import (
	"errors"
	"log"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// registryDB is the SQL write path. The in-memory store does not implement it.
type registryDB interface {
	PutTenantRegistry(store.TenantRegistryRow) error
}

// tenantRegistryLogf writes registry failures. The format is
// "goso tenant registry: <op> failed class=<class>". Both tokens come from
// a fixed set. Slugs, names, and member labels are not passed in.
var tenantRegistryLogf = log.Printf

// Load reads tenant rows from a SQLite or Postgres store and returns a
// registry that writes through to that store. A nil store, or the in-memory
// store, returns (nil, nil) so the caller keeps DefaultRegistry.
//
// An empty table loads as master-only, the same startup state as New().
// Master is always present. Load returns ErrLoad when the read fails or a
// stored row breaks the registry rules. It does not apply a partial set.
func Load(st store.StoreIface) (*Registry, error) {
	db := registrySQL(st)
	if db == nil {
		return nil, nil
	}
	rows, err := db.ListTenantRegistry()
	if err != nil {
		logTenantRegistry("load", err)
		return nil, ErrLoad
	}
	reg, err := registryFromRows(rows, db)
	if err != nil {
		logTenantRegistry("load", err)
		return nil, ErrLoad
	}
	return reg, nil
}

func registrySQL(st store.StoreIface) *store.SQLiteStore {
	switch s := st.(type) {
	case *store.SQLiteStore:
		return s
	case *store.PostgresStore:
		if s == nil {
			return nil
		}
		return s.SQLiteStore
	default:
		return nil
	}
}

func registryFromRows(rows []store.TenantRegistryRow, db registryDB) (*Registry, error) {
	r := New()
	r.save = db
	var maxSeq int64
	for _, row := range rows {
		rec, n, err := recordFromRow(row)
		if err != nil {
			return nil, err
		}
		if _, exists := r.rows[rec.ID]; exists && rec.ID != Default {
			return nil, tenantSchemaFault()
		}
		if n > maxSeq {
			maxSeq = n
		}
		r.rows[rec.ID] = rec
	}
	if _, ok := r.rows[Default]; !ok {
		return nil, tenantSchemaFault()
	}
	if len(r.rows) > maxTenants {
		return nil, tenantSchemaFault()
	}
	if maxSeq > 0 {
		r.seq.Store(maxSeq)
	}
	return r, nil
}

func recordFromRow(row store.TenantRegistryRow) (*record, int64, error) {
	id := strings.TrimSpace(row.ID)
	if !store.TenantOK(id) {
		return nil, 0, tenantSchemaFault()
	}
	name := row.Name
	if strings.TrimSpace(name) == "" || len(name) > maxName {
		return nil, 0, tenantSchemaFault()
	}
	if !statusOK(row.Status) {
		return nil, 0, tenantSchemaFault()
	}
	master := id == Default
	if row.Master != master {
		return nil, 0, tenantSchemaFault()
	}
	if master && row.Status != StatusActive {
		return nil, 0, tenantSchemaFault()
	}
	if row.CreatedAt.IsZero() {
		return nil, 0, tenantSchemaFault()
	}
	if len(row.Members) > maxMembers {
		return nil, 0, tenantSchemaFault()
	}
	members := make([]Member, 0, len(row.Members))
	seenID := map[string]struct{}{}
	seenSubject := map[string]struct{}{}
	var maxSeq int64
	for _, m := range row.Members {
		mid := strings.TrimSpace(m.ID)
		if mid == "" || len(mid) > 64 {
			return nil, 0, tenantSchemaFault()
		}
		if _, ok := seenID[mid]; ok {
			return nil, 0, tenantSchemaFault()
		}
		seenID[mid] = struct{}{}
		if !subjectOK(m.Subject) || !roleOK(m.Role) {
			return nil, 0, tenantSchemaFault()
		}
		key := strings.ToLower(m.Subject)
		if _, ok := seenSubject[key]; ok {
			return nil, 0, tenantSchemaFault()
		}
		seenSubject[key] = struct{}{}
		if n := memberSeq(mid); n > maxSeq {
			maxSeq = n
		}
		members = append(members, Member{
			ID: mid, Subject: m.Subject, Role: m.Role, CreatedAt: m.CreatedAt.UTC(),
		})
	}
	return &record{
		ID: id, Name: name, Status: row.Status, Master: master,
		CreatedAt: row.CreatedAt.UTC(), Members: members,
	}, maxSeq, nil
}

func tenantSchemaFault() error {
	return &store.TenantRegistryFault{Op: "load", Class: "schema"}
}

func (r *Registry) persist(op string, rec *record) error {
	if r == nil || r.save == nil || rec == nil {
		return nil
	}
	if err := r.save.PutTenantRegistry(registryRow(rec)); err != nil {
		logTenantRegistry(op, err)
		return ErrStore
	}
	return nil
}

func registryRow(rec *record) store.TenantRegistryRow {
	members := make([]store.TenantRegistryMember, len(rec.Members))
	for i, m := range rec.Members {
		members[i] = store.TenantRegistryMember{
			ID: m.ID, Subject: m.Subject, Role: m.Role, CreatedAt: m.CreatedAt, Pos: i,
		}
	}
	return store.TenantRegistryRow{
		ID: rec.ID, Name: rec.Name, Status: rec.Status, Master: rec.Master,
		CreatedAt: rec.CreatedAt, Members: members,
	}
}

func cloneRecord(rec *record) *record {
	if rec == nil {
		return nil
	}
	cp := *rec
	cp.Members = append([]Member(nil), rec.Members...)
	return &cp
}

func memberSeq(id string) int64 {
	const prefix = "tm_"
	if !strings.HasPrefix(id, prefix) {
		return 0
	}
	n, err := strconv.ParseInt(id[len(prefix):], 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func logTenantRegistry(op string, err error) {
	if err == nil {
		return
	}
	class := "driver"
	var fault *store.TenantRegistryFault
	if errors.As(err, &fault) && fault != nil {
		class = safeTenantLogClass(fault.Class)
	}
	tenantRegistryLogf("goso tenant registry: %s failed class=%s", safeTenantLogOp(op), class)
}

func safeTenantLogOp(op string) string {
	switch op {
	case "load", "create", "status", "member_add", "member_role", "member_remove":
		return op
	default:
		return "op"
	}
}

func safeTenantLogClass(class string) string {
	switch class {
	case "closed", "conflict", "schema", "driver", "store":
		return class
	default:
		return "driver"
	}
}
