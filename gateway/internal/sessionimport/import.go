// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package sessionimport loads one tenant from an offline JSONL export of
// GoClaw sessions. Messages live in the sessions.messages array. The writer
// inserts goso sessions and messages only. It does not call PutMemory,
// SaveSummary, or the chat pipeline, and it does not keep the SQLite message
// search trigger armed while it inserts.
package sessionimport

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	batchSessions  = 32
	batchBytes     = 4 << 20
	maxLineDefault = 16 << 20
	maxIDLen       = 128
	maxKeyLen      = 500
	maxUserLen     = 255
	maxLabelRunes  = 500
	maxModelRunes  = 200
	maxProvider    = 50
)

// Columns and message fields that have no home on goso sessions or messages.
// channel is read only to match a contact identifier. It is not stored.
var notImportedColumns = []string{
	"channel",
	"compaction_count",
	"input_tokens",
	"memory_flush_at",
	"memory_flush_compaction_count",
	"messages.id",
	"metadata",
	"output_tokens",
	"spawn_depth",
	"spawned_by",
	"summary",
	"team_id",
	"updated_at",
}

var errLineTooLarge = errors.New("line too large")

// ConfigError is a caller mistake. The command prints it as-is.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

// SQLDB is a goso SQLite or Postgres store.
type SQLDB interface {
	SQL() (*sql.DB, bool)
}

// Options selects the destination tenant. DryRun writes nothing.
type Options struct {
	Tenant       string
	SourceTenant string
	DryRun       bool
	MaxLine      int
}

// Report is counts only. It has no message text, labels, or source ids.
type Report struct {
	DryRun      bool          `json:"dry_run"`
	Tenants     []TenantCount `json:"tenants"`
	NotImported []string      `json:"not_imported"`
}

// TenantCount is one source tenant_id in the file.
type TenantCount struct {
	TenantID          string `json:"tenant_id"`
	Sessions          int    `json:"sessions"`
	Messages          int    `json:"messages"`
	New               int    `json:"new"`
	Skipped           int    `json:"skipped"`
	Invalid           int    `json:"invalid"`
	InvalidMessages   int    `json:"invalid_messages"`
	Foreign           int    `json:"foreign"`
	UnresolvedAgent   int    `json:"unresolved_agent"`
	UnresolvedContact int    `json:"unresolved_contact"`
}

type sessionRow struct {
	bucket      string
	id          string
	key         string
	label       string
	at          time.Time
	model       string
	provider    string
	agentSrc    string
	agentID     string
	userSrc     string
	contactID   string
	msgs        []outMsg
	badMsgs     int
	needAgent   bool
	needContact bool
	foreign     bool
}

type outMsg struct {
	id        string
	role      string
	content   string
	agentID   string
	mentionID string
	model     string
	provider  string
	at        time.Time
}

// Import streams r once. opt.Tenant is the goso tenant that receives rows.
// opt.SourceTenant, when set, is the only export tenant_id that is eligible.
// An empty export tenant_id is eligible only when SourceTenant is empty, and
// those rows are counted under opt.Tenant.
func Import(st SQLDB, r io.Reader, opt Options) (rep *Report, err error) {
	if st == nil {
		return nil, &ConfigError{Msg: "database is not open"}
	}
	db, pg := st.SQL()
	if db == nil {
		return nil, &ConfigError{Msg: "database is not open"}
	}
	opt.Tenant = strings.TrimSpace(opt.Tenant)
	opt.SourceTenant = strings.TrimSpace(opt.SourceTenant)
	if !store.TenantOK(opt.Tenant) {
		return nil, &ConfigError{Msg: "tenant is invalid"}
	}
	if opt.SourceTenant != "" && !sourceOK(opt.SourceTenant) {
		return nil, &ConfigError{Msg: "source tenant is invalid"}
	}
	if r == nil {
		return nil, &ConfigError{Msg: "export is required"}
	}
	if opt.MaxLine <= 0 {
		opt.MaxLine = maxLineDefault
	}
	w := &writer{db: db, pg: pg, dest: opt.Tenant, dry: opt.DryRun}
	guard := &triggerGuard{w: w}
	if !opt.DryRun {
		if err := w.ensureSchema(); err != nil {
			return nil, err
		}
		if err := guard.drop(); err != nil {
			return nil, err
		}
		defer func() {
			if rerr := guard.restore(); rerr != nil && err == nil {
				err = rerr
			}
		}()
	}
	links, err := w.hasLinks()
	if err != nil {
		return nil, err
	}
	w.links = links
	res, err := loadResolver(w)
	if err != nil {
		return nil, err
	}
	acc := newAcc()
	if err := stream(r, opt, res, w, acc); err != nil {
		return nil, err
	}
	return acc.report(opt.DryRun), nil
}

