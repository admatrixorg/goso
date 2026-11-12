// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package mcpimport loads MCP servers and tool config from an offline
// GoClaw export into goso's existing connector and agent-link rows.
//
// The file is JSON or JSONL. Column names match the source tables. This
// package does not open a database by itself and does not read credential
// tables into the store. mcp_servers.api_key, headers, env, and URL userinfo
// or query strings are not copied. An imported connector is stored disabled
// with an empty credential_ref.
//
// Connectors have no source-id column. The existing schema_version text is
// the source-id map: goclaw-src:<dest tenant>:<source id>. A later run that
// sees that exact value does not insert another row. If the stored row
// differs from the export, the reason is native-exists and the row is not
// updated. A connector this importer did not write is left as it is.
package mcpimport

import (
	"errors"
	"sort"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Store is the subset of the goso store this importer writes. It cannot
// update, delete, or store a secret.
type Store interface {
	ListConnectors() []*store.ConnectorRecord
	CreateConnector(store.ConnectorRecord) (*store.ConnectorRecord, error)
	ListAgents() []*store.Agent
	LinkAgentConnector(agentID, connectorName string) error
	ListAgentConnectors(agentID string) ([]string, error)
}

// Options is one run. Tenant is the goso tenant written into connector names
// and the source-id marker. SourceTenant is the export tenant_id and is
// required. Deny, when set, stops the run before any write when Tenant or
// SourceTenant is on the list.
type Options struct {
	Tenant       string
	SourceTenant string
	DryRun       bool
	Deny         map[string]struct{}
}

// Report is the dry-run and apply result. NeedsCredential names servers
// whose secret was left empty. NotImported is the decision list. Neither
// carries secret values.
type Report struct {
	DryRun          bool             `json:"dry_run"`
	Tenants         []TenantReport   `json:"tenants"`
	BuiltinTools    Counts           `json:"builtin_tools"`
	NeedsCredential []CredentialNeed `json:"needs_credential"`
	NotImported     []Decision       `json:"not_imported"`
	DeniedTenants   int              `json:"denied_tenants"`
}

// TenantReport is one source tenant_id.
type TenantReport struct {
	TenantID                 string `json:"tenant_id"`
	MCPServers               Counts `json:"mcp_servers"`
	MCPAgentGrants           Counts `json:"mcp_agent_grants"`
	MCPUserGrants            Counts `json:"mcp_user_grants"`
	BuiltinToolTenantConfigs Counts `json:"builtin_tool_tenant_configs"`
	CredentialTables         Counts `json:"credential_tables"`
}

// Counts is one table inside one tenant. Input is inserted + skipped +
// rejected + not_imported.
type Counts struct {
	Input       int      `json:"input"`
	Inserted    int      `json:"inserted"`
	Skipped     int      `json:"skipped"`
	Rejected    int      `json:"rejected"`
	NotImported int      `json:"not_imported"`
	Reasons     []Reason `json:"reasons,omitempty"`
}

// Reason is how many rows share one code.
type Reason struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

// CredentialNeed is one accepted server whose secret was not copied.
type CredentialNeed struct {
	TenantID   string `json:"tenant_id"`
	SourceID   string `json:"source_id"`
	SourceName string `json:"source_name"`
	GosoName   string `json:"goso_name"`
	Reason     string `json:"reason"`
}

// Decision is a table or column that was not written.
type Decision struct {
	Table  string `json:"table"`
	Column string `json:"column,omitempty"`
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// ConfigError is a bad option. The text names the option, not a row.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func errConfig(msg string) error { return &ConfigError{Msg: msg} }

const (
	reasonNeedsDecision = "needs-decision"
	reasonCredentialTbl = "credential-table"
	reasonNeedsCred     = "needs-credential"
	reasonNativeExists  = "native-exists"
	markerPrefix        = "goclaw-src:"
)

// Import plans every row, then writes only on apply. Dry-run uses the same
// counts and does not call CreateConnector or LinkAgentConnector.
func Import(db Store, ex *Export, opt Options) (Report, error) {
	dest := strings.TrimSpace(opt.Tenant)
	if dest == "" {
		return Report{}, errConfig("tenant is required")
	}
	if !store.TenantOK(dest) {
		return Report{}, errConfig("tenant is invalid")
	}
	source := strings.TrimSpace(opt.SourceTenant)
	if source == "" {
		return Report{}, errConfig("source tenant is required")
	}
	if !store.TenantOK(source) {
		return Report{}, errConfig("source tenant is invalid")
	}
	if len(opt.Deny) == 0 {
		return Report{}, errConfig("deny file")
	}
	if n := deniedFlags(opt.Deny, dest, source); n > 0 {
		return deniedReport(opt.DryRun, n), errConfig("tenant is denied")
	}
	if ex == nil {
		return Report{}, errConfig("export is required")
	}
	if db == nil {
		return Report{}, errConfig("store is required")
	}

	byName, byMarker, err := indexConnectors(db)
	if err != nil {
		return Report{}, err
	}
	agents := db.ListAgents()
	linked, err := indexLinks(db, agents, dest)
	if err != nil {
		return Report{}, err
	}

	tenants := map[string]*accTenant{}
	cols := map[string]int{}
	var needs []CredentialNeed
	var inserts []store.ConnectorRecord
	var links []linkPlan

	accepted := map[string]acceptedServer{}
	blocked := map[string]string{}
	seenServer := map[string]struct{}{}
	usedNames := map[string]string{}
	rowDenied := map[string]struct{}{}

	for _, row := range ex.servers {
		if noteDenied(opt.Deny, row.TenantID, rowDenied) {
			continue
		}
		id := reportTenant(row.TenantID, dest, source, row.Bad || row.Control)
		bucket := tenants[id]
		if bucket == nil {
			bucket = newAccTenant(id)
			tenants[id] = bucket
		}
		noteServerColumns(cols, row)
		if row.Bad {
			bucket.servers.add("rejected", "invalid")
			continue
		}
		if row.Control {
			bucket.servers.add("rejected", "control")
			continue
		}
		if reason := tenantReason(row.TenantID, dest, source); reason != "" {
			bucket.servers.add("rejected", reason)
			continue
		}
		if row.EnabledBad {
			bucket.servers.add("rejected", "invalid")
			continue
		}
		if row.TimeoutBad {
			bucket.servers.add("rejected", "invalid-timeout")
			continue
		}
		if !validSourceID(row.ID) {
			bucket.servers.add("rejected", "invalid-id")
			continue
		}
		if !validName(row.Name) {
			bucket.servers.add("rejected", "invalid-name")
			continue
		}
		transport := mapTransport(row.Transport)
		if transport == "" {
			bucket.servers.add("rejected", "invalid-transport")
			continue
		}
		goso := connectorName(dest, row.Name)
		if goso == "" || goso == "builtin" {
			bucket.servers.add("rejected", "invalid-name")
			continue
		}
		if row.SecretHold {
			seenServer[row.ID] = struct{}{}
			bucket.servers.add("skipped", reasonNeedsCred)
			needs = append(needs, credNeed(dest, row, goso))
			continue
		}
		if row.EndpointReason != "" || row.Endpoint == "" {
			bucket.servers.add("rejected", "invalid-endpoint")
			continue
		}
		if _, ok := seenServer[row.ID]; ok {
			bucket.servers.add("skipped", "duplicate")
			continue
		}
		marker := sourceMarker(dest, row.ID)
		want := store.ConnectorRecord{
			Name:          goso,
			Transport:     transport,
			Endpoint:      row.Endpoint,
			SchemaVersion: marker,
			Enabled:       false,
			TimeoutMS:     row.TimeoutMS,
		}
		if cur, ok := byMarker[marker]; ok && cur != nil {
			seenServer[row.ID] = struct{}{}
			if sameImported(cur, want) {
				accepted[row.ID] = acceptedServer{gosoName: cur.Name, sourceName: row.Name, sourceID: row.ID}
				bucket.servers.add("skipped", "unchanged")
				if row.NeedsCredential {
					needs = append(needs, credNeed(dest, row, cur.Name))
				}
			} else {
				blocked[row.ID] = reasonNativeExists
				bucket.servers.add("skipped", reasonNativeExists)
			}
			continue
		}
		if cur, ok := byName[goso]; ok && cur != nil {
			seenServer[row.ID] = struct{}{}
			if sameImported(cur, want) {
				accepted[row.ID] = acceptedServer{gosoName: cur.Name, sourceName: row.Name, sourceID: row.ID}
				bucket.servers.add("skipped", "unchanged")
				if row.NeedsCredential {
					needs = append(needs, credNeed(dest, row, cur.Name))
				}
			} else {
				blocked[row.ID] = reasonNativeExists
				bucket.servers.add("skipped", reasonNativeExists)
			}
			continue
		}
		if owner, ok := usedNames[goso]; ok && owner != row.ID {
			seenServer[row.ID] = struct{}{}
			blocked[row.ID] = reasonNativeExists
			bucket.servers.add("skipped", reasonNativeExists)
			continue
		}
		seenServer[row.ID] = struct{}{}
		usedNames[goso] = row.ID
		rec := store.ConnectorRecord{
			Name:          goso,
			Transport:     transport,
			Endpoint:      row.Endpoint,
			SchemaVersion: marker,
			Enabled:       false,
			TimeoutMS:     row.TimeoutMS,
		}
		byName[goso] = &rec
		byMarker[marker] = &rec
		accepted[row.ID] = acceptedServer{gosoName: goso, sourceName: row.Name, sourceID: row.ID}
		inserts = append(inserts, rec)
		bucket.servers.add("inserted", "")
		if row.NeedsCredential {
			needs = append(needs, credNeed(dest, row, goso))
		}
	}

	plannedGrant := map[string]struct{}{}
	for _, row := range ex.agentGrants {
		if noteDenied(opt.Deny, row.TenantID, rowDenied) {
			continue
		}
		id := reportTenant(row.TenantID, dest, source, row.Bad || row.Control)
		bucket := tenants[id]
		if bucket == nil {
			bucket = newAccTenant(id)
			tenants[id] = bucket
		}
		noteGrantColumns(cols, row)
		if row.Bad {
			bucket.grants.add("rejected", "invalid")
			continue
		}
		if row.Control {
			bucket.grants.add("rejected", "control")
			continue
		}
		if reason := tenantReason(row.TenantID, dest, source); reason != "" {
			bucket.grants.add("rejected", reason)
			continue
		}
		if !validSourceID(row.ServerID) || !validRef(row.AgentID) {
			bucket.grants.add("rejected", "invalid-id")
			continue
		}
		key := row.ServerID + "\x00" + row.AgentID
		if _, ok := plannedGrant[key]; ok {
			bucket.grants.add("skipped", "duplicate")
			continue
		}
		plannedGrant[key] = struct{}{}
		srv, ok := accepted[row.ServerID]
		if !ok {
			if blocked[row.ServerID] == reasonNativeExists {
				bucket.grants.add("rejected", "server-not-owned")
			} else {
				bucket.grants.add("rejected", "server-missing")
			}
			continue
		}
		agentID, reason := resolveAgent(agents, dest, row.AgentID)
		if reason != "" {
			bucket.grants.add("rejected", reason)
			continue
		}
		if linked[agentID] != nil {
			if _, exists := linked[agentID][srv.gosoName]; exists {
				bucket.grants.add("skipped", "unchanged")
				continue
			}
		}
		if !row.Enabled {
			bucket.grants.add("skipped", "grant-disabled")
			continue
		}
		if row.HadAllow || row.HadDeny || row.HadOverrides {
			bucket.grants.add("not_imported", reasonNeedsDecision)
			continue
		}
		if linked[agentID] == nil {
			linked[agentID] = map[string]struct{}{}
		}
		linked[agentID][srv.gosoName] = struct{}{}
		links = append(links, linkPlan{agentID: agentID, connector: srv.gosoName})
		bucket.grants.add("inserted", "")
	}

	addTenants(tenants, ex.userGrants, dest, source, opt.Deny, rowDenied, func(b *accTenant) *acc { return &b.userGrants }, reasonNeedsDecision)
	addTenants(tenants, ex.builtinConfigs, dest, source, opt.Deny, rowDenied, func(b *accTenant) *acc { return &b.builtinConfigs }, reasonNeedsDecision)
	addTenants(tenants, ex.userCredentials, dest, source, opt.Deny, rowDenied, func(b *accTenant) *acc { return &b.credentials }, reasonCredentialTbl)
	addTenants(tenants, ex.contextCredentials, dest, source, opt.Deny, rowDenied, func(b *accTenant) *acc { return &b.credentials }, reasonCredentialTbl)

	rep := Report{
		DryRun:          opt.DryRun,
		Tenants:         finishTenants(tenants),
		BuiltinTools:    finishCounts(acc{Counts: Counts{Input: ex.builtinTools, NotImported: ex.builtinTools}, reasons: reasonMap(ex.builtinTools, reasonNeedsDecision)}),
		NeedsCredential: finishNeeds(needs),
		NotImported:     finishDecisions(ex, cols),
		DeniedTenants:   len(rowDenied),
	}
	rep.redact()
	if err := balanced(rep); err != nil {
		return Report{}, err
	}
	if opt.DryRun {
		return rep, nil
	}
	for _, rec := range inserts {
		if _, err := db.CreateConnector(rec); err != nil {
			return Report{}, errors.New("import failed")
		}
	}
	for _, link := range links {
		if err := db.LinkAgentConnector(link.agentID, link.connector); err != nil {
			return Report{}, errors.New("import failed")
		}
	}
	return rep, nil
}

type acceptedServer struct {
	gosoName   string
	sourceName string
	sourceID   string
}

type linkPlan struct {
	agentID   string
	connector string
}

func indexConnectors(db Store) (map[string]*store.ConnectorRecord, map[string]*store.ConnectorRecord, error) {
	byName := map[string]*store.ConnectorRecord{}
	byMarker := map[string]*store.ConnectorRecord{}
	for _, c := range db.ListConnectors() {
		if c == nil || c.Name == "" {
			continue
		}
		if _, ok := byName[c.Name]; !ok {
			byName[c.Name] = c
		}
		if strings.HasPrefix(c.SchemaVersion, markerPrefix) {
			if _, ok := byMarker[c.SchemaVersion]; !ok {
				byMarker[c.SchemaVersion] = c
			}
		}
	}
	return byName, byMarker, nil
}

func indexLinks(db Store, agents []*store.Agent, dest string) (map[string]map[string]struct{}, error) {
	out := map[string]map[string]struct{}{}
	for _, a := range agents {
		if a == nil || !store.SameTenant(a.TenantID, dest) {
			continue
		}
		names, err := db.ListAgentConnectors(a.ID)
		if err != nil {
			return nil, errors.New("import failed")
		}
		set := map[string]struct{}{}
		for _, name := range names {
			set[name] = struct{}{}
		}
		out[a.ID] = set
	}
	return out, nil
}

func noteServerColumns(cols map[string]int, row serverInput) {
	addCol(cols, "mcp_servers", "display_name", row.HadDisplayName)
	addCol(cols, "mcp_servers", "headers", row.HadHeaders)
	addCol(cols, "mcp_servers", "env", row.HadEnv)
	addCol(cols, "mcp_servers", "api_key", row.HadAPIKey)
	addCol(cols, "mcp_servers", "tool_prefix", row.HadToolPrefix)
	addCol(cols, "mcp_servers", "settings", row.HadSettings)
	addCol(cols, "mcp_servers", "created_by", row.HadCreatedBy)
	addCol(cols, "mcp_servers", "created_at", row.HadCreatedAt)
	addCol(cols, "mcp_servers", "updated_at", row.HadUpdatedAt)
}

func noteGrantColumns(cols map[string]int, row grantInput) {
	addCol(cols, "mcp_agent_grants", "tool_allow", row.HadAllow)
	addCol(cols, "mcp_agent_grants", "tool_deny", row.HadDeny)
	addCol(cols, "mcp_agent_grants", "config_overrides", row.HadOverrides)
	addCol(cols, "mcp_agent_grants", "granted_by", row.HadGrantedBy)
	addCol(cols, "mcp_agent_grants", "created_at", row.HadCreatedAt)
}

func addCol(cols map[string]int, table, column string, set bool) {
	if set {
		cols[table+"\x00"+column]++
	}
}

func credNeed(dest string, row serverInput, goso string) CredentialNeed {
	return CredentialNeed{
		TenantID:   dest,
		SourceID:   row.ID,
		SourceName: row.Name,
		GosoName:   goso,
		Reason:     reasonNeedsCred,
	}
}

func deniedFlags(list map[string]struct{}, ids ...string) int {
	seen := map[string]struct{}{}
	for _, id := range ids {
		if Denied(list, id) {
			seen[strings.ToLower(strings.TrimSpace(id))] = struct{}{}
		}
	}
	return len(seen)
}

func deniedReport(dry bool, n int) Report {
	return Report{
		DryRun:          dry,
		Tenants:         []TenantReport{},
		NeedsCredential: []CredentialNeed{},
		NotImported:     []Decision{},
		DeniedTenants:   n,
	}
}

func noteDenied(list map[string]struct{}, id string, seen map[string]struct{}) bool {
	if !Denied(list, id) {
		return false
	}
	seen[strings.ToLower(strings.TrimSpace(id))] = struct{}{}
	return true
}

func addTenants(tenants map[string]*accTenant, rows []tenantInput, dest, source string, deny map[string]struct{}, denied map[string]struct{}, pick func(*accTenant) *acc, reason string) {
	for _, row := range rows {
		if noteDenied(deny, row.TenantID, denied) {
			continue
		}
		id := row.TenantID
		if row.Bad {
			id = ""
		}
		if id == "" && source == "" && !row.Bad {
			id = dest
		}
		bucket := tenants[id]
		if bucket == nil {
			bucket = newAccTenant(id)
			tenants[id] = bucket
		}
		pick(bucket).add("not_imported", reason)
	}
}

func reportTenant(rowTenantID, dest, source string, bad bool) string {
	if bad {
		return strings.TrimSpace(rowTenantID)
	}
	if reason := tenantReason(rowTenantID, dest, source); reason != "" {
		return strings.TrimSpace(rowTenantID)
	}
	if strings.TrimSpace(rowTenantID) == "" {
		return dest
	}
	return strings.TrimSpace(rowTenantID)
}

func tenantReason(rowTenantID, dest, source string) string {
	rowTenantID = strings.TrimSpace(rowTenantID)
	if source != "" {
		if rowTenantID != source {
			return "foreign-tenant"
		}
		return ""
	}
	if rowTenantID == "" {
		return ""
	}
	if !store.TenantOK(rowTenantID) {
		return "tenant"
	}
	if rowTenantID != dest {
		return "foreign-tenant"
	}
	return ""
}

func resolveAgent(agents []*store.Agent, dest, ref string) (string, string) {
	var keyHit *store.Agent
	for _, a := range agents {
		if a == nil {
			continue
		}
		if a.ID == ref {
			if store.SameTenant(a.TenantID, dest) {
				return a.ID, ""
			}
			return "", "agent-other-tenant"
		}
		if a.AgentKey == ref && keyHit == nil {
			keyHit = a
		}
	}
	if keyHit != nil {
		if store.SameTenant(keyHit.TenantID, dest) {
			return keyHit.ID, ""
		}
		return "", "agent-other-tenant"
	}
	return "", "agent-missing"
}

func sourceMarker(tenant, id string) string {
	return markerPrefix + tenant + ":" + id
}

// sameImported reports that the stored connector is already the row this
// import would insert. Any difference is left in place.
func sameImported(cur *store.ConnectorRecord, want store.ConnectorRecord) bool {
	if cur == nil {
		return false
	}
	if cur.Name != want.Name || cur.Transport != want.Transport || cur.Endpoint != want.Endpoint {
		return false
	}
	if cur.SchemaVersion != want.SchemaVersion || cur.TimeoutMS != want.TimeoutMS || cur.Retries != want.Retries {
		return false
	}
	if cur.Enabled || want.Enabled {
		return false
	}
	if strings.TrimSpace(cur.CredentialRef) != "" || strings.TrimSpace(want.CredentialRef) != "" {
		return false
	}
	if strings.TrimSpace(cur.ManifestURL) != "" || len(cur.ManifestJSON) > 0 {
		return false
	}
	return true
}

func connectorName(tenant, name string) string {
	t := slug(tenant)
	n := slug(name)
	if t == "" || n == "" {
		return ""
	}
	return "goclaw-" + t + "-" + n
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 80 {
		out = strings.Trim(out[:80], "-")
	}
	return out
}

func validSourceID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func validName(name string) bool {
	return name != "" && len(name) <= 255 && !hasControl(name)
}

func validRef(ref string) bool {
	return ref != "" && len(ref) <= 255 && !hasControl(ref)
}

type acc struct {
	Counts
	reasons map[string]int
}

func (a *acc) add(kind, reason string) {
	if a.reasons == nil {
		a.reasons = map[string]int{}
	}
	a.Input++
	switch kind {
	case "inserted":
		a.Inserted++
	case "skipped":
		a.Skipped++
		a.reasons[reason]++
	case "rejected":
		a.Rejected++
		a.reasons[reason]++
	case "not_imported":
		a.NotImported++
		a.reasons[reason]++
	default:
		a.Rejected++
		a.reasons["invalid"]++
	}
}

type accTenant struct {
	id             string
	servers        acc
	grants         acc
	userGrants     acc
	builtinConfigs acc
	credentials    acc
}

func newAccTenant(id string) *accTenant {
	return &accTenant{id: id}
}

func finishTenants(m map[string]*accTenant) []TenantReport {
	if len(m) == 0 {
		return []TenantReport{}
	}
	ids := make([]string, 0, len(m))
	for id, bucket := range m {
		if bucket == nil || bucket.empty() {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]TenantReport, 0, len(ids))
	for _, id := range ids {
		b := m[id]
		out = append(out, TenantReport{
			TenantID:                 b.id,
			MCPServers:               finishCounts(b.servers),
			MCPAgentGrants:           finishCounts(b.grants),
			MCPUserGrants:            finishCounts(b.userGrants),
			BuiltinToolTenantConfigs: finishCounts(b.builtinConfigs),
			CredentialTables:         finishCounts(b.credentials),
		})
	}
	return out
}

func (b *accTenant) empty() bool {
	return b.servers.Input == 0 && b.grants.Input == 0 && b.userGrants.Input == 0 && b.builtinConfigs.Input == 0 && b.credentials.Input == 0
}

func finishCounts(a acc) Counts {
	a.Reasons = reasonList(a.reasons)
	return a.Counts
}

func reasonMap(n int, code string) map[string]int {
	if n == 0 {
		return nil
	}
	return map[string]int{code: n}
}

func reasonList(m map[string]int) []Reason {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k, n := range m {
		if k == "" || n == 0 {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return nil
	}
	out := make([]Reason, 0, len(keys))
	for _, k := range keys {
		out = append(out, Reason{Code: k, Count: m[k]})
	}
	return out
}

func finishNeeds(needs []CredentialNeed) []CredentialNeed {
	if len(needs) == 0 {
		return []CredentialNeed{}
	}
	sort.Slice(needs, func(i, j int) bool {
		if needs[i].TenantID != needs[j].TenantID {
			return needs[i].TenantID < needs[j].TenantID
		}
		if needs[i].SourceID != needs[j].SourceID {
			return needs[i].SourceID < needs[j].SourceID
		}
		return needs[i].SourceName < needs[j].SourceName
	})
	return needs
}

func finishDecisions(ex *Export, cols map[string]int) []Decision {
	var out []Decision
	if ex.builtinTools > 0 {
		out = append(out, Decision{Table: "builtin_tools", Reason: reasonNeedsDecision, Count: ex.builtinTools})
	}
	if n := len(ex.userGrants); n > 0 {
		out = append(out, Decision{Table: "mcp_user_grants", Reason: reasonNeedsDecision, Count: n})
	}
	if n := len(ex.builtinConfigs); n > 0 {
		out = append(out, Decision{Table: "builtin_tool_tenant_configs", Reason: reasonNeedsDecision, Count: n})
	}
	if n := len(ex.userCredentials); n > 0 {
		out = append(out, Decision{Table: "mcp_user_credentials", Reason: reasonCredentialTbl, Count: n})
	}
	if n := len(ex.contextCredentials); n > 0 {
		out = append(out, Decision{Table: "mcp_context_credentials", Reason: reasonCredentialTbl, Count: n})
	}
	keys := make([]string, 0, len(cols))
	for k, n := range cols {
		if n > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		table, column, _ := strings.Cut(k, "\x00")
		reason := reasonNeedsDecision
		if column == "api_key" {
			reason = "dropped-secret"
		}
		out = append(out, Decision{Table: table, Column: column, Reason: reason, Count: cols[k]})
	}
	if out == nil {
		return []Decision{}
	}
	return out
}

func balanced(rep Report) error {
	for _, t := range rep.Tenants {
		if !countsOK(t.MCPServers) || !countsOK(t.MCPAgentGrants) || !countsOK(t.MCPUserGrants) || !countsOK(t.BuiltinToolTenantConfigs) || !countsOK(t.CredentialTables) {
			return errors.New("import tally failed")
		}
	}
	if !countsOK(rep.BuiltinTools) {
		return errors.New("import tally failed")
	}
	return nil
}

func countsOK(c Counts) bool {
	if c.Input != c.Inserted+c.Skipped+c.Rejected+c.NotImported {
		return false
	}
	sum := 0
	for _, r := range c.Reasons {
		sum += r.Count
	}
	return sum == c.Skipped+c.Rejected+c.NotImported
}

var (
	_ Store = (*store.Store)(nil)
	_ Store = (*store.SQLiteStore)(nil)
	_ Store = (*store.PostgresStore)(nil)
)
