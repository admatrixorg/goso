// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package contactsimport loads one tenant from an offline export of
// public.channel_contacts into goso's existing contacts tables.
//
// There is no contacts table in that source. This package does not create
// one, and it does not add the source columns to goso. goso's natural key
// stays (tenant_id, channel, dest).
//
// The source natural key is (tenant_id, channel_type, sender_id, thread_id).
// channel is the goso name from a fixed rename table. dest is the raw id
// inbound already uses: sender_id for the person, and thread_id too when
// it differs. A later ObserveContact of that sender returns this row.
//
// A local customer-map file records channel, sender_id, and user_id to
// customer_key "<channel>:<sender_id>", or "goclaw:<user_id>" when the
// sender is empty. The key is never empty. The file is not a goso table.
//
// contact_type, peer_kind, and thread_type are kept only as goso's kind.
// kind is group when any of those three is "group". Otherwise contact_type
// must be "user", "direct", or empty, and kind is user. An unknown
// contact_type is invalid. An unknown peer_kind does not change kind.
//
// display is display_name, then username, then channel:dest.
// channel_instance, avatar_url, and metadata are not stored. user_id is
// not agent_id; it is kept only in the local customer-map file.
// seen_count is not a source column. A new row stores 1. An existing row
// is skipped and is not changed. first_seen_at and last_seen_at are stored
// as first_seen and last_seen on insert.
//
// A non-empty merged_id means the source row was already merged into
// another contact. The row is skipped and is not inserted. goso merge
// history is not copied. merged_id equal to the row id is invalid.
//
// The file is JSON {"channel_contacts":[...]} or JSONL of one object per
// line, using these column names. --tenant is the goso tenant that is
// written. --source-tenant is required. A deny list stops the run before
// any contact write. The report is counts and reason codes.
package contactsimport

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
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
	PutContactImport(tenant, contactID, channel, dest string) error
	RollbackContactImport(tenant string) (int, error)
	ObserveContact(in store.ContactSighting) (store.ContactRow, error)
}

// Options is one run. Tenant is the goso tenant written on every kept row.
// SourceTenant is required and is the channel_contacts.tenant_id to keep.
// Deny lists tenant ids that must not be written. CustomerMap, when set
// and the run is not a dry-run, is a local file of customer keys.
type Options struct {
	Tenant       string
	SourceTenant string
	DryRun       bool
	Deny         map[string]struct{}
	CustomerMap  string
}

