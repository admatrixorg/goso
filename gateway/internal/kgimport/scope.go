// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package kgimport

import "strings"

// Skip reasons. Dry-run and apply use the same strings.
// expired stays in the report for the shared importer shape. SPEC 181
// does not drop a KG row because valid_until is in the past.
const (
	reasonExpired         = "expired"
	reasonCustomerPrivate = "customer-private"
	reasonTeamScope       = "team/scope"
	reasonUnmapped        = "unmapped"
	reasonMissingEndpoint = "missing relation endpoint"
	reasonExists          = "exists"
	reasonNativeExists    = "native-exists"
	reasonInvalid         = "invalid"
)

// reasonKeys is every counter the report prints, including the ones this
// spec keeps at zero.
var reasonKeys = []string{
	reasonExpired,
	reasonCustomerPrivate,
	reasonTeamScope,
	reasonUnmapped,
	reasonMissingEndpoint,
	reasonExists,
	reasonNativeExists,
	reasonInvalid,
}

// admit is the only user_id / team_id / custom_scope decision in SPEC 181.
//
// An empty result means the row is tenant-wide and may be written.
// team_id and custom_scope are a guard: GoClaw's KG counts are zero for
// team_id, and kg_entities / kg_relations have no custom_scope column.
// A non-empty value is still counted as team/scope and is not written.
// There is no separate import path for those rows.
//
// A non-empty user_id is that customer's row. SPEC 182 added customer_key
// on the Goso KG tables, so this importer now writes those rows with the
// key set instead of holding them as customer-private. A row is visible to
// a read scoped to that customer and invisible to a read scoped to another
// customer; the tenant-wide rows (empty user_id) stay visible to everyone
// in the tenant. team/scope stays a skip: kg_entities and kg_relations have
// no team column.
func admit(userID, teamID, customScope string) string {
	if strings.TrimSpace(teamID) != "" || strings.TrimSpace(customScope) != "" {
		return reasonTeamScope
	}
	return ""
}