func sourceOK(s string) bool {
	if s == "" || len(s) > maxIDLen {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

type accumulator struct {
	m     map[string]*TenantCount
	extra map[string]struct{}
}

func newAcc() *accumulator {
	a := &accumulator{m: map[string]*TenantCount{}, extra: map[string]struct{}{}}
	return a
}

func (a *accumulator) tenant(id string) *TenantCount {
	t, ok := a.m[id]
	if !ok {
		t = &TenantCount{TenantID: id}
		a.m[id] = t
	}
	return t
}

func (a *accumulator) addKeys(keys []string) {
	for _, k := range keys {
		if k != "" {
			a.extra[k] = struct{}{}
		}
	}
}

func (a *accumulator) report(dry bool) *Report {
	keys := make([]string, 0, len(a.m))
	for k := range a.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	tenants := make([]TenantCount, 0, len(keys))
	for _, k := range keys {
		tenants = append(tenants, *a.m[k])
	}
	ni := make([]string, 0, len(notImportedColumns)+len(a.extra))
	ni = append(ni, notImportedColumns...)
	for k := range a.extra {
		ni = append(ni, k)
	}
	sort.Strings(ni)
	ni = uniqueStrings(ni)
	return &Report{DryRun: dry, Tenants: tenants, NotImported: ni}
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}

func stream(r io.Reader, opt Options, res resolver, w *writer, acc *accumulator) error {
	br := bufio.NewReaderSize(r, 64*1024)
	seenID := map[string]struct{}{}
	seenKey := map[string]struct{}{}
	var batch []sessionRow
	batchN := 0
	first := true
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := w.flush(batch, acc)
		batch = nil
		batchN = 0
		return err
	}
	for {
		line, err := readLine(br, opt.MaxLine)
		if first && len(line) > 0 {
			line = bytes.TrimPrefix(line, []byte{0xEF, 0xBB, 0xBF})
			first = false
		}
		if errors.Is(err, errLineTooLarge) {
			acc.tenant("").Invalid++
			acc.tenant("").Sessions++
			continue
		}
		if len(bytes.TrimSpace(line)) > 0 {
			row, keys, invalid := parseLine(line, opt, res)
			acc.addKeys(keys)
			if invalid {
				acc.tenant(row.bucket).Invalid++
				acc.tenant(row.bucket).Sessions++
			} else if row.foreign {
				acc.tenant(row.bucket).Foreign++
				acc.tenant(row.bucket).Sessions++
			} else if _, ok := seenID[row.id]; ok {
				acc.tenant(row.bucket).Skipped++
				acc.tenant(row.bucket).Sessions++
			} else if _, ok := seenKey[row.key]; ok {
				acc.tenant(row.bucket).Skipped++
				acc.tenant(row.bucket).Sessions++
			} else {
				seenID[row.id] = struct{}{}
				seenKey[row.key] = struct{}{}
				if opt.DryRun {
					for i := range row.msgs {
						row.msgs[i].content = ""
					}
				}
				batch = append(batch, row)
				batchN += row.bytes()
				if len(batch) >= batchSessions || batchN >= batchBytes {
					if err := flush(); err != nil {
						return err
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return flush()
}

func (s sessionRow) bytes() int {
	n := len(s.label)
	for _, m := range s.msgs {
		n += len(m.content)
	}
	return n
}

func readLine(r *bufio.Reader, max int) ([]byte, error) {
	var out []byte
	for {
		frag, err := r.ReadSlice('\n')
		if max > 0 && len(out)+len(frag) > max {
			if errors.Is(err, bufio.ErrBufferFull) {
				for {
					_, err = r.ReadSlice('\n')
					if errors.Is(err, bufio.ErrBufferFull) {
						continue
					}
					break
				}
			}
			if err != nil && err != io.EOF && !errors.Is(err, bufio.ErrBufferFull) {
				return nil, err
			}
			return nil, errLineTooLarge
		}
		out = append(out, frag...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return out, err
	}
}

func parseLine(line []byte, opt Options, res resolver) (sessionRow, []string, bool) {
	line = bytes.TrimSpace(line)
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(line, &obj); err != nil || obj == nil {
		return sessionRow{}, nil, true
	}
	row, keys, invalid := parseSession(obj, res)
	if invalid {
		row.bucket = bucketOf(row.bucket, opt, false)
		return row, keys, true
	}
	eligible := rowEligible(row.bucket, opt)
	if !eligible {
		row.foreign = true
		row.bucket = bucketOf(row.bucket, opt, false)
		return row, keys, false
	}
	row.bucket = bucketOf(row.bucket, opt, true)
	return row, keys, false
}

func rowEligible(sourceTenant string, opt Options) bool {
	sourceTenant = strings.TrimSpace(sourceTenant)
	if opt.SourceTenant != "" {
		return sourceTenant == opt.SourceTenant
	}
	return sourceTenant == "" || sourceTenant == opt.Tenant
}

func bucketOf(sourceTenant string, opt Options, eligible bool) string {
	sourceTenant = strings.TrimSpace(sourceTenant)
	if eligible && sourceTenant == "" {
		return opt.Tenant
	}
	return sourceTenant
}

func parseSession(obj map[string]json.RawMessage, res resolver) (sessionRow, []string, bool) {
	var extra []string
	tenant, _, ok := takeString(obj, "tenant_id")
	if !ok {
		return sessionRow{}, extra, true
	}
	row := sessionRow{bucket: strings.TrimSpace(tenant)}
	id, _, ok := takeString(obj, "id")
	if !ok {
		return row, extra, true
	}
	key, _, ok := takeString(obj, "session_key")
	if !ok {
		return row, extra, true
	}
	label, _, ok := takeString(obj, "label")
	if !ok {
		return row, extra, true
	}
	model, _, ok := takeString(obj, "model")
	if !ok {
		return row, extra, true
	}
	provider, _, ok := takeString(obj, "provider")
	if !ok {
		return row, extra, true
	}
	agentSrc, _, ok := takeString(obj, "agent_id")
	if !ok {
		return row, extra, true
	}
	userSrc, _, ok := takeString(obj, "user_id")
	if !ok {
		return row, extra, true
	}
	channel, _, ok := takeString(obj, "channel")
	if !ok {
		return row, extra, true
	}
	createdRaw, hasCreated := take(obj, "created_at")
	if !hasCreated {
		return row, extra, true
	}
	created, okTime := parseTimeValue(createdRaw)
	if !okTime {
		return row, extra, true
	}
	msgRaw, hasMsg := take(obj, "messages")
	// Known columns we do not store. Taking them keeps them out of extras.
	for _, k := range []string{
		"summary", "input_tokens", "output_tokens", "compaction_count",
		"memory_flush_compaction_count", "memory_flush_at", "spawned_by",
		"spawn_depth", "updated_at", "metadata", "team_id",
	} {
		delete(obj, k)
	}
	for k := range obj {
		extra = append(extra, "sessions."+k)
	}
	id = strings.TrimSpace(id)
	key = strings.TrimSpace(key)
	if !tokenOK(id, maxIDLen) || !tokenOK(key, maxKeyLen) {
		return row, extra, true
	}
	agentSrc = strings.TrimSpace(agentSrc)
	userSrc = strings.TrimSpace(userSrc)
	channel = strings.TrimSpace(channel)
	if !tokenOK(agentSrc, maxIDLen) && agentSrc != "" {
		return row, extra, true
	}
	if !tokenOK(userSrc, maxUserLen) && userSrc != "" {
		return row, extra, true
	}
	if hasControl(channel) {
		return row, extra, true
	}
	var msgs []map[string]json.RawMessage
	var msgKeys []string
	if hasMsg {
		msgs, msgKeys, ok = decodeMessages(msgRaw)
		if !ok {
			return row, append(extra, msgKeys...), true
		}
	}
	extra = append(extra, msgKeys...)
	label = strings.TrimSpace(label)
	if label == "" {
		label = key
	}
	label = clipRunes(label, maxLabelRunes)
	agentID, _ := res.agent(agentSrc)
	contactID, referenced := res.contact(userSrc, channel)
	row.id = id
	row.key = key
	row.label = label
	row.at = created
	row.model = clipRunes(strings.TrimSpace(model), maxModelRunes)
	row.provider = clipRunes(strings.TrimSpace(provider), maxProvider)
	row.agentSrc = agentSrc
	row.agentID = agentID
	row.userSrc = userSrc
	row.contactID = contactID
	row.needAgent = agentSrc != "" && agentID == ""
	row.needContact = referenced && contactID == ""
	var prev time.Time
	for _, raw := range msgs {
		msg, keys, ok := parseMessage(raw, res, id, len(row.msgs), prev, created)
		extra = append(extra, keys...)
		if !ok {
			row.badMsgs++
			continue
		}
		prev = msg.at
		row.msgs = append(row.msgs, msg)
	}
	return row, extra, false
}

func parseMessage(obj map[string]json.RawMessage, res resolver, sessionID string, n int, prev, fallback time.Time) (outMsg, []string, bool) {
	if obj == nil {
		return outMsg{}, nil, false
	}
	var extra []string
	role, _, ok := takeString(obj, "role")
	if !ok || !validRole(strings.TrimSpace(role)) {
		return outMsg{}, extra, false
	}
	contentRaw, hasContent := take(obj, "content")
	var content string
	if hasContent {
		var keys []string
		content, keys, ok = textContent(contentRaw)
		extra = append(extra, keys...)
		if !ok {
			return outMsg{}, extra, false
		}
	}
	createdRaw, hasCreated := take(obj, "created_at")
	tsRaw, hasTS := take(obj, "timestamp")
	var at time.Time
	var hasAt bool
	if hasCreated {
		at, hasAt = parseTimeValue(createdRaw)
		if !hasAt {
			return outMsg{}, extra, false
		}
		if hasTS {
			extra = append(extra, "messages.timestamp")
		}
	} else if hasTS {
		at, hasAt = parseTimeValue(tsRaw)
		if !hasAt {
			return outMsg{}, extra, false
		}
	}
	agentSrc, _, ok := takeString(obj, "agent_id")
	if !ok {
		return outMsg{}, extra, false
	}
	mention, _, ok := takeString(obj, "mention_agent_id")
	if !ok {
		return outMsg{}, extra, false
	}
	model, _, ok := takeString(obj, "model")
	if !ok {
		return outMsg{}, extra, false
	}
	provider, _, ok := takeString(obj, "provider")
	if !ok {
		return outMsg{}, extra, false
	}
	delete(obj, "id")
	for k := range obj {
		extra = append(extra, "messages."+k)
	}
	agentID, _ := res.agent(strings.TrimSpace(agentSrc))
	mentionID, _ := res.agent(strings.TrimSpace(mention))
	msg := outMsg{
		id:        sessionID + ":" + pad6(n),
		role:      strings.TrimSpace(role),
		content:   content,
		agentID:   agentID,
		mentionID: mentionID,
		model:     clipRunes(strings.TrimSpace(model), maxModelRunes),
		provider:  clipRunes(strings.TrimSpace(provider), maxProvider),
		at:        nextStamp(prev, at, hasAt, fallback),
	}
	return msg, extra, true
}

func pad6(n int) string {
	const digits = "000000"
	s := itoa(n)
	if len(s) >= 6 {
		return s
	}
	return digits[:6-len(s)] + s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func nextStamp(prev, src time.Time, ok bool, fallback time.Time) time.Time {
	t := fallback.UTC()
	if ok {
		t = src.UTC()
	}
	if !prev.IsZero() && !t.After(prev) {
		return prev.Add(time.Nanosecond)
	}
	return t
}

func formatStamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func decodeMessages(raw json.RawMessage) ([]map[string]json.RawMessage, []string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, true
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return nil, nil, false
		}
		raw = bytes.TrimSpace([]byte(s))
		if len(raw) == 0 || string(raw) == "null" {
			return nil, nil, true
		}
	}
	var arr []map[string]json.RawMessage
	if json.Unmarshal(raw, &arr) != nil {
		return nil, nil, false
	}
	return arr, nil, true
}

func textContent(raw json.RawMessage) (string, []string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil, true
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil, true
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) == nil {
		var texts []string
		nonText := false
		for _, p := range parts {
			t, ok := partText(p)
			if !ok {
				nonText = true
				continue
			}
			texts = append(texts, t)
		}
		var keys []string
		if nonText {
			keys = append(keys, "messages.content.non_text")
		}
		if len(texts) == 0 {
			return "", keys, false
		}
		return strings.Join(texts, "\n"), keys, true
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		if t, ok := obj["text"]; ok {
			var s string
			if json.Unmarshal(t, &s) == nil {
				return s, nil, true
			}
		}
	}
	return "", nil, false
}

func partText(raw json.RawMessage) (string, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, true
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		if t, ok := obj["text"]; ok {
			var s string
			if json.Unmarshal(t, &s) == nil {
				return s, true
			}
		}
	}
	return "", false
}

func take(obj map[string]json.RawMessage, key string) (json.RawMessage, bool) {
	v, ok := obj[key]
	if ok {
		delete(obj, key)
	}
	return v, ok
}

func takeString(obj map[string]json.RawMessage, key string) (string, bool, bool) {
	raw, ok := take(obj, key)
	if !ok || len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return "", false, true
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", true, false
	}
	return s, true, true
}

func parseTimeValue(raw json.RawMessage) (time.Time, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
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
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return unixFlex(n)
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil && f >= 0 {
		return unixFlex(int64(f))
	}
	return time.Time{}, false
}

func unixFlex(n int64) (time.Time, bool) {
	if n < 0 {
		return time.Time{}, false
	}
	if n > 1e11 {
		return time.UnixMilli(n).UTC(), true
	}
	return time.Unix(n, 0).UTC(), true
}

func validRole(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		if r > 127 {
			return false
		}
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

func tokenOK(s string, max int) bool {
	if s == "" || len(s) > max || hasControl(s) {
		return false
	}
	return true
}

func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func (t *TenantCount) addNew(row sessionRow) {
	t.Sessions++
	t.New++
	t.Messages += len(row.msgs)
	t.InvalidMessages += row.badMsgs
	if row.needAgent {
		t.UnresolvedAgent++
	}
	if row.needContact {
		t.UnresolvedContact++
	}
}

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for j := range s {
		if i == n {
			return s[:j]
		}
		i++
	}
	return s
}
