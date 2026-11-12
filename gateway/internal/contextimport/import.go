// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package contextimport loads one tenant from an offline context-file export
// into the context_files table from SPEC 150.
//
// The export mirrors the GoClaw tables agent_context_files and
// user_context_files. Column names are tenant_id, agent_id, file_name,
// content, created_at, updated_at, and, for a user row, user_id. The GoClaw
// uuid primary key is not stored. Goso keeps its own id. Upsert uses the
// GoClaw natural key: (agent_id, file_name) or (agent_id, user_id, file_name).
//
// agent_id selects an agent that already exists in the destination tenant
// (see Options.AgentMap). The stored agent owner is that agent's id.
// user_id is stored as the per-customer user_key. Goso user files are not
// per agent, so two agents that disagree on the same user_id and file_name
// are scope-conflict and neither body is written. The importer does not
// create agents, users, memories, or vault rows, and it does not open a
// database other than the one the caller passes.
//
// Unknown JSON fields are ignored. A later run writes nothing when the
// stored body and the source clocks already match.
package contextimport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// MaxExportBytes is the largest JSON document ReadExport will decode.
const MaxExportBytes = 32 << 20

const (
	reasonUnchanged        = "unchanged"
	reasonDuplicate        = "duplicate"
	reasonTenantMismatch   = "tenant-mismatch"
	reasonAgentMissing     = "agent-missing"
	reasonAgentOtherTenant = "agent-other-tenant"
	reasonInvalid          = "invalid"
	reasonTooLarge         = "too-large"
	reasonScopeConflict    = "scope-conflict"
	reasonNativeExists     = "native-exists"
)

// Options selects the single destination tenant for this run.
type Options struct {
	// SourceTenant is the export tenant_id to keep. Match is case-insensitive
	// after trim. Any other tenant_id is rejected and is not written.
	SourceTenant string
	// DestTenant is the goso tenant_id stored on context_files. It is not
	// defaulted: an empty or invalid id fails the run before any write.
	DestTenant string
	// Apply writes. The zero value is a dry-run, which reports the same
	// counts and does not call PutContextFile.
	Apply bool
	// AgentMap binds a GoClaw agent_id to a goso agent_key in DestTenant.
	// When an id is absent, an agent whose id or agent_key equals that
	// agent_id is used. The map does not create agents.
	AgentMap map[string]string
	// Customers indexes user_id to a customer key. Missing ids use goclaw:<user_id>.
	Customers map[string]string
	// DenyFile is a local denylist. It is required. A missing, empty, or
	// malformed file stops the run. Listed tenants are not named in the report.
	DenyFile string
	// Log receives rejection lines. Nil discards them. Arguments are names,
	// owner keys, and reason codes, never file bodies or user ids.
	Log Logger
}

// Logger is the rejection log. Printf must not be given a file body.
type Logger interface {
	Printf(format string, args ...any)
}

// Export is an offline dump of the two GoClaw tables. The uuid primary key
// and any other column are not represented, so the decoder drops them.
type Export struct {
	Agents []AgentFile `json:"agent_context_files"`
	Users  []UserFile  `json:"user_context_files"`
}

