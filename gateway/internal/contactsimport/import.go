// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package contactsimport loads one tenant from an offline contacts file into
// goso's existing contacts and contact_identifiers tables.
//
// The GoClaw contacts and channel_contacts columns are not recorded in this
// repo. The file below is the assumption this package reads. Its fields are
// the goso contact seed and nothing else. Phone, email, notes, tokens, and
// message text are not fields. Unknown JSON keys are ignored and are not
// stored. This package does not create a contacts table and does not copy
// another product's schema.
//
//	{
//	  "contacts": [
//	    {
//	      "tenant_id": "minh-an",
//	      "channel": "telegram",
//	      "dest": "ext-minh-an-0001",
//	      "display": "Minh An 0001",
//	      "kind": "user",
//	      "agent_id": "minh-an-sales",
//	      "seen_count": 2,
//	      "first_seen": "2026-01-15T08:00:00Z",
//	      "last_seen": "2026-01-15T09:00:00Z"
//	    }
//	  ]
//	}
//
// The natural key is (tenant_id, channel, dest), the primary key of
// contact_identifiers. A second run updates that row or skips it. It does
// not insert another contact.
//
// tenant_id on a row is optional. Empty means the row belongs to the run's
// tenant. A non-empty id that is not that tenant is foreign-tenant and is
// not written. One run writes one tenant.
//
// kind accepts user, direct, group, or empty. direct is stored as user.
// Empty uses the directory rule: a dest that starts with "-" is group, and
// any other dest is user. Any other kind is invalid.
//
// seen_count omitted, null, or 0 is stored as 1. A negative count is invalid.
// first_seen and last_seen are required RFC3339 timestamps.
//
// Channel longer than 64 bytes, or dest longer than 128, is invalid. Display
// and agent_id longer than 256 bytes are invalid. The directory would clip
// those strings. This importer does not, so two dests cannot collapse into
// one key.
//
// The report is counts and reason codes. It does not include a display, a
// dest, a channel, or an agent id. DryRun lists the tenant and does not call
// the upsert. Opening the database still runs the existing migrations.
//
// GOSO_CONTACTS_DURABLE is not read or changed. Memory, vault, and
// knowledge-graph rows are not written.
package contactsimport

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	// Directory limits in gateway/internal/store/sqlite_contacts.go.
	maxChannelLen = 64
	maxDestLen    = 128
	maxDisplayLen = 256
	maxAgentLen   = 256
)

// maxExportBytes is the file cap. A larger file is refused before decode
// so a dump of notes cannot be loaded into memory without a bound.
var maxExportBytes = 64 << 20

// ErrExportTooLarge is returned when the file is over maxExportBytes.
var ErrExportTooLarge = errors.New("export is too large")

// Store is the durable contact directory. SQLiteStore and PostgresStore both
// implement it. The in-memory store does not.
type Store interface {
	ListContacts(tenant, q, channel, kind string) ([]store.ContactRow, error)
	BulkUpsertContacts(seeds []store.ContactSeed) (store.ContactBulkResult, error)
}

// Options is one run. Tenant is the only tenant that can be written.
type Options struct {
	Tenant string
	DryRun bool
}

// Report is counts only. Reasons sums to Invalid. New + Updated + Skipped +
// Invalid equals the number of input rows.
type Report struct {
	New     int      `json:"new"`
	Updated int      `json:"updated"`
	Skipped int      `json:"skipped"`
	Invalid int      `json:"invalid"`
	DryRun  bool     `json:"dry_run"`
	Reasons []Reason `json:"reasons,omitempty"`
}

// Reason is how many rows failed one code. Code is a fixed token.
type Reason struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

