// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package episodicimport

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

// mqSlug is never written. Compared case-insensitively on the source id,
// the source slug, and the destination tenant.
const mqSlug = "mq-capital"

// TenantEntry is one SPEC 152 source-tenant to goso-tenant line.
type TenantEntry struct {
	SourceTenant string
	Slug         string
	DestTenant   string
	SkipMQ       bool
}

// AgentEntry is one agents-importer line. DestAgent must already exist.
type AgentEntry struct {
	SourceAgent string
	DestAgent   string
	DestTenant  string
}

// TenantMap indexes GoClaw tenant ids.
type TenantMap struct {
	bySource map[string]TenantEntry
}

// AgentMap indexes GoClaw agent ids.
type AgentMap struct {
	bySource map[string]AgentEntry
}

// Lookup returns the entry for a source tenant id or slug.
func (m TenantMap) Lookup(source string) (TenantEntry, bool) {
	e, ok := m.bySource[normID(source)]
	return e, ok
}

// Lookup returns the entry for a source agent id.
func (m AgentMap) Lookup(source string) (AgentEntry, bool) {
	e, ok := m.bySource[normID(source)]
	return e, ok
}

// LoadTenants reads a JSONL tenant map. Each line is
// {"source_tenant":"<goclaw uuid or slug>","slug":"<goclaw slug>","dest_tenant":"<goso tenant>"}.
// A line whose source, slug, or dest is mq-capital is a skip marker.
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
		if src == "" {
			return TenantMap{}, configf("tenant map line %d is missing source_tenant", i+1)
		}
		key := normID(src)
		if _, ok := out.bySource[key]; ok {
			return TenantMap{}, configf("tenant map duplicate source %s", src)
		}
		entry := TenantEntry{
			SourceTenant: src,
			Slug:         strings.TrimSpace(row.Slug),
			DestTenant:   strings.TrimSpace(row.DestTenant),
		}
		entry.SkipMQ = isMQ(entry.SourceTenant) || isMQ(entry.Slug) || isMQ(entry.DestTenant)
		if !entry.SkipMQ && entry.DestTenant == "" {
			return TenantMap{}, configf("tenant map line %d is missing dest_tenant", i+1)
		}
		out.bySource[key] = entry
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
		if isMQ(ten) {
			return AgentMap{}, config("agent map must not name mq-capital")
		}
		key := normID(src)
		if _, ok := out.bySource[key]; ok {
			return AgentMap{}, configf("agent map duplicate source %s", src)
		}
		out.bySource[key] = AgentEntry{SourceAgent: src, DestAgent: dst, DestTenant: ten}
	}
	return out, nil
}

func readLines(r io.Reader) ([][]byte, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	var out [][]byte
	n := 0
	for sc.Scan() {
		n++
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

func isMQ(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), mqSlug)
}