// AgentFile is one agent_context_files row.
// Natural key: agent_id + file_name.
type AgentFile struct {
	TenantID  string `json:"tenant_id"`
	AgentID   string `json:"agent_id"`
	FileName  string `json:"file_name"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// UserFile is one user_context_files row.
// Natural key: agent_id + user_id + file_name.
// user_id is stored as the per-customer user_key.
type UserFile struct {
	TenantID  string `json:"tenant_id"`
	AgentID   string `json:"agent_id"`
	UserID    string `json:"user_id"`
	FileName  string `json:"file_name"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ConfigError is a bad option. The text names the option, not a file body.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func errConfig(msg string) error { return &ConfigError{Msg: msg} }

// ReadExport decodes an export from r. A document larger than MaxExportBytes
// is refused before the rows are used.
func ReadExport(r io.Reader) (*Export, error) {
	return readExport(r, MaxExportBytes)
}

func readExport(r io.Reader, max int64) (*Export, error) {
	if r == nil {
		return nil, errors.New("read export")
	}
	if max <= 0 {
		max = MaxExportBytes
	}
	buf, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, fmt.Errorf("read export: %w", err)
	}
	if int64(len(buf)) > max {
		return nil, errors.New("export too large")
	}
	buf = bytes.TrimSpace(buf)
	if len(buf) == 0 {
		return &Export{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(buf))
	dec.UseNumber()
	var first json.RawMessage
	if err := dec.Decode(&first); err != nil {
		return nil, fmt.Errorf("read export: %w", err)
	}
	if dec.More() {
		return parseJSONL(buf)
	}
	first = bytes.TrimSpace(first)
	if len(first) > 0 && first[0] == '[' {
		return parseRowArray(first)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(first, &probe); err != nil {
		return nil, fmt.Errorf("read export: %w", err)
	}
	if _, ok := probe["agent_context_files"]; ok {
		return decodeEnvelope(first)
	}
	if _, ok := probe["user_context_files"]; ok {
		return decodeEnvelope(first)
	}
	if _, ok := probe["agent_id"]; !ok {
		if _, ok := probe["file_name"]; !ok {
			return decodeEnvelope(first)
		}
	}
	return oneRow(first)
}

func decodeEnvelope(raw []byte) (*Export, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var ex Export
	if err := dec.Decode(&ex); err != nil {
		return nil, fmt.Errorf("read export: %w", err)
	}
	return &ex, nil
}

func parseJSONL(buf []byte) (*Export, error) {
	sc := bufio.NewScanner(bytes.NewReader(buf))
	sc.Buffer(make([]byte, 64*1024), len(buf)+1)
	var ex Export
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := appendRow(&ex, line); err != nil {
			return nil, err
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read export: %w", err)
	}
	return &ex, nil
}

func parseRowArray(raw []byte) (*Export, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var rows []json.RawMessage
	if err := dec.Decode(&rows); err != nil {
		return nil, fmt.Errorf("read export: %w", err)
	}
	var ex Export
	for _, row := range rows {
		if err := appendRow(&ex, row); err != nil {
			return nil, err
		}
	}
	return &ex, nil
}

func oneRow(raw []byte) (*Export, error) {
	var ex Export
	if err := appendRow(&ex, raw); err != nil {
		return nil, err
	}
	return &ex, nil
}

func appendRow(ex *Export, raw []byte) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return fmt.Errorf("read export: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if _, ok := keys["user_id"]; ok {
		var row UserFile
		if err := dec.Decode(&row); err != nil {
			return fmt.Errorf("read export: %w", err)
		}
		ex.Users = append(ex.Users, row)
		return nil
	}
	var row AgentFile
	if err := dec.Decode(&row); err != nil {
		return fmt.Errorf("read export: %w", err)
	}
	ex.Agents = append(ex.Agents, row)
	return nil
}

// ReadAgentMap decodes a JSON object of GoClaw agent_id to goso agent_key.
func ReadAgentMap(r io.Reader) (map[string]string, error) {
	if r == nil {
		return nil, errors.New("read agent map")
	}
	buf, err := io.ReadAll(io.LimitReader(r, 1<<20+1))
	if err != nil {
		return nil, fmt.Errorf("read agent map: %w", err)
	}
	if len(buf) > 1<<20 {
		return nil, errors.New("agent map too large")
	}
	dec := json.NewDecoder(bytes.NewReader(buf))
	dec.UseNumber()
	var raw map[string]string
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("read agent map: %w", err)
	}
	out := make(map[string]string, len(raw))
	for id, key := range raw {
		id = strings.TrimSpace(id)
		key = strings.TrimSpace(key)
		if id == "" || key == "" || strings.ContainsAny(id, "\r\n\x00") || strings.ContainsAny(key, "\r\n\x00") {
			return nil, errors.New("read agent map")
		}
		out[id] = key
	}
	return out, nil
}

type item struct {
	kind     string
	scope    string
	tenant   string
	ownerKey string
	ownerID  string
	natural  string
	name     string
	body     string
	created  time.Time
	updated  time.Time
	reason   string
	keep     bool
}

