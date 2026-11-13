// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memdocimport

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Mapping is the offline input for one run.
// Tenants are the SPEC 152 pair (GoClaw tenant id → Goso tenant id), plus the
// source slug. Agents are the SPEC 174 id map, read from this file. This
// package does not import that branch. Customer keys come from a separate
// channel_contacts file, not from this mapping.
type Mapping struct {
	Tenants []TenantMap `json:"tenants"`
	Agents  []AgentMap  `json:"agents"`
}

// TenantMap is one SPEC 152 source tenant and its Goso dest tenant.
type TenantMap struct {
	SourceID string `json:"source_id"`
	Slug     string `json:"slug"`
	Dest     string `json:"dest"`
}

// AgentMap points a GoClaw agent uuid at an existing Goso agent id.
type AgentMap struct {
	SourceID     string `json:"source_id"`
	SourceTenant string `json:"source_tenant"`
	DestID       string `json:"dest_id"`
}

type tenantBind struct {
	source string
	slug   string
	dest   string
}

type agentBind struct {
	dest   string
	source string
}

type loadedMap struct {
	tenants map[string]tenantBind
	agents  map[string]agentBind
}

// ReadMapping parses and checks the mapping file.
func ReadMapping(r io.Reader) (Mapping, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var m Mapping
	if err := dec.Decode(&m); err != nil {
		return Mapping{}, errConfig("mapping file is not the documented JSON")
	}
	var extra struct{}
	if err := dec.Decode(&extra); err != io.EOF {
		return Mapping{}, errConfig("mapping file is not the documented JSON")
	}
	if _, err := compileMap(m); err != nil {
		return Mapping{}, err
	}
	return m, nil
}

func compileMap(m Mapping) (*loadedMap, error) {
	out := &loadedMap{
		tenants: map[string]tenantBind{},
		agents:  map[string]agentBind{},
	}
	if len(m.Tenants) == 0 {
		return nil, errConfig("mapping file needs a tenant")
	}
	for _, t := range m.Tenants {
		src := strings.TrimSpace(t.SourceID)
		dest := strings.TrimSpace(t.Dest)
		slug := strings.TrimSpace(t.Slug)
		if src == "" || dest == "" {
			return nil, errConfig("tenant mapping needs source_id and dest")
		}
		if !store.TenantOK(dest) {
			return nil, errConfig("dest tenant id is empty or not a goso tenant id")
		}
		if _, ok := out.tenants[src]; ok {
			return nil, errConfig("tenant source_id is repeated")
		}
		out.tenants[src] = tenantBind{source: src, slug: slug, dest: dest}
	}
	for _, a := range m.Agents {
		src := strings.TrimSpace(a.SourceID)
		tenant := strings.TrimSpace(a.SourceTenant)
		dest := strings.TrimSpace(a.DestID)
		if src == "" || tenant == "" || dest == "" {
			return nil, errConfig("agent mapping needs source_id, source_tenant, and dest_id")
		}
		tb, ok := out.tenants[tenant]
		if !ok {
			return nil, errConfig("agent mapping names a tenant that is not in the tenant list")
		}
		if !safeSegment(dest) {
			return nil, errConfig("agent dest_id is not a single path segment")
		}
		if _, ok := out.agents[agentKey(tenant, src)]; ok {
			return nil, errConfig("agent source_id is repeated")
		}
		out.agents[agentKey(tenant, src)] = agentBind{dest: dest, source: tb.source}
	}
	return out, nil
}

// resolve finds the tenant named by a source id or a slug. Two matches is an error.
func (m *loadedMap) resolve(token string) (tenantBind, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return tenantBind{}, errConfig("source tenant is required")
	}
	if tb, ok := m.tenants[token]; ok {
		return tb, nil
	}
	var found tenantBind
	n := 0
	for _, tb := range m.tenants {
		if strings.EqualFold(tb.slug, token) || strings.EqualFold(tb.source, token) || strings.EqualFold(tb.dest, token) {
			found = tb
			n++
		}
	}
	if n == 1 {
		return found, nil
	}
	if n > 1 {
		return tenantBind{}, errConfig("source tenant is ambiguous")
	}
	return tenantBind{}, errConfig("source tenant is not in the mapping")
}

func agentKey(tenant, agent string) string { return tenant + "\x00" + agent }

func safeSegment(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > 200 {
		return false
	}
	if strings.ContainsAny(s, `/\:`) || strings.Contains(s, "..") {
		return false
	}
	for _, r := range s {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	return true
}