// Report is counts only. Reasons other than merged and native-exists sum
// to Invalid. Those two codes are skipped rows. Updated stays 0: an
// existing contact is not changed. New + Updated + Skipped + Invalid
// equals the input.
type Report struct {
	New     int      `json:"new"`
	Updated int      `json:"updated"`
	Skipped int      `json:"skipped"`
	Invalid int      `json:"invalid"`
	DryRun  bool     `json:"dry_run"`
	Denied  int      `json:"denied"`
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

// Import lists the destination tenant once, then inserts new rows in one
// store call. An existing contact is skipped. A merged source row is
// skipped. DryRun does not call the upsert and does not write the map file.
func Import(db Store, ex *Export, opt Options) (Report, error) {
	tenant := strings.TrimSpace(opt.Tenant)
	if tenant == "" {
		return Report{}, errConfig("tenant is required")
	}
	if !store.TenantOK(tenant) {
		return Report{}, errConfig("tenant is invalid")
	}
	source := strings.TrimSpace(opt.SourceTenant)
	if source == "" {
		return Report{}, errConfig("source tenant is required")
	}
	if hasControl(source) {
		return Report{}, errConfig("source tenant is invalid")
	}
	if len(opt.Deny) == 0 {
		return Report{}, errConfig("deny file is empty")
	}
	if n := DeniedCount(opt.Deny, tenant, source); n > 0 {
		return Report{Denied: n, DryRun: opt.DryRun}, errConfig("tenant is denied")
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
	inserted := make([][2]string, 0)
	mapLines := make([]mapLine, 0)
	for _, raw := range ex.rows {
		plan, reason, merged := accept(raw, tenant, source)
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
		mapLines = append(mapLines, plan.line)
		fresh := make([]store.ContactSeed, 0, len(plan.seeds))
		existed := 0
		dup := 0
		for _, seed := range plan.seeds {
			k := key(seed.Channel, seed.Dest)
			if _, ok := seen[k]; ok {
				dup++
				continue
			}
			if _, ok := existing[k]; ok {
				existed++
				seen[k] = struct{}{}
				continue
			}
			seen[k] = struct{}{}
			seed.Count = 1
			fresh = append(fresh, seed)
		}
		if len(fresh) == 0 {
			rep.Skipped++
			if existed > 0 && dup == 0 {
				reasons["native-exists"]++
			} else if dup > 0 && existed == 0 {
				reasons["duplicate"]++
				rep.Skipped--
				rep.Invalid++
			} else if existed > 0 {
				reasons["native-exists"]++
			} else {
				reasons["duplicate"]++
				rep.Skipped--
				rep.Invalid++
			}
			continue
		}
		rep.New++
		for _, seed := range fresh {
			upserts = append(upserts, seed)
			inserted = append(inserted, [2]string{seed.Channel, seed.Dest})
		}
	}
	rep.Reasons = reasonList(reasons)
	if err := balanced(rep, len(ex.rows)); err != nil {
		return Report{}, err
	}
	if opt.DryRun {
		return rep, nil
	}
	if len(upserts) > 0 {
		if _, err := db.BulkUpsertContacts(upserts); err != nil {
			return Report{}, errors.New("upsert contacts failed")
		}
		if err := recordInserted(db, tenant, inserted); err != nil {
			return Report{}, err
		}
	}
	if err := writeCustomerMap(opt.CustomerMap, mapLines); err != nil {
		return Report{}, err
	}
	return rep, nil
}

type rowPlan struct {
	seeds []store.ContactSeed
	line  mapLine
}

type mapLine struct {
	channel string
	sender  string
	user    string
	key     string
}

// accept returns merged=true when the source row was already merged away.
func accept(raw json.RawMessage, destTenant, sourceTenant string) (rowPlan, string, bool) {
	var row rowJSON
	if err := json.Unmarshal(raw, &row); err != nil || row.bad() {
		return rowPlan{}, "row", false
	}
	if hasControl(row.TenantID.Value) || hasControl(row.ChannelType.Value) || hasControl(row.SenderID.Value) || hasControl(row.ThreadID.Value) || hasControl(row.DisplayName.Value) || hasControl(row.Username.Value) || hasControl(row.ContactType.Value) || hasControl(row.PeerKind.Value) || hasControl(row.ThreadType.Value) || hasControl(row.MergedID.Value) || hasControl(row.ID.Value) || hasControl(row.UserID.Value) {
		return rowPlan{}, "control", false
	}
	if reason := tenantReason(row.TenantID.Value, destTenant, sourceTenant); reason != "" {
		return rowPlan{}, reason, false
	}
	mergedID := strings.TrimSpace(row.MergedID.Value)
	rowID := strings.TrimSpace(row.ID.Value)
	if mergedID != "" {
		if rowID != "" && mergedID == rowID {
			return rowPlan{}, "merged-self", false
		}
		return rowPlan{}, "merged", true
	}
	rawChannel := strings.TrimSpace(row.ChannelType.Value)
	sender := strings.TrimSpace(row.SenderID.Value)
	thread := strings.TrimSpace(row.ThreadID.Value)
	user := strings.TrimSpace(row.UserID.Value)
	if rawChannel == "" {
		return rowPlan{}, "empty-channel", false
	}
	channel, ok := gosoChannel(rawChannel)
	if !ok {
		return rowPlan{}, "channel", false
	}
	if sender == "" {
		return rowPlan{}, "empty-sender", false
	}
	if len(sender) > maxSenderLen || len(thread) > maxThreadLen {
		return rowPlan{}, "sender-length", false
	}
	dests := rawDests(sender, thread)
	for _, dest := range dests {
		if len(dest) > maxDestLen {
			return rowPlan{}, "dest-length", false
		}
	}
	display := strings.TrimSpace(row.DisplayName.Value)
	if display == "" {
		display = strings.TrimSpace(row.Username.Value)
	}
	if len(display) > maxDisplayLen {
		return rowPlan{}, "display-length", false
	}
	kind, ok := kindOf(row.ContactType.Value, row.PeerKind.Value, row.ThreadType.Value)
	if !ok {
		return rowPlan{}, "kind", false
	}
	first, okFirst := parseStamp(row.FirstSeenAt.Value)
	last, okLast := parseStamp(row.LastSeenAt.Value)
	if !okFirst || !okLast {
		return rowPlan{}, "time", false
	}
	ck, ok := customerKey(channel, sender, user)
	if !ok {
		return rowPlan{}, "identity", false
	}
	seeds := make([]store.ContactSeed, 0, len(dests))
	for _, dest := range dests {
		d := display
		if d == "" {
			d = channel + ":" + dest
		}
		k := kind
		if dest == thread && thread != sender && (strings.EqualFold(strings.TrimSpace(row.ThreadType.Value), "group") || strings.EqualFold(strings.TrimSpace(row.ContactType.Value), "group")) {
			k = "group"
		}
		if dest == sender && thread != "" && thread != sender && strings.EqualFold(strings.TrimSpace(row.ContactType.Value), "user") {
			k = "user"
		}
		seeds = append(seeds, store.ContactSeed{
			TenantID:  destTenant,
			Display:   d,
			Kind:      k,
			Channel:   channel,
			Dest:      dest,
			FirstSeen: first,
			LastSeen:  last,
		})
	}
	return rowPlan{
		seeds: seeds,
		line:  mapLine{channel: channel, sender: sender, user: user, key: ck},
	}, "", false
}

// gosoChannel maps a source channel_type onto goso's catalog name.
// Unknown names are rejected so a later inbound event uses the same channel.
func gosoChannel(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "telegram":
		return "telegram", true
	case "zalo_personal", "zalo-personal":
		return "zalo-personal", true
	case "zalo_oa", "zalo-oa":
		return "zalo-oa", true
	case "discord":
		return "discord", true
	case "slack":
		return "slack", true
	case "feishu", "lark":
		return "feishu", true
	case "whatsapp":
		return "whatsapp", true
	default:
		return "", false
	}
}

// rawDests is the inbound key: the sender id, plus the thread id when it
// names a different chat. Nothing is length-prefixed.
func rawDests(sender, thread string) []string {
	out := []string{sender}
	if thread != "" && thread != sender {
		out = append(out, thread)
	}
	return out
}

func customerKey(channel, sender, user string) (string, bool) {
	if channel != "" && sender != "" {
		return channel + ":" + sender, true
	}
	if user != "" {
		return "goclaw:" + user, true
	}
	return "", false
}

func recordInserted(db Store, tenant string, keys [][2]string) error {
	rows, err := db.ListContacts(tenant, "", "", "")
	if err != nil {
		return errors.New("list contacts failed")
	}
	by := map[string]string{}
	for _, row := range rows {
		if row.Channel != "" && row.Dest != "" {
			by[key(row.Channel, row.Dest)] = row.ID
		}
		for _, id := range row.Identifiers {
			by[key(id.Channel, id.Dest)] = row.ID
		}
	}
	for _, pair := range keys {
		id := by[key(pair[0], pair[1])]
		if id == "" {
			return errors.New("import map failed")
		}
		if err := db.PutContactImport(tenant, id, pair[0], pair[1]); err != nil {
			return errors.New("import map failed")
		}
	}
	return nil
}

func writeCustomerMap(path string, lines []mapLine) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	uniq := map[string]mapLine{}
	for _, line := range lines {
		if line.key == "" || line.channel == "" {
			return errConfig("customer key is empty")
		}
		uniq[line.channel+"\t"+line.sender+"\t"+line.user] = line
	}
	keys := make([]string, 0, len(uniq))
	for k := range uniq {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		line := uniq[k]
		b.WriteString(line.channel)
		b.WriteByte('\t')
		b.WriteString(line.sender)
		b.WriteByte('\t')
		b.WriteString(line.user)
		b.WriteByte('\t')
		b.WriteString(line.key)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return errors.New("write customer map failed")
	}
	return nil
}