// Import writes ex into st. A nil store or export, or a bad option, returns
// before any context-file write. A row rejection does not stop the run.
// A store failure while writing stops the run; rows already written stay.
func Import(st store.StoreIface, ex *Export, opt Options) (*Report, error) {
	ids, err := loadDenylist(opt.DenyFile)
	if err != nil {
		return nil, err
	}
	if n := deniedCount(ids, opt.SourceTenant, opt.DestTenant); n > 0 {
		return &Report{DeniedTenants: n, DryRun: !opt.Apply, Agents: []TenantSection{}, Users: []TenantSection{}}, &DenyError{Count: n}
	}
	if st == nil {
		return nil, errConfig("store is required")
	}
	if ex == nil {
		return nil, errConfig("export is required")
	}
	source := strings.TrimSpace(opt.SourceTenant)
	if source == "" || len(source) > 128 || strings.ContainsAny(source, "\r\n\x00") {
		return nil, errConfig("source tenant is required")
	}
	dest := strings.TrimSpace(opt.DestTenant)
	if !store.TenantOK(dest) {
		return nil, errConfig("tenant is required")
	}
	tally := &tally{log: opt.Log}
	items := make([]item, 0, len(ex.Agents)+len(ex.Users))
	for _, row := range ex.Agents {
		items = append(items, classifyAgent(st, dest, source, opt.AgentMap, row))
	}
	for _, row := range ex.Users {
		items = append(items, classifyUser(st, dest, source, opt.AgentMap, opt.Customers, row))
	}
	markNatural(items)
	markDest(items)
	maps, err := openMapper(st, opt.Apply)
	if err != nil {
		return tally.abort(!opt.Apply, errors.New("store"))
	}
	owners := map[string]*ownerFiles{}
	for i := range items {
		it := &items[i]
		if !it.keep {
			if it.reason == reasonDuplicate {
				tally.skip(it.kind, it.tenant, it.name, it.ownerKey, it.reason)
				continue
			}
			tally.reject(it.kind, it.tenant, it.name, it.ownerKey, it.reason)
			continue
		}
		key := it.scope + "\x00" + it.ownerID
		state := owners[key]
		if state == nil {
			var err error
			state, err = loadOwner(st, dest, it.scope, it.ownerID)
			if err != nil {
				return tally.abort(!opt.Apply, errors.New("store"))
			}
			owners[key] = state
		}
		prev, exists := state.files[it.name]
		mapped, hasMap, err := maps.get(dest, sourceTable(it.kind), it.natural)
		if err != nil {
			return tally.abort(!opt.Apply, errors.New("store"))
		}
		owned := hasMap && mapped.Scope == it.scope && mapped.Owner == it.ownerID && mapped.Name == it.name
		if exists && !owned {
			tally.skip(it.kind, it.tenant, it.name, it.ownerKey, reasonNativeExists)
			continue
		}
		if exists && owned && prev.body == it.body {
			tally.skip(it.kind, it.tenant, it.name, it.ownerKey, reasonUnchanged)
			continue
		}
		if !state.fits(it.name, it.body) {
			tally.reject(it.kind, it.tenant, it.name, it.ownerKey, reasonTooLarge)
			continue
		}
		if opt.Apply {
			saved, err := st.PutContextFile(store.ContextFile{
				TenantID:  dest,
				Scope:     it.scope,
				OwnerID:   it.ownerID,
				Name:      it.name,
				Body:      it.body,
				CreatedAt: it.created,
				UpdatedAt: it.updated,
			})
			if err != nil {
				if reason, ok := putReason(err); ok {
					tally.reject(it.kind, it.tenant, it.name, it.ownerKey, reason)
					continue
				}
				return tally.abort(false, errors.New("store"))
			}
			body := it.body
			if saved != nil && saved.Body != "" {
				body = saved.Body
			}
			if err := maps.put(mapRow{
				DestTenant:  dest,
				SourceTable: sourceTable(it.kind),
				SourceKey:   it.natural,
				Scope:       it.scope,
				Owner:       it.ownerID,
				Name:        it.name,
				Sum:         bodySum(body),
			}); err != nil {
				return tally.abort(false, errors.New("store"))
			}
		}
		_, existed := state.files[it.name]
		state.files[it.name] = keptFile{body: it.body, created: it.created, updated: it.updated}
		if existed {
			tally.add(it.kind, it.tenant, "updated", "", it.name, it.ownerKey)
			continue
		}
		tally.add(it.kind, it.tenant, "inserted", "", it.name, it.ownerKey)
	}
	rep, err := tally.finish(!opt.Apply, len(ex.Agents), len(ex.Users))
	if err != nil {
		return rep, err
	}
	if rep != nil {
		rep.DeniedTenants = 0
	}
	return rep, nil
}

