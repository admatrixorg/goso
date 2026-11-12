// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package contextimport loads one tenant from an offline context-file export
// into the context_files table from SPEC 150.
//
// The GoClaw export columns for agent_context_files and user_context_files
// are not listed in this repo or in goso#26, goso#28, or goso#29. The JSON
// shape below is an assumption. It carries only what context_files stores:
// a tenant id, an owner, a file name, and a body. Scope is which array the
// row sits in. agent_key is not a context_files column. It names an agent
// that already exists in the destination tenant; the stored owner is that
// agent's id. user_key is stored as the owner. The importer does not create
// agents, users, memories, vault rows, or a version table.
//
// Unknown JSON fields are ignored. A later run writes nothing when the
// stored body is already the body this run would store. A changed body
// replaces that same row. Rows that share a destination key collapse to the
// last valid row, so a repeated export does not oscillate.
package contextimport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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
	// Log receives rejection lines. Nil discards them. Arguments are names,
	// owner keys, and reason codes, never file bodies.
	Log Logger
}

// Logger is the rejection log. Printf must not be given a file body.
type Logger interface {
	Printf(format string, args ...any)
}

// Export is the assumed offline dump. Embedding and other unknown fields are
// not represented, so the decoder drops them.
type Export struct {
	Agents []AgentFile `json:"agent_context_files"`
	Users  []UserFile  `json:"user_context_files"`
}

// AgentFile is one assumed agent_context_files row.
type AgentFile struct {
	TenantID string `json:"tenant_id"`
	AgentKey string `json:"agent_key"`
	Name     string `json:"name"`
	Body     string `json:"body"`
}

// UserFile is one assumed user_context_files row.
type UserFile struct {
	TenantID string `json:"tenant_id"`
	UserKey  string `json:"user_key"`
	Name     string `json:"name"`
	Body     string `json:"body"`
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
	dec := json.NewDecoder(bytes.NewReader(buf))
	dec.UseNumber()
	var ex Export
	if err := dec.Decode(&ex); err != nil {
		return nil, fmt.Errorf("read export: %w", err)
	}
	return &ex, nil
}

type item struct {
	kind     string
	scope    string
	tenant   string
	ownerKey string
	ownerID  string
	name     string
	body     string
	reason   string
	keep     bool
}

// Import writes ex into st. A nil store or export, or a bad option, returns
// before any context-file write. A row rejection does not stop the run.
// A store failure while writing stops the run; rows already written stay.
func Import(st store.StoreIface, ex *Export, opt Options) (*Report, error) {
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
		items = append(items, classifyAgent(st, dest, source, row))
	}
	for _, row := range ex.Users {
		items = append(items, classifyUser(source, row))
	}
	markDuplicates(items)
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
		if prev, ok := state.files[it.name]; ok && prev == it.body {
			tally.skip(it.kind, it.tenant, it.name, it.ownerKey, reasonUnchanged)
			continue
		}
		if !state.fits(it.name, it.body) {
			tally.reject(it.kind, it.tenant, it.name, it.ownerKey, reasonTooLarge)
			continue
		}
		if opt.Apply {
			if _, err := st.PutContextFile(store.ContextFile{
				TenantID: dest,
				Scope:    it.scope,
				OwnerID:  it.ownerID,
				Name:     it.name,
				Body:     it.body,
			}); err != nil {
				if reason, ok := putReason(err); ok {
					tally.reject(it.kind, it.tenant, it.name, it.ownerKey, reason)
					continue
				}
				return tally.abort(false, errors.New("store"))
			}
		}
		_, existed := state.files[it.name]
		state.files[it.name] = it.body
		if existed {
			tally.add(it.kind, it.tenant, "updated", "", it.name, it.ownerKey)
			continue
		}
		tally.add(it.kind, it.tenant, "inserted", "", it.name, it.ownerKey)
	}
	return tally.finish(!opt.Apply, len(ex.Agents), len(ex.Users))
}

func classifyAgent(st store.StoreIface, dest, source string, row AgentFile) item {
	it := item{
		kind:     "agent",
		scope:    store.ContextScopeAgent,
		tenant:   clipTenant(row.TenantID),
		ownerKey: clipOwner(row.AgentKey),
		name:     strings.TrimSpace(row.Name),
		body:     strings.TrimSpace(row.Body),
	}
	if !sameSource(row.TenantID, source) {
		it.reason = tenantReason(row.TenantID)
		return it
	}
	it.tenant = strings.TrimSpace(row.TenantID)
	if !nameOK(it.name) || it.body == "" {
		it.reason = reasonInvalid
		return it
	}
	if len(it.body) > store.MaxContextFileBytes {
		it.reason = reasonTooLarge
		return it
	}
	id, reason := resolveAgent(st, dest, row.AgentKey)
	if reason != "" {
		it.reason = reason
		return it
	}
	it.ownerID = id
	it.keep = true
	return it
}

func classifyUser(source string, row UserFile) item {
	it := item{
		kind:     "user",
		scope:    store.ContextScopeUser,
		tenant:   clipTenant(row.TenantID),
		ownerKey: clipOwner(row.UserKey),
		name:     strings.TrimSpace(row.Name),
		body:     strings.TrimSpace(row.Body),
	}
	if !sameSource(row.TenantID, source) {
		it.reason = tenantReason(row.TenantID)
		return it
	}
	it.tenant = strings.TrimSpace(row.TenantID)
	if !nameOK(it.name) || it.body == "" || !store.UserKeyOK(strings.TrimSpace(row.UserKey)) {
		it.reason = reasonInvalid
		return it
	}
	if len(it.body) > store.MaxContextFileBytes {
		it.reason = reasonTooLarge
		return it
	}
	it.ownerID = strings.TrimSpace(row.UserKey)
	it.ownerKey = it.ownerID
	it.keep = true
	return it
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

func resolveAgent(st store.StoreIface, dest, key string) (string, string) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 128 || strings.ContainsAny(key, "\r\n\x00") {
		return "", reasonInvalid
	}
	other := false
	for _, a := range st.ListAgents() {
		if a == nil || strings.TrimSpace(a.AgentKey) != key {
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

func markDuplicates(items []item) {
	last := map[string]int{}
	for i := range items {
		if !items[i].keep {
			continue
		}
		k := items[i].scope + "\x00" + items[i].ownerID + "\x00" + items[i].name
		if prev, ok := last[k]; ok {
			items[prev].keep = false
			items[prev].reason = reasonDuplicate
		}
		last[k] = i
	}
}

type ownerFiles struct {
	files map[string]string
}

func loadOwner(st store.StoreIface, tenant, scope, owner string) (*ownerFiles, error) {
	listed, err := st.ListContextFiles(tenant, scope, owner)
	if err != nil {
		return nil, err
	}
	files := map[string]string{}
	for _, f := range listed {
		if f == nil {
			continue
		}
		files[f.Name] = f.Body
	}
	return &ownerFiles{files: files}, nil
}

func (o *ownerFiles) fits(name, body string) bool {
	next := make([]*store.ContextFile, 0, len(o.files)+1)
	seen := false
	for n, b := range o.files {
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
