// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package episodicimport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// TenantEntry is one SPEC 152 source-tenant to goso-tenant line.
type TenantEntry struct {
	SourceTenant string
	Slug         string
	DestTenant   string
}

// AgentEntry is one agents-importer line. DestAgent must already exist.
type AgentEntry struct {
	SourceAgent string
	DestAgent   string
	DestTenant  string
}

// TenantMap indexes GoClaw tenant ids and slugs.
type TenantMap struct {
	bySource map[string]TenantEntry
}

// AgentMap indexes GoClaw agent ids.
type AgentMap struct {
	bySource map[string]AgentEntry
}

// Denylist is a local file of tenant ids. Names are not copied into reports.
type Denylist struct {
	ids []string
}

var (
	errDenylistEmpty     = config("denylist is empty")
	errDenylistMalformed = config("denylist is malformed")
	errDenylistMissing   = config("denylist is missing")
)

// Lookup returns the entry for a source tenant id or slug.
func (m TenantMap) Lookup(source string) (TenantEntry, bool) {
	if m.bySource == nil {
		return TenantEntry{}, false
	}
	e, ok := m.bySource[normID(source)]
	return e, ok
}

// Lookup returns the entry for a source agent id.
func (m AgentMap) Lookup(source string) (AgentEntry, bool) {
	if m.bySource == nil {
		return AgentEntry{}, false
	}
	e, ok := m.bySource[normID(source)]
	return e, ok
}

// LoadTenants reads a JSONL tenant map. Each line is
// {"source_tenant":"<goclaw uuid or slug>","slug":"<goclaw slug>","dest_tenant":"<goso tenant>"}.
func LoadTenants(r io.Reader) (TenantMap, error) {
	rows, err := readLines(r)
	if err != nil {
		return TenantMap{}, err
	}
	out := TenantMap{bySource: map[string]TenantEntry{}}
	for i, raw := range rows {
		var row struct {
			SourceTenant string `json:"source_tenant"`
			Slug         string `json:"slug"`
			DestTenant   string `json:"dest_tenant"`
		}
		if err := json.Unmarshal(raw, &row); err != nil {
			return TenantMap{}, configf("tenant map line %d is not JSON", i+1)
		}
		src := strings.TrimSpace(row.SourceTenant)
		if src == "" || strings.TrimSpace(row.DestTenant) == "" {
			return TenantMap{}, configf("tenant map line %d is incomplete", i+1)
		}
		entry := TenantEntry{
			SourceTenant: src,
			Slug:         strings.TrimSpace(row.Slug),
			DestTenant:   strings.TrimSpace(row.DestTenant),
		}
		if _, ok := out.bySource[normID(src)]; ok {
			return TenantMap{}, config("tenant map source is repeated")
		}
		out.bySource[normID(src)] = entry
		if entry.Slug != "" {
			if prev, ok := out.bySource[normID(entry.Slug)]; ok && normID(prev.SourceTenant) != normID(src) {
				return TenantMap{}, config("tenant map slug is repeated")
			}
			out.bySource[normID(entry.Slug)] = entry
		}
	}
	return out, nil
}

// LoadAgents reads a JSONL agent map. Each line is
// {"source_agent":"<goclaw uuid>","dest_agent":"<goso agent id>","dest_tenant":"<goso tenant>"}.
func LoadAgents(r io.Reader) (AgentMap, error) {
	rows, err := readLines(r)
	if err != nil {
		return AgentMap{}, err
	}
	out := AgentMap{bySource: map[string]AgentEntry{}}
	for i, raw := range rows {
		var row struct {
			SourceAgent string `json:"source_agent"`
			DestAgent   string `json:"dest_agent"`
			DestTenant  string `json:"dest_tenant"`
		}
		if err := json.Unmarshal(raw, &row); err != nil {
			return AgentMap{}, configf("agent map line %d is not JSON", i+1)
		}
		src := strings.TrimSpace(row.SourceAgent)
		dst := strings.TrimSpace(row.DestAgent)
		ten := strings.TrimSpace(row.DestTenant)
		if src == "" || dst == "" || ten == "" {
			return AgentMap{}, configf("agent map line %d is incomplete", i+1)
		}
		key := normID(src)
		if _, ok := out.bySource[key]; ok {
			return AgentMap{}, config("agent map source is repeated")
		}
		out.bySource[key] = AgentEntry{SourceAgent: src, DestAgent: dst, DestTenant: ten}
	}
	return out, nil
}

// LoadCustomers reads the shared customer map. A JSON object
// {"customers":[{"user_id","customer_key"}]} matches the documents importer.
// JSONL objects with those fields are the same rows. A line that has
// channel, sender_id, and user_id and no customer_key uses channel:sender_id.
// An empty customer_key is not stored; lookup then falls back to goclaw:<user_id>.
func LoadCustomers(r io.Reader) (map[string]string, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, config("customer map read failed")
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	out := map[string]string{}
	if raw[0] == '{' {
		var doc struct {
			Customers []customerRow `json:"customers"`
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		if err := dec.Decode(&doc); err != nil {
			return nil, config("customer map is not JSON")
		}
		if err := addCustomers(out, doc.Customers); err != nil {
			return nil, err
		}
		return out, nil
	}
	if customerMapIsTSV(raw) {
		return loadCustomerTSV(raw)
	}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var row customerRow
		if err := json.Unmarshal([]byte(text), &row); err != nil {
			return nil, configf("customer map line %d is not JSON", line)
		}
		if err := addCustomers(out, []customerRow{row}); err != nil {
			return nil, err
		}
	}
	if err := sc.Err(); err != nil {
		return nil, config("customer map read failed")
	}
	return out, nil
}

