// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package contactsimport loads one tenant from an offline export of
// public.channel_contacts into goso's existing contacts tables.
//
// There is no contacts table in that source. This package does not create
// one, and it does not add the source columns to goso. goso's natural key
// stays (tenant_id, channel, dest).
//
// The source natural key is (tenant_id, channel_type, sender_id,
// COALESCE(thread_id, ”)). It maps as:
//
//	channel = channel_type
//	dest    = "s" + len(sender_id) + ":" + sender_id + "t" + len(thread_id) + ":" + thread_id
//
// Lengths are bytes. An empty thread_id and a non-empty thread_id for the
// same sender are two dests, so a thread contact does not replace the
// person. The encoding is length-prefixed, so a sender_id that contains
// "t0:" does not collide with another pair. A dest over 128 bytes is
// invalid and is not clipped.
//
// contact_type, peer_kind, and thread_type are kept only as goso's kind.
// kind is group when any of those three is "group". Otherwise contact_type
// must be "user", "direct", or empty, and kind is user. An unknown
// contact_type is invalid. An unknown peer_kind does not change kind.
//
// display is display_name, then username, then channel:dest. user_id is not
// agent_id. channel_instance, avatar_url, and metadata are not stored.
// seen_count is not a source column: a new row stores 1, and an update keeps
// the stored count and agent_id. first_seen_at and last_seen_at are stored
// as first_seen and last_seen.
//
// A non-empty merged_id means the source row was already merged into
// another contact. The row is skipped and is not inserted. goso merge
// history is not copied. merged_id equal to the row id is invalid.
//
// The file is JSON {"channel_contacts":[...]} or JSONL of one object per
// line, using these column names. --tenant is the goso tenant that is
// written. --source-tenant, when set, keeps only rows with that source
// tenant_id. The report is counts and reason codes.
package contactsimport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
	// Source column limits from public.channel_contacts. A longer value is
	// not a row that column could hold.
	maxSenderLen = 255
	maxThreadLen = 100
)

// maxExportBytes is the file cap. A larger file is refused before decode.
var maxExportBytes = 64 << 20

// ErrExportTooLarge is returned when the file is over maxExportBytes.
var ErrExportTooLarge = errors.New("export is too large")

// Store is the durable contact directory. SQLiteStore and PostgresStore both
// implement it. The in-memory store does not.
type Store interface {
	ListContacts(tenant, q, channel, kind string) ([]store.ContactRow, error)
	BulkUpsertContacts(seeds []store.ContactSeed) (store.ContactBulkResult, error)
}

// Options is one run. Tenant is the goso tenant written on every kept row.
// SourceTenant, when set, is the channel_contacts.tenant_id to keep.
type Options struct {
	Tenant       string
	SourceTenant string
	DryRun       bool
}

// Report is counts only. Reasons for invalid rows sum to Invalid. The
// reason code "merged" counts skipped rows whose merged_id is set, and is
// not part of Invalid. New + Updated + Skipped + Invalid equals the input.
type Report struct {
	New     int      `json:"new"`
	Updated int      `json:"updated"`
	Skipped int      `json:"skipped"`
	Invalid int      `json:"invalid"`
	DryRun  bool     `json:"dry_run"`
	Reasons []Reason `json:"reasons,omitempty"`
}

// Reason is how many rows share one code. Code is a fixed token.
type Reason struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

// Contact is one channel_contacts object. Unknown JSON keys are dropped.
type Contact struct {
	ID              string `json:"id,omitempty"`
	TenantID        string `json:"tenant_id,omitempty"`
	ChannelType     string `json:"channel_type"`
	ChannelInstance string `json:"channel_instance,omitempty"`
	SenderID        string `json:"sender_id"`
	UserID          string `json:"user_id,omitempty"`
	DisplayName     string `json:"display_name,omitempty"`
	Username        string `json:"username,omitempty"`
	AvatarURL       string `json:"avatar_url,omitempty"`
	PeerKind        string `json:"peer_kind,omitempty"`
	MergedID        string `json:"merged_id,omitempty"`
	FirstSeenAt     string `json:"first_seen_at"`
	LastSeenAt      string `json:"last_seen_at"`
	ContactType     string `json:"contact_type,omitempty"`
	ThreadID        string `json:"thread_id,omitempty"`
	ThreadType      string `json:"thread_type,omitempty"`
}

