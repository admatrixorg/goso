// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"strings"
)

// errKGSessionNotFound is returned when a KG write names a session that is
// missing or in another tenant. The caller-supplied customer_key is ignored.
var errKGSessionNotFound = errors.New("session not found")

// KGScope is the customer visibility of one knowledge-graph read.
//
// The zero value is web/API: every row in the tenant, the same as memory
// recall with CustomerScoped false. Web chat stays agent-wide.
//
// Scoped is a channel caller. A row is visible when its customer_key is
// empty (shared with every customer) or equal to CustomerKey. Another
// customer's rows are not. This is the memory stamp (CustomerKeyForLabel,
// set on write, not taken from a caller-supplied field) with the shared
// rows kept, which memory notes do not do.
type KGScope struct {
	Scoped      bool
	CustomerKey string
}

// Allows reports whether a row with customerKey is visible under sc.
func (sc KGScope) Allows(customerKey string) bool {
	if !sc.Scoped {
		return true
	}
	return customerKey == "" || customerKey == sc.CustomerKey
}

// kgCustomerPredicate is the SQL form of Allows. col is a fixed identifier
// such as "customer_key" or "e.customer_key", never request input.
// Unscoped reads add nothing, so existing web queries stay as they are.
func kgCustomerPredicate(col string, sc KGScope) (string, []any) {
	if !sc.Scoped {
		return "", nil
	}
	return ` AND (` + col + ` = '' OR ` + col + ` = ?)`, []any{sc.CustomerKey}
}

// kgCustomerIndexSQL is the production Postgres index build for SPEC 182.
// migratePostgres does not run it. CREATE INDEX CONCURRENTLY cannot run in
// the startup path, and the build waits for Dat's approval.
// Operators run docs/qa/182-kg-customer-scope/migration.sql after the
// column exists. SQLite creates the same keys at open; it has no CONCURRENTLY.
const kgCustomerIndexSQL = `
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_kg_entities_customer
    ON kg_entities (tenant_id, agent_id, customer_key);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_kg_relations_customer
    ON kg_relations (tenant_id, customer_key);
`

// stampKGCustomer clears a caller-supplied key and, when sessionID is set,
// stamps CustomerKeyForLabel of that session. A missing session is an error.
// A web label stamps "". The key is not read back from the session later.
func stampKGCustomer(sess *Session, sessionID, tenant string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", nil
	}
	if sess == nil || !SameTenant(sess.TenantID, tenant) {
		return "", errKGSessionNotFound
	}
	return CustomerKeyForLabel(sess.Label), nil
}