func classifyAgent(st store.StoreIface, dest, source string, agentMap map[string]string, row AgentFile) item {
	it := item{
		kind:     "agent",
		scope:    store.ContextScopeAgent,
		tenant:   clipTenant(row.TenantID),
		ownerKey: clipOwner(row.AgentID),
		name:     strings.TrimSpace(row.FileName),
		body:     strings.TrimSpace(row.Content),
	}
	if !sameSource(row.TenantID, source) {
		it.reason = tenantReason(row.TenantID)
		return it
	}
	it.tenant = strings.TrimSpace(row.TenantID)
	created, updated, ok := rowClocks(row.CreatedAt, row.UpdatedAt)
	if !ok || !nameOK(it.name) || it.body == "" {
		it.reason = reasonInvalid
		return it
	}
	it.created = created
	it.updated = updated
	if len(it.body) > store.MaxContextFileBytes {
		it.reason = reasonTooLarge
		return it
	}
	id, reason := resolveAgent(st, dest, agentMap, row.AgentID)
	if reason != "" {
		it.reason = reason
		return it
	}
	it.ownerID = id
	it.natural = "agent|" + strings.TrimSpace(row.AgentID) + "|" + it.name
	it.keep = true
	return it
}

func classifyUser(st store.StoreIface, dest, source string, agentMap, customers map[string]string, row UserFile) item {
	it := item{
		kind:     "user",
		scope:    store.ContextScopeUser,
		tenant:   clipTenant(row.TenantID),
		ownerKey: clipOwner(row.UserID),
		name:     strings.TrimSpace(row.FileName),
		body:     strings.TrimSpace(row.Content),
	}
	if !sameSource(row.TenantID, source) {
		it.reason = tenantReason(row.TenantID)
		return it
	}
	it.tenant = strings.TrimSpace(row.TenantID)
	userID := strings.TrimSpace(row.UserID)
	created, updated, ok := rowClocks(row.CreatedAt, row.UpdatedAt)
	key := customerKey(customers, userID)
	if !ok || !nameOK(it.name) || it.body == "" || !store.TenantOK(userID) || !store.UserKeyOK(key) {
		it.reason = reasonInvalid
		return it
	}
	it.created = created
	it.updated = updated
	if len(it.body) > store.MaxContextFileBytes {
		it.reason = reasonTooLarge
		return it
	}
	if _, reason := resolveAgent(st, dest, agentMap, row.AgentID); reason != "" {
		it.reason = reason
		return it
	}
	it.ownerID = key
	it.ownerKey = userID
	it.natural = "user|" + strings.TrimSpace(row.AgentID) + "|" + userID + "|" + it.name
	it.keep = true
	return it
}

func rowClocks(created, updated string) (time.Time, time.Time, bool) {
	c, okC := parseStamp(created)
	u, okU := parseStamp(updated)
	if !okC || !okU {
		return time.Time{}, time.Time{}, false
	}
	return c, u, true
}

func parseStamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, true
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func clockMatches(stored, want time.Time) bool {
	if want.IsZero() {
		return true
	}
	return stored.UTC().Equal(want.UTC())
}

func tenantReason(tenant string) string {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" || len(tenant) > 128 || strings.ContainsAny(tenant, "\r\n\x00") {
		return reasonInvalid
	}
	return reasonTenantMismatch
}

func sameSource(row, source string) bool {
	row = strings.TrimSpace(row)
	if row == "" || len(row) > 128 || strings.ContainsAny(row, "\r\n\x00") {
		return false
	}
	return strings.EqualFold(row, strings.TrimSpace(source))
}

func clipTenant(tenant string) string {
	tenant = strings.TrimSpace(tenant)
	if len(tenant) > 128 || strings.ContainsAny(tenant, "\r\n\x00") {
		return ""
	}
	return tenant
}