// File is a JSON export of channel_contacts.
type File struct {
	Contacts []Contact `json:"channel_contacts"`
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

// ReadExport decodes a channel_contacts JSON document or a JSONL file.
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
	trim := bytes.TrimSpace(body)
	if len(trim) == 0 {
		return nil, errors.New("read export")
	}
	if bytes.HasPrefix(trim, []byte("{")) {
		dec := json.NewDecoder(bytes.NewReader(body))
		var probe map[string]json.RawMessage
		if err := dec.Decode(&probe); err != nil {
			if bytes.Contains(trim, []byte(`"channel_contacts"`)) {
				return nil, errors.New("read export")
			}
			return readJSONL(body)
		}
		raw, ok := probe["channel_contacts"]
		if !ok {
			return readJSONL(body)
		}
		if dec.More() {
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
	return readJSONL(body)
}

func readJSONL(body []byte) (*Export, error) {
	lines := bytes.Split(body, []byte("\n"))
	rows := make([]json.RawMessage, 0, len(lines))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		rows = append(rows, append(json.RawMessage(nil), line...))
	}
	if len(rows) == 0 {
		return nil, errors.New("read export")
	}
	return &Export{rows: rows}, nil
}

type rowJSON struct {
	ID              flexString      `json:"id"`
	TenantID        flexString      `json:"tenant_id"`
	ChannelType     flexString      `json:"channel_type"`
	ChannelInstance flexString      `json:"channel_instance"`
	SenderID        flexString      `json:"sender_id"`
	UserID          flexString      `json:"user_id"`
	DisplayName     flexString      `json:"display_name"`
	Username        flexString      `json:"username"`
	AvatarURL       flexString      `json:"avatar_url"`
	PeerKind        flexString      `json:"peer_kind"`
	Metadata        json.RawMessage `json:"metadata"`
	MergedID        flexString      `json:"merged_id"`
	FirstSeenAt     flexString      `json:"first_seen_at"`
	LastSeenAt      flexString      `json:"last_seen_at"`
	ContactType     flexString      `json:"contact_type"`
	ThreadID        flexString      `json:"thread_id"`
	ThreadType      flexString      `json:"thread_type"`
}

// flexString accepts a JSON string or null. Any other JSON type marks the
// row invalid without failing the rest of the file.
type flexString struct {
	Value string
	Bad   bool
}

func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &f.Value); err != nil {
			f.Bad = true
		}
		return nil
	}
	f.Bad = true
	return nil
}

func (r rowJSON) bad() bool {
	return r.ID.Bad || r.TenantID.Bad || r.ChannelType.Bad || r.ChannelInstance.Bad || r.SenderID.Bad || r.UserID.Bad || r.DisplayName.Bad || r.Username.Bad || r.AvatarURL.Bad || r.PeerKind.Bad || r.MergedID.Bad || r.FirstSeenAt.Bad || r.LastSeenAt.Bad || r.ContactType.Bad || r.ThreadID.Bad || r.ThreadType.Bad
}