// LoadDenyFile reads one tenant id per line. A missing, empty, or malformed
// file is an error. The returned set is lower-case and contains no names
// in the error text.
func LoadDenyFile(path string) (map[string]struct{}, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errConfig("deny file is missing")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, errConfig("deny file is missing")
	}
	if bytes.Contains(body, []byte{0}) {
		return nil, errConfig("deny file is malformed")
	}
	ids := map[string]struct{}{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\uFEFF"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.ContainsAny(line, " \t") || !denyTokenOK(line) {
			return nil, errConfig("deny file is malformed")
		}
		ids[strings.ToLower(line)] = struct{}{}
	}
	if len(ids) == 0 {
		return nil, errConfig("deny file is empty")
	}
	return ids, nil
}

func denyTokenOK(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

// DeniedCount is how many of the named tenants are on the denylist.
// The number is safe to print. The names are not.
func DeniedCount(ids map[string]struct{}, tenants ...string) int {
	n := 0
	seen := map[string]struct{}{}
	for _, tenant := range tenants {
		tenant = strings.ToLower(strings.TrimSpace(tenant))
		if tenant == "" {
			continue
		}
		if _, ok := seen[tenant]; ok {
			continue
		}
		seen[tenant] = struct{}{}
		if _, ok := ids[tenant]; ok {
			n++
		}
	}
	return n
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
	skipSum := 0
	for _, r := range rep.Reasons {
		if r.Code == "merged" || r.Code == "native-exists" {
			skipSum += r.Count
			continue
		}
		invalidSum += r.Count
	}
	if invalidSum != rep.Invalid || skipSum != rep.Skipped {
		return errors.New("import tally failed")
	}
	return nil
}