// Contact is one documented input object. Unknown JSON keys are dropped.
type Contact struct {
	TenantID  string `json:"tenant_id,omitempty"`
	Channel   string `json:"channel"`
	Dest      string `json:"dest"`
	Display   string `json:"display,omitempty"`
	Kind      string `json:"kind,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	SeenCount int    `json:"seen_count"`
	FirstSeen string `json:"first_seen"`
	LastSeen  string `json:"last_seen"`
}

// File is the documented top-level object.
type File struct {
	Contacts []Contact `json:"contacts"`
}

// Export is a decoded file. Rows stay raw so one bad object does not reject
// the rest of the file.
type Export struct {
	rows []json.RawMessage
}

// ConfigError is a bad option. The text names the option, not a contact.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func errConfig(msg string) error { return &ConfigError{Msg: msg} }

// ReadExport decodes a contacts file from r.
func ReadExport(r io.Reader) (*Export, error) {
	if r == nil {
		return nil, errors.New("read export")
	}
	body, err := io.ReadAll(io.LimitReader(r, int64(maxExportBytes)+1))
	if err != nil {
		return nil, errors.New("read export")
	}
	if len(body) > maxExportBytes {
		return nil, ErrExportTooLarge
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	var probe map[string]json.RawMessage
	if err := dec.Decode(&probe); err != nil {
		return nil, errors.New("read export")
	}
	if dec.More() {
		return nil, errors.New("read export")
	}
	raw, ok := probe["contacts"]
	if !ok {
		return nil, errors.New("read export")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return &Export{}, nil
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, errors.New("read export")
	}
	return &Export{rows: rows}, nil
}

type rowJSON struct {
	TenantID  string   `json:"tenant_id"`
	Channel   string   `json:"channel"`
	Dest      string   `json:"dest"`
	Display   string   `json:"display"`
	Kind      string   `json:"kind"`
	AgentID   string   `json:"agent_id"`
	SeenCount intField `json:"seen_count"`
	FirstSeen string   `json:"first_seen"`
	LastSeen  string   `json:"last_seen"`
}

// intField accepts one JSON integer. A missing field is not present. Any
// other JSON type marks the row invalid without failing the rest of the file.
type intField struct {
	Present bool
	Value   int
	Bad     bool
}

func (f *intField) UnmarshalJSON(b []byte) error {
	f.Present = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(b, &f.Value); err != nil {
		f.Bad = true
	}
	return nil
}

// Import classifies every row, then upserts the new and updated rows in one
// store call. DryRun does not call the upsert. Invalid rows are not written.
// A row that matches the stored fields is skipped and is not written again.
func Import(db Store, ex *Export, opt Options) (Report, error) {
	tenant := strings.TrimSpace(opt.Tenant)
	if tenant == "" {
		return Report{}, errConfig("tenant is required")
	}
	if !store.TenantOK(tenant) {
		return Report{}, errConfig("tenant is invalid")
	}
	if ex == nil {
		return Report{}, errConfig("export is required")
	}
	if db == nil {
		return Report{}, errConfig("contact store is required")
	}
	have, err := db.ListContacts(tenant, "", "", "")
	if err != nil {
		return Report{}, errors.New("list contacts failed")
	}
	existing := map[string]store.ContactRow{}
	for _, row := range have {
		if !store.SameTenant(row.TenantID, tenant) {
			continue
		}
		if len(row.Identifiers) == 0 {
			existing[key(row.Channel, row.Dest)] = row
			continue
		}
		for _, id := range row.Identifiers {
			existing[key(id.Channel, id.Dest)] = row
		}
	}
	rep := Report{DryRun: opt.DryRun}
	reasons := map[string]int{}
	seen := map[string]struct{}{}
	upserts := make([]store.ContactSeed, 0)
	for _, raw := range ex.rows {
		seed, reason := accept(raw, tenant)
		if reason != "" {
			rep.Invalid++
			reasons[reason]++
			continue
		}
		k := key(seed.Channel, seed.Dest)
		if _, ok := seen[k]; ok {
			rep.Invalid++
			reasons["duplicate"]++
			continue
		}
		seen[k] = struct{}{}
		if prev, ok := existing[k]; ok {
			if sameContact(prev, seed) {
				rep.Skipped++
				continue
			}
			rep.Updated++
			upserts = append(upserts, seed)
			continue
		}
		rep.New++
		upserts = append(upserts, seed)
	}
	rep.Reasons = reasonList(reasons)
	if err := balanced(rep, len(ex.rows)); err != nil {
		return Report{}, err
	}
	if opt.DryRun || len(upserts) == 0 {
		return rep, nil
	}
	if _, err := db.BulkUpsertContacts(upserts); err != nil {
		return Report{}, errors.New("upsert contacts failed")
	}
	return rep, nil
}

func accept(raw json.RawMessage, tenant string) (store.ContactSeed, string) {
	var row rowJSON
	if err := json.Unmarshal(raw, &row); err != nil {
		return store.ContactSeed{}, "row"
	}
	if hasControl(row.TenantID) || hasControl(row.Channel) || hasControl(row.Dest) || hasControl(row.Display) || hasControl(row.Kind) || hasControl(row.AgentID) {
		return store.ContactSeed{}, "control"
	}
	rowTenant := strings.TrimSpace(row.TenantID)
	if rowTenant != "" {
		if !store.TenantOK(rowTenant) {
			return store.ContactSeed{}, "tenant"
		}
		if !store.SameTenant(rowTenant, tenant) {
			return store.ContactSeed{}, "foreign-tenant"
		}
	}
	channel := strings.TrimSpace(row.Channel)
	dest := strings.TrimSpace(row.Dest)
	if channel == "" {
		return store.ContactSeed{}, "empty-channel"
	}
	if dest == "" {
		return store.ContactSeed{}, "empty-dest"
	}
	if len(channel) > maxChannelLen {
		return store.ContactSeed{}, "channel-length"
	}
	if len(dest) > maxDestLen {
		return store.ContactSeed{}, "dest-length"
	}
	display := strings.TrimSpace(row.Display)
	if len(display) > maxDisplayLen {
		return store.ContactSeed{}, "display-length"
	}
	agent := strings.TrimSpace(row.AgentID)
	if len(agent) > maxAgentLen {
		return store.ContactSeed{}, "agent-length"
	}
	kind, ok := kindOf(dest, row.Kind)
	if !ok {
		return store.ContactSeed{}, "kind"
	}
	if row.SeenCount.Bad || row.SeenCount.Value < 0 {
		return store.ContactSeed{}, "count"
	}
	count := row.SeenCount.Value
	if !row.SeenCount.Present || count < 1 {
		count = 1
	}
	first, okFirst := parseStamp(row.FirstSeen)
	last, okLast := parseStamp(row.LastSeen)
	if !okFirst || !okLast {
		return store.ContactSeed{}, "time"
	}
	if display == "" {
		display = channel + ":" + dest
	}
	return store.ContactSeed{
		TenantID:  tenant,
		Display:   display,
		Kind:      kind,
		Channel:   channel,
		Dest:      dest,
		AgentID:   agent,
		Count:     count,
		FirstSeen: first,
		LastSeen:  last,
	}, ""
}

func kindOf(dest, hint string) (string, bool) {
	h := strings.ToLower(strings.TrimSpace(hint))
	switch h {
	case "group":
		return "group", true
	case "direct", "user":
		return "user", true
	case "":
		if strings.HasPrefix(dest, "-") {
			return "group", true
		}
		return "user", true
	default:
		return "", false
	}
}

func parseStamp(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func key(channel, dest string) string {
	return channel + "\x00" + dest
}

func sameContact(row store.ContactRow, seed store.ContactSeed) bool {
	return row.Display == seed.Display &&
		row.Kind == seed.Kind &&
		row.AgentID == seed.AgentID &&
		row.Count == seed.Count &&
		row.FirstSeen.Equal(seed.FirstSeen) &&
		row.LastSeen.Equal(seed.LastSeen)
}

func reasonList(m map[string]int) []Reason {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Reason, 0, len(keys))
	for _, k := range keys {
		out = append(out, Reason{Code: k, Count: m[k]})
	}
	return out
}

func balanced(rep Report, n int) error {
	if rep.New+rep.Updated+rep.Skipped+rep.Invalid != n {
		return errors.New("import tally failed")
	}
	sum := 0
	for _, r := range rep.Reasons {
		sum += r.Count
	}
	if sum != rep.Invalid {
		return errors.New("import tally failed")
	}
	return nil
}