func clipOwner(owner string) string {
	owner = strings.TrimSpace(owner)
	owner = strings.ReplaceAll(owner, "\n", " ")
	owner = strings.ReplaceAll(owner, "\r", " ")
	if len(owner) > 128 {
		return owner[:128]
	}
	return owner
}

// nameOK matches store.contextNameOK: one path segment, 1–64, no slash and no "..".
func nameOK(name string) bool {
	if name == "" || len(name) > 64 || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return false
	}
	for i, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		if i > 0 && (r == '.' || r == '_' || r == '-') {
			continue
		}
		return false
	}
	return true
}

func resolveAgent(st store.StoreIface, dest string, agentMap map[string]string, agentID string) (string, string) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || len(agentID) > 128 || strings.ContainsAny(agentID, "\r\n\x00") {
		return "", reasonInvalid
	}
	wantKey := ""
	if agentMap != nil {
		wantKey = strings.TrimSpace(agentMap[agentID])
	}
	other := false
	for _, a := range st.ListAgents() {
		if a == nil {
			continue
		}
		match := strings.TrimSpace(a.ID) == agentID || strings.TrimSpace(a.AgentKey) == agentID
		if wantKey != "" {
			match = strings.TrimSpace(a.AgentKey) == wantKey
		}
		if !match {
			continue
		}
		if store.SameTenant(a.TenantID, dest) {
			return a.ID, ""
		}
		other = true
	}
	if other {
		return "", reasonAgentOtherTenant
	}
	return "", reasonAgentMissing
}

func markNatural(items []item) {
	last := map[string]int{}
	for i := range items {
		if !items[i].keep {
			continue
		}
		if prev, ok := last[items[i].natural]; ok {
			items[prev].keep = false
			items[prev].reason = reasonDuplicate
		}
		last[items[i].natural] = i
	}
}

func markDest(items []item) {
	groups := map[string][]int{}
	var order []string
	for i := range items {
		if !items[i].keep {
			continue
		}
		k := items[i].scope + "\x00" + items[i].ownerID + "\x00" + items[i].name
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], i)
	}
	for _, k := range order {
		idxs := groups[k]
		body := items[idxs[0]].body
		same := true
		for _, i := range idxs[1:] {
			if items[i].body != body {
				same = false
				break
			}
		}
		if !same {
			for _, i := range idxs {
				items[i].keep = false
				items[i].reason = reasonScopeConflict
			}
			continue
		}
		for _, i := range idxs[:len(idxs)-1] {
			items[i].keep = false
			items[i].reason = reasonDuplicate
		}
	}
}

type keptFile struct {
	body    string
	created time.Time
	updated time.Time
}

type ownerFiles struct {
	files map[string]keptFile
}

func loadOwner(st store.StoreIface, tenant, scope, owner string) (*ownerFiles, error) {
	listed, err := st.ListContextFiles(tenant, scope, owner)
	if err != nil {
		return nil, err
	}
	files := map[string]keptFile{}
	for _, f := range listed {
		if f == nil {
			continue
		}
		files[f.Name] = keptFile{body: f.Body, created: f.CreatedAt, updated: f.UpdatedAt}
	}
	return &ownerFiles{files: files}, nil
}

func (o *ownerFiles) fits(name, body string) bool {
	next := make([]*store.ContextFile, 0, len(o.files)+1)
	seen := false
	for n, f := range o.files {
		b := f.body
		if n == name {
			seen = true
			b = body
		}
		next = append(next, &store.ContextFile{Name: n, Body: b})
	}
	if !seen {
		next = append(next, &store.ContextFile{Name: name, Body: body})
	}
	return store.ContextFilesOverCap(next) == nil
}

func putReason(err error) (string, bool) {
	switch {
	case errors.Is(err, store.ErrContextTooLarge), errors.Is(err, store.ErrContextOwnerCap):
		return reasonTooLarge, true
	case errors.Is(err, store.ErrContextInvalid):
		return reasonInvalid, true
	case errors.Is(err, store.ErrNotFound):
		return reasonAgentMissing, true
	default:
		return "", false
	}
}