type customerRow struct {
	UserID      string `json:"user_id"`
	CustomerKey string `json:"customer_key"`
	Channel     string `json:"channel"`
	SenderID    string `json:"sender_id"`
}

// customerMapIsTSV reports the 4-column file written by the contacts
// importer: channel, sender, user, key, separated by tabs.
func customerMapIsTSV(raw []byte) bool {
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		return bytes.Contains(line, []byte{'\t'}) && line[0] != '{'
	}
	return false
}

// loadCustomerTSV reads channel, sender, user, key. Column 3 is the GoClaw
// user id and column 4 is the customer key. zalo_personal and lark in the
// key's channel are normalized.
//
// A row that cannot name a customer is skipped, not fatal: GoClaw exports
// carry rows whose user_id is empty, and one such row used to abort the whole
// load. A user that appears under two keys keeps the first row, which matches
// the first-wins rule of the contacts importer (SPEC 172) and of the kgimport
// CustomerMap. The only remaining fatal case is a structural one, a line that
// does not have four columns, because then the file is not this format.
// Skipped rows never print the user id.
func loadCustomerTSV(raw []byte) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) != 4 {
			return nil, configf("customer map line %d has the wrong column count", lineNo)
		}
		user := strings.TrimSpace(cols[2])
		key := normalizeCustomerKey(cols[3])
		if user == "" || key == "" || strings.ContainsRune(key, 0) || strings.ContainsAny(key, "\r\n") {
			continue
		}
		if _, ok := out[user]; ok {
			continue
		}
		out[user] = key
	}
	if err := sc.Err(); err != nil {
		return nil, config("customer map read failed")
	}
	return out, nil
}

// normalizeCustomerKey rewrites a channel prefix the contacts importer accepts
// under more than one spelling. The sender id after the colon is unchanged.
func normalizeCustomerKey(key string) string {
	key = strings.TrimSpace(key)
	ch, rest, ok := strings.Cut(key, ":")
	if !ok {
		return key
	}
	switch strings.ToLower(strings.TrimSpace(ch)) {
	case "zalo_personal":
		ch = "zalo-personal"
	case "lark":
		ch = "feishu"
	}
	return ch + ":" + rest
}

// addCustomers folds JSON rows into the same map as the TSV path. A row that
// cannot name a customer is skipped rather than fatal, and the first row for a
// user wins, so the JSON and TSV paths agree. Neither path can abort a load on
// a single empty or duplicated user_id.
func addCustomers(dst map[string]string, rows []customerRow) error {
	for _, row := range rows {
		user := strings.TrimSpace(row.UserID)
		key := strings.TrimSpace(row.CustomerKey)
		if key == "" {
			ch := strings.TrimSpace(row.Channel)
			sid := strings.TrimSpace(row.SenderID)
			if ch != "" && sid != "" {
				key = ch + ":" + sid
			}
		}
		if user == "" || key == "" || strings.ContainsRune(key, 0) || strings.ContainsAny(key, "\r\n") {
			continue
		}
		if _, ok := dst[user]; ok {
			continue
		}
		dst[user] = key
	}
	return nil
}

// LoadDenylist reads one tenant id per line. Blank lines and # comments are
// ignored. A file with no ids is empty. A line that is not one tenant id is
// malformed. The ids are kept only for matching; reports never print them.
func LoadDenylist(r io.Reader) (Denylist, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var ids []string
	seen := map[string]bool{}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.ContainsAny(line, " \t") || !store.TenantOK(line) {
			return Denylist{}, errDenylistMalformed
		}
		key := normID(line)
		if seen[key] {
			continue
		}
		seen[key] = true
		ids = append(ids, key)
	}
	if err := sc.Err(); err != nil {
		return Denylist{}, errDenylistMalformed
	}
	if len(ids) == 0 {
		return Denylist{}, errDenylistEmpty
	}
	return Denylist{ids: ids}, nil
}

// Hits reports whether any candidate is on the list.
func (d Denylist) Hits(cands ...string) bool {
	if len(d.ids) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, id := range d.ids {
		set[id] = true
	}
	for _, c := range cands {
		if set[normID(c)] && strings.TrimSpace(c) != "" {
			return true
		}
	}
	return false
}

// DeniedCount is how many distinct tenants among source and dest are blocked.
// Source aliases (id and slug) are one tenant.
func (d Denylist) DeniedCount(sourceAliases []string, dest string) int {
	n := 0
	if d.Hits(sourceAliases...) {
		n++
	}
	if strings.TrimSpace(dest) == "" || !d.Hits(dest) {
		return n
	}
	for _, a := range sourceAliases {
		if normID(a) == normID(dest) {
			return n
		}
	}
	return n + 1
}

func readLines(r io.Reader) ([][]byte, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	var out [][]byte
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		out = append(out, []byte(line))
	}
	if err := sc.Err(); err != nil {
		return nil, config("map read failed")
	}
	return out, nil
}

func normID(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
