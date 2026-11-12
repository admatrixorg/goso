// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package kgimport

import (
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// TenantMap is the SPEC 152 pairing: one GoClaw tenant id to one Goso
// tenant id. A run names one source tenant and one dest tenant explicitly.
// A tenant on the local denylist is refused before any write. This file
// does not name tenants that must be refused.
//
//	{"tenants":[{"source_tenant":"<goclaw tenant uuid>","dest_tenant":"minh-an","slug":"minh-an"}]}
type TenantMap struct {
	bySource map[string]tenantDest
}

type tenantDest struct {
	Source string
	Dest   string
	Slug   string
}

type tenantMapFile struct {
	Tenants []tenantMapRow `json:"tenants"`
}

type tenantMapRow struct {
	SourceTenant string `json:"source_tenant"`
	DestTenant   string `json:"dest_tenant"`
	Slug         string `json:"slug"`
}

// AgentMap is the input from the agents importer (SPEC 174). This PR does
// not import that PR's code. One GoClaw agent id maps to one Goso agent
// id that already exists in the destination tenant.
//
//	{"agents":[{"goclaw_agent_id":"<uuid>","goso_agent_id":"<goso id>","source_tenant":"<goclaw tenant uuid>"}]}
//
// source_tenant is optional. When it is set, the row applies only to that
// GoClaw tenant.
type AgentMap struct {
	byID map[string]agentDest
}

type agentDest struct {
	GoClawID     string
	GosoID       string
	SourceTenant string
}

type agentMapFile struct {
	Agents []agentMapRow `json:"agents"`
}

type agentMapRow struct {
	GoClawAgentID string `json:"goclaw_agent_id"`
	GosoAgentID   string `json:"goso_agent_id"`
	SourceTenant  string `json:"source_tenant"`
}

// ReadTenantMap decodes the SPEC 152 tenant file.
func ReadTenantMap(r io.Reader) (TenantMap, error) {
	var f tenantMapFile
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return TenantMap{}, errConfig("tenant map is not json")
	}
	if len(f.Tenants) == 0 {
		return TenantMap{}, errConfig("tenant map is empty")
	}
	out := TenantMap{bySource: make(map[string]tenantDest, len(f.Tenants))}
	for _, row := range f.Tenants {
		src := strings.TrimSpace(row.SourceTenant)
		if id, ok := canonUUID(src); ok {
			src = id
		}
		if src == "" {
			return TenantMap{}, errConfig("tenant map: source tenant is empty")
		}
		if _, dup := out.bySource[src]; dup {
			return TenantMap{}, errConfig("tenant map: duplicate source tenant")
		}
		slug := strings.ToLower(strings.TrimSpace(row.Slug))
		dest := strings.TrimSpace(row.DestTenant)
		if !store.TenantOK(dest) {
			return TenantMap{}, errConfig("tenant map: dest tenant is invalid")
		}
		out.bySource[src] = tenantDest{Source: src, Dest: dest, Slug: slug}
	}
	return out, nil
}

// ReadAgentMap decodes the SPEC 174 agent-id file.
func ReadAgentMap(r io.Reader) (AgentMap, error) {
	var f agentMapFile
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return AgentMap{}, errConfig("agent map is not json")
	}
	if len(f.Agents) == 0 {
		return AgentMap{}, errConfig("agent map is empty")
	}
	out := AgentMap{byID: make(map[string]agentDest, len(f.Agents))}
	for _, row := range f.Agents {
		gc, ok := canonUUID(row.GoClawAgentID)
		if !ok {
			return AgentMap{}, errConfig("agent map: goclaw agent id is invalid")
		}
		goso := strings.TrimSpace(row.GosoAgentID)
		if goso == "" {
			return AgentMap{}, errConfig("agent map: goso agent id is empty")
		}
		if _, dup := out.byID[gc]; dup {
			return AgentMap{}, errConfig("agent map: duplicate goclaw agent id")
		}
		src := ""
		if strings.TrimSpace(row.SourceTenant) != "" {
			src, ok = canonUUID(row.SourceTenant)
			if !ok {
				return AgentMap{}, errConfig("agent map: source tenant is invalid")
			}
		}
		out.byID[gc] = agentDest{GoClawID: gc, GosoID: goso, SourceTenant: src}
	}
	return out, nil
}

func (m TenantMap) lookup(source string) (tenantDest, bool) {
	if id, ok := canonUUID(source); ok {
		source = id
	} else {
		source = strings.TrimSpace(source)
	}
	d, ok := m.bySource[source]
	return d, ok
}

func (m AgentMap) lookup(goclawID, sourceTenant string) (agentDest, bool) {
	id, ok := canonUUID(goclawID)
	if !ok {
		return agentDest{}, false
	}
	d, ok := m.byID[id]
	if !ok {
		return agentDest{}, false
	}
	if d.SourceTenant != "" && d.SourceTenant != sourceTenant {
		return agentDest{}, false
	}
	return d, true
}

func canonUUID(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if !uuidRE.MatchString(s) {
		return "", false
	}
	return s, true
}

func gcKGID(orig string) (string, bool) {
	id, ok := canonUUID(orig)
	if !ok {
		return "", false
	}
	return "gc-kg:" + id, true
}

func provenanceID(orig string) (string, bool) {
	id, ok := canonUUID(orig)
	if !ok {
		return "", false
	}
	return "goclaw:" + id, true
}