// Import lists the destination tenant once, then upserts every new and
// updated row in one store call. A merged source row is skipped. DryRun
// does not call the upsert.
func Import(db Store, ex *Export, opt Options) (Report, error) {
	tenant := strings.TrimSpace(opt.Tenant)
	if tenant == "" {
		return Report{}, errConfig("tenant is required")
	}
	if !store.TenantOK(tenant) {
		return Report{}, errConfig("tenant is invalid")
	}
	source := strings.TrimSpace(opt.SourceTenant)
	if hasControl(source) {
		return Report{}, errConfig("source tenant is invalid")
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
		seed, reason, merged := accept(raw, tenant, source)
		if merged {
			rep.Skipped++
			reasons["merged"]++
			continue
		}
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
			// The export has no seen_count and no agent. Keep the stored
			// values so a rerun does not wipe them.
			seed.Count = prev.Count
			seed.AgentID = prev.AgentID
			if sameContact(prev, seed) {
				rep.Skipped++
				continue
			}
			rep.Updated++
			upserts = append(upserts, seed)
			continue
		}
		seed.Count = 1
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

// accept returns merged=true when the source row was already merged away.
func accept(raw json.RawMessage, destTenant, sourceTenant string) (store.ContactSeed, string, bool) {
	var row rowJSON
	if err := json.Unmarshal(raw, &row); err != nil || row.bad() {
		return store.ContactSeed{}, "row", false
	}
	if hasControl(row.TenantID.Value) || hasControl(row.ChannelType.Value) || hasControl(row.SenderID.Value) || hasControl(row.ThreadID.Value) || hasControl(row.DisplayName.Value) || hasControl(row.Username.Value) || hasControl(row.ContactType.Value) || hasControl(row.PeerKind.Value) || hasControl(row.ThreadType.Value) || hasControl(row.MergedID.Value) || hasControl(row.ID.Value) {
		return store.ContactSeed{}, "control", false
	}
	if reason := tenantReason(row.TenantID.Value, destTenant, sourceTenant); reason != "" {
		return store.ContactSeed{}, reason, false
	}
	mergedID := strings.TrimSpace(row.MergedID.Value)
	rowID := strings.TrimSpace(row.ID.Value)
	if mergedID != "" {
		if rowID != "" && mergedID == rowID {
			return store.ContactSeed{}, "merged-self", false
		}
		return store.ContactSeed{}, "merged", true
	}
	channel := strings.TrimSpace(row.ChannelType.Value)
	sender := strings.TrimSpace(row.SenderID.Value)
	thread := strings.TrimSpace(row.ThreadID.Value)
	if channel == "" {
		return store.ContactSeed{}, "empty-channel", false
	}
	if len(channel) > maxChannelLen {
		return store.ContactSeed{}, "channel-length", false
	}
	if sender == "" {
		return store.ContactSeed{}, "empty-sender", false
	}
	if len(sender) > maxSenderLen {
		return store.ContactSeed{}, "sender-length", false
	}
	if len(thread) > maxThreadLen {
		return store.ContactSeed{}, "thread-length", false
	}
	dest := encodeDest(sender, thread)
	if len(dest) > maxDestLen {
		return store.ContactSeed{}, "dest-length", false
	}
	display := strings.TrimSpace(row.DisplayName.Value)
	if display == "" {
		display = strings.TrimSpace(row.Username.Value)
	}
	if len(display) > maxDisplayLen {
		return store.ContactSeed{}, "display-length", false
	}
	kind, ok := kindOf(row.ContactType.Value, row.PeerKind.Value, row.ThreadType.Value)
	if !ok {
		return store.ContactSeed{}, "kind", false
	}
	first, okFirst := parseStamp(row.FirstSeenAt.Value)
	last, okLast := parseStamp(row.LastSeenAt.Value)
	if !okFirst || !okLast {
		return store.ContactSeed{}, "time", false
	}
	if display == "" {
		display = channel + ":" + dest
	}
	return store.ContactSeed{
		TenantID:  destTenant,
		Display:   display,
		Kind:      kind,
		Channel:   channel,
		Dest:      dest,
		FirstSeen: first,
		LastSeen:  last,
	}, "", false
}

// encodeDest is the goso dest for one source key. sender and thread are
// already trimmed. len is the byte length.
func encodeDest(sender, thread string) string {
	return fmt.Sprintf("s%d:%st%d:%s", len(sender), sender, len(thread), thread)
}

func tenantReason(rowTenant, destTenant, sourceTenant string) string {
	rowTenant = strings.TrimSpace(rowTenant)
	if sourceTenant != "" {
		if rowTenant != sourceTenant {
			return "foreign-tenant"
		}
		return ""
	}
	if rowTenant == "" {
		return ""
	}
	if !store.TenantOK(rowTenant) {
		return "tenant"
	}
	if !store.SameTenant(rowTenant, destTenant) {
		return "foreign-tenant"
	}
	return ""
}

func kindOf(contactType, peerKind, threadType string) (string, bool) {
	ct := strings.ToLower(strings.TrimSpace(contactType))
	pk := strings.ToLower(strings.TrimSpace(peerKind))
	tt := strings.ToLower(strings.TrimSpace(threadType))
	if ct == "group" || pk == "group" || tt == "group" {
		return "group", true
	}
	switch ct {
	case "", "user", "direct":
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
	invalidSum := 0
	merged := 0
	for _, r := range rep.Reasons {
		if r.Code == "merged" {
			merged += r.Count
			continue
		}
		invalidSum += r.Count
	}
	if invalidSum != rep.Invalid || merged > rep.Skipped {
		return errors.New("import tally failed")
	}
	return nil
}
