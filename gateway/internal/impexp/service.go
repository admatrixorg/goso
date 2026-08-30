// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package impexp

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/skill"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const maxJobs = 64

// Service holds portable catalogs, archives, and in-memory import jobs.
type Service struct {
	mu   sync.Mutex
	st   store.StoreIface
	jobs map[string]*Job
}

// New wires a store. Nil store yields empty catalogs and failed writes.
func New(st store.StoreIface) *Service {
	return &Service{st: st, jobs: map[string]*Job{}}
}

func (s *Service) Catalog(tenantID string) Catalog {
	tenantID = store.NormalizeTenant(tenantID)
	out := Catalog{
		Teams:       []CatalogTeam{},
		Agents:      []CatalogAgent{},
		Skills:      []CatalogSkill{},
		MCP:         []CatalogMCP{},
		GeneratedAt: time.Now().UTC(),
	}
	if s == nil || s.st == nil {
		return out
	}
	for _, a := range s.st.ListAgents() {
		if a == nil || !store.SameTenant(a.TenantID, tenantID) {
			continue
		}
		out.Agents = append(out.Agents, CatalogAgent{
			ID: a.ID, AgentKey: a.AgentKey, DisplayName: a.DisplayName,
			Enabled: a.Enabled, Model: a.Model,
		})
	}
	for _, t := range s.st.ListTeams() {
		if t == nil || !store.SameTenant(t.TenantID, tenantID) {
			continue
		}
		n := 0
		if mem, err := s.st.ListTeamMembers(t.ID); err == nil {
			n = len(mem)
		}
		out.Teams = append(out.Teams, CatalogTeam{
			ID: t.ID, Name: t.Name, LeadAgentID: t.LeadAgentID, Members: n,
		})
	}
	out.SkillsConfigured = skill.Configured()
	if list, err := skill.List(); err == nil {
		for _, info := range list {
			out.Skills = append(out.Skills, CatalogSkill{Name: info.Name, Path: info.Path})
		}
	}
	for _, rec := range s.st.ListConnectors() {
		if rec == nil {
			continue
		}
		out.MCP = append(out.MCP, catalogMCP(s.st, rec))
	}
	return out
}

func catalogMCP(st store.StoreIface, rec *store.ConnectorRecord) CatalogMCP {
	item := CatalogMCP{
		Name: rec.Name, Transport: rec.Transport, Endpoint: rec.Endpoint, Enabled: rec.Enabled,
	}
	cred := strings.TrimSpace(rec.CredentialRef)
	item.EnvOwned = isEnvName(cred)
	if item.EnvOwned {
		item.TokenSet = strings.TrimSpace(os.Getenv(cred)) != ""
	} else if st != nil {
		if row, err := st.GetSecret(connector.TokenSecretName(rec.Name)); err == nil && row != nil {
			item.TokenSet = true
		} else if cred != "" {
			item.TokenSet = true
		}
	}
	return item
}

func (s *Service) Get(id string) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[strings.TrimSpace(id)]
	if !ok || j == nil {
		return nil, ErrNotFound
	}
	return PublicJob(j), nil
}

func (s *Service) putJob(j *Job) {
	if j == nil {
		return
	}
	if s.jobs == nil {
		s.jobs = map[string]*Job{}
	}
	s.jobs[j.ID] = j
	for len(s.jobs) > maxJobs {
		var oldest string
		var ts time.Time
		for id, job := range s.jobs {
			if oldest == "" || job.CreatedAt.Before(ts) {
				oldest = id
				ts = job.CreatedAt
			}
		}
		if oldest == "" || oldest == j.ID {
			break
		}
		delete(s.jobs, oldest)
	}
}

func (s *Service) Export(tenantID string, sel Selection) (*Job, error) {
	tenantID = store.NormalizeTenant(tenantID)
	j := newJob(KindExport)
	j.Status = StatusRunning
	if s == nil || s.st == nil {
		failJob(j, "store unavailable")
		s.remember(j)
		return PublicJob(j), nil
	}
	if len(sel.TeamIDs) == 0 && len(sel.AgentIDs) == 0 && len(sel.SkillNames) == 0 && len(sel.MCPNames) == 0 {
		failJob(j, ErrEmptySelection.Error())
		s.remember(j)
		return PublicJob(j), ErrEmptySelection
	}
	markStep(j, "validate", StatusDone, "")
	arch := &Archive{
		Schema: Schema, SchemaVersion: SchemaVersion, ExportedAt: time.Now().UTC(),
		IncludeSecrets: false,
	}
	wantAgents := setOf(sel.AgentIDs)
	wantTeams := setOf(sel.TeamIDs)
	wantSkills := setOf(sel.SkillNames)
	wantMCP := setOf(sel.MCPNames)

	byID := map[string]*store.Agent{}
	for _, a := range s.st.ListAgents() {
		if a == nil || !store.SameTenant(a.TenantID, tenantID) {
			continue
		}
		cp := *a
		byID[a.ID] = &cp
	}
	for _, a := range s.st.ListAgents() {
		if a == nil || !store.SameTenant(a.TenantID, tenantID) {
			continue
		}
		if wantAgents[a.ID] || wantAgents[a.AgentKey] {
			arch.Agents = append(arch.Agents, exportAgent(s.st, a, byID))
		}
	}
	markStep(j, "agents", StatusDone, strconv.Itoa(len(arch.Agents)))

	for _, t := range s.st.ListTeams() {
		if t == nil || !store.SameTenant(t.TenantID, tenantID) {
			continue
		}
		if !wantTeams[t.ID] && !wantTeams[t.Name] {
			continue
		}
		arch.Teams = append(arch.Teams, exportTeam(s.st, t, byID))
	}
	markStep(j, "teams", StatusDone, strconv.Itoa(len(arch.Teams)))

	if len(wantSkills) > 0 {
		if !skill.Configured() {
			j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "skill", Name: "*", Detail: "not_configured"})
		} else {
			for name := range wantSkills {
				doc, err := skill.Load(name)
				if err != nil {
					j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "skill", Name: name, Detail: err.Error()})
					continue
				}
				arch.Skills = append(arch.Skills, SkillItem{Name: doc.Name, Body: redactText(doc.Body)})
			}
		}
	}
	markStep(j, "skills", StatusDone, strconv.Itoa(len(arch.Skills)))

	for _, rec := range s.st.ListConnectors() {
		if rec == nil {
			continue
		}
		if _, ok := wantMCP[rec.Name]; !ok {
			continue
		}
		item := exportMCP(s.st, rec)
		arch.MCP = append(arch.MCP, item)
		if item.TokenSet {
			j.Report.CredentialsNeeded = append(j.Report.CredentialsNeeded, CredentialNeed{
				Kind: "mcp", Name: item.Name, Reason: "reenter", EnvName: item.EnvName,
			})
		}
	}
	markStep(j, "mcp", StatusDone, strconv.Itoa(len(arch.MCP)))

	if len(arch.Agents)+len(arch.Teams)+len(arch.Skills)+len(arch.MCP) == 0 {
		failJob(j, ErrEmptySelection.Error())
		s.remember(j)
		return PublicJob(j), ErrEmptySelection
	}
	StripArchive(arch)
	if ContainsSecrets(arch) {
		arch.Warnings = append(arch.Warnings, "dropped secret-shaped values")
		StripArchive(arch)
	}
	j.Archive = arch
	j.Status = StatusDone
	j.Progress = 100
	j.UpdatedAt = time.Now().UTC()
	markStep(j, "report", StatusDone, "")
	s.remember(j)
	return PublicJob(j), nil
}

func exportAgent(st store.StoreIface, a *store.Agent, byID map[string]*store.Agent) AgentItem {
	item := AgentItem{
		AgentKey: a.AgentKey, DisplayName: a.DisplayName, Model: a.Model,
		LLMProvider: a.LLMProvider, Instructions: redactText(a.Instructions),
		OrchestrationMode: a.OrchestrationMode, Enabled: a.Enabled,
	}
	if links, err := st.ListAgentLinks(a.ID); err == nil {
		for _, ln := range links {
			if tgt, ok := byID[ln.ToAgentID]; ok {
				item.Links = append(item.Links, tgt.AgentKey)
			}
		}
	}
	return item
}

func exportTeam(st store.StoreIface, t *store.Team, byID map[string]*store.Agent) TeamItem {
	item := TeamItem{Name: t.Name}
	if lead, ok := byID[t.LeadAgentID]; ok {
		item.LeadAgentKey = lead.AgentKey
	}
	if mem, err := st.ListTeamMembers(t.ID); err == nil {
		for _, m := range mem {
			if m == nil {
				continue
			}
			ag, ok := byID[m.AgentID]
			if !ok {
				continue
			}
			item.Members = append(item.Members, MemberItem{AgentKey: ag.AgentKey, Role: m.Role})
		}
	}
	return item
}

func exportMCP(st store.StoreIface, rec *store.ConnectorRecord) MCPItem {
	item := MCPItem{
		Name: rec.Name, Transport: rec.Transport, Endpoint: rec.Endpoint,
		SchemaVersion: rec.SchemaVersion, Enabled: rec.Enabled,
		ManifestURL: rec.ManifestURL, TimeoutMS: rec.TimeoutMS, Retries: rec.Retries,
	}
	cred := strings.TrimSpace(rec.CredentialRef)
	if isEnvName(cred) {
		item.EnvOwned = true
		item.CredentialKind = "env"
		item.EnvName = cred
		item.TokenSet = strings.TrimSpace(os.Getenv(cred)) != ""
		return item
	}
	if st != nil {
		if row, err := st.GetSecret(connector.TokenSecretName(rec.Name)); err == nil && row != nil {
			item.TokenSet = true
			item.CredentialKind = "secret"
		}
	}
	if !item.TokenSet && cred != "" {
		item.TokenSet = true
		item.CredentialKind = "secret"
	}
	return item
}

func (s *Service) Preview(tenantID string, raw []byte) (*Preview, error) {
	tenantID = store.NormalizeTenant(tenantID)
	p := &Preview{Errors: []string{}, Warnings: []string{}, Conflicts: []ConflictItem{}}
	a, err := DecodeArchive(raw)
	if err != nil {
		p.Errors = append(p.Errors, err.Error())
		return p, nil
	}
	p.Errors = append(p.Errors, ValidateSchema(a)...)
	if len(p.Errors) == 0 {
		StripArchive(a)
	}
	if ContainsSecrets(a) {
		p.Warnings = append(p.Warnings, "secrets dropped from archive")
		StripArchive(a)
	}
	p.Warnings = append(p.Warnings, a.Warnings...)
	p.Manifest = a.Manifest
	p.Archive = a
	if s != nil && s.st != nil {
		p.Conflicts = s.findConflicts(tenantID, a)
	}
	for _, m := range a.MCP {
		if m.TokenSet {
			p.Warnings = append(p.Warnings, "mcp "+m.Name+": re-enter credentials after import")
		}
	}
	p.Valid = len(p.Errors) == 0
	return p, nil
}

func (s *Service) findConflicts(tenantID string, a *Archive) []ConflictItem {
	var out []ConflictItem
	agents := s.agentsByKey(tenantID)
	teams := s.teamsByName(tenantID)
	skills := s.skillSet()
	mcp := s.mcpSet()
	for _, ag := range a.Agents {
		if cur, ok := agents[ag.AgentKey]; ok {
			out = append(out, ConflictItem{Kind: "agent", Name: ag.AgentKey, Existing: cur.ID})
		}
	}
	for _, t := range a.Teams {
		if cur, ok := teams[strings.ToLower(t.Name)]; ok {
			out = append(out, ConflictItem{Kind: "team", Name: t.Name, Existing: cur.ID})
		}
	}
	for _, sk := range a.Skills {
		if skills[sk.Name] {
			out = append(out, ConflictItem{Kind: "skill", Name: sk.Name, Existing: sk.Name})
		}
	}
	for _, m := range a.MCP {
		if mcp[m.Name] {
			out = append(out, ConflictItem{Kind: "mcp", Name: m.Name, Existing: m.Name})
		}
	}
	return out
}

func (s *Service) Import(tenantID string, raw []byte, conflict string, dryRun bool) (*Job, error) {
	tenantID = store.NormalizeTenant(tenantID)
	j := newJob(KindImport)
	j.DryRun = dryRun
	norm, err := NormalizeConflict(conflict)
	if err != nil {
		failJob(j, err.Error())
		s.remember(j)
		return PublicJob(j), err
	}
	j.Conflict = norm
	j.Status = StatusRunning
	a, err := DecodeArchive(raw)
	if err != nil {
		failJob(j, err.Error())
		s.remember(j)
		return PublicJob(j), err
	}
	if errs := ValidateSchema(a); len(errs) > 0 {
		failJob(j, strings.Join(errs, "; "))
		s.remember(j)
		return PublicJob(j), ErrSchema
	}
	StripArchive(a)
	j.Archive = a
	markStep(j, "validate", StatusDone, "")
	if s == nil || s.st == nil {
		failJob(j, "store unavailable")
		s.remember(j)
		return PublicJob(j), errors.New("store unavailable")
	}
	plan := &rollbackPlan{}
	keyToID := s.agentKeyIDs(tenantID)

	s.importAgents(tenantID, j, a, norm, dryRun, plan, keyToID)
	markStep(j, "agents", StatusDone, "")
	j.Progress = 25
	s.importTeams(tenantID, j, a, norm, dryRun, plan, keyToID)
	markStep(j, "teams", StatusDone, "")
	j.Progress = 50
	s.importSkills(j, a, norm, dryRun, plan)
	markStep(j, "skills", StatusDone, "")
	j.Progress = 75
	s.importMCP(j, a, norm, dryRun, plan)
	markStep(j, "mcp", StatusDone, "")
	j.Progress = 100
	markStep(j, "report", StatusDone, "")
	j.Status = StatusDone
	j.UpdatedAt = time.Now().UTC()
	if !dryRun {
		j.rollback = plan
	}
	s.remember(j)
	return PublicJob(j), nil
}

func (s *Service) importAgents(tenantID string, j *Job, a *Archive, conflict string, dryRun bool, plan *rollbackPlan, keyToID map[string]string) {
	existing := s.agentsByKey(tenantID)
	pending := make([]AgentItem, 0, len(a.Agents))
	for _, item := range a.Agents {
		key := strings.TrimSpace(item.AgentKey)
		if key == "" {
			j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "agent", Name: item.DisplayName, Detail: "agent_key required"})
			continue
		}
		cur, exists := existing[key]
		if !exists {
			if other := s.agentByKeyAny(key); other != nil && !store.SameTenant(other.TenantID, tenantID) {
				j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "agent", Name: key, Detail: "agent_key exists"})
				continue
			}
		}
		finalKey := key
		action := "create"
		if exists {
			switch conflict {
			case ConflictSkip:
				j.Report.Skipped = append(j.Report.Skipped, ReportItem{Kind: "agent", Name: key, ID: cur.ID})
				keyToID[key] = cur.ID
				continue
			case ConflictOverwrite:
				action = "overwrite"
			case ConflictRename:
				finalKey = uniqueAgentKey(existing, key)
				action = "rename"
			}
		}
		if dryRun {
			ri := ReportItem{Kind: "agent", Name: finalKey}
			if action == "overwrite" {
				j.Report.Overwritten = append(j.Report.Overwritten, ri)
			} else if action == "rename" {
				ri.Detail = key
				j.Report.Renamed = append(j.Report.Renamed, ri)
			} else {
				j.Report.Created = append(j.Report.Created, ri)
			}
			continue
		}
		if action == "overwrite" {
			plan.PrevAgents = append(plan.PrevAgents, agentSnap{
				ID: cur.ID, Instructions: cur.Instructions, Model: cur.Model,
				LLMProvider: cur.LLMProvider, OrchestrationMode: cur.OrchestrationMode, Enabled: cur.Enabled,
			})
			cur.Instructions = item.Instructions
			cur.Model = item.Model
			cur.LLMProvider = item.LLMProvider
			cur.OrchestrationMode = item.OrchestrationMode
			cur.Enabled = item.Enabled
			if _, err := s.st.UpdateAgent(*cur); err != nil {
				j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "agent", Name: key, Detail: err.Error()})
				continue
			}
			keyToID[key] = cur.ID
			j.Report.Overwritten = append(j.Report.Overwritten, ReportItem{Kind: "agent", Name: key, ID: cur.ID})
			pending = append(pending, item)
			continue
		}
		created, err := s.st.CreateAgent(store.Agent{
			TenantID: tenantID, AgentKey: finalKey, DisplayName: item.DisplayName, Model: item.Model,
			LLMProvider: item.LLMProvider, Instructions: item.Instructions,
			OrchestrationMode: item.OrchestrationMode,
		})
		if err != nil {
			j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "agent", Name: finalKey, Detail: err.Error()})
			continue
		}
		if !item.Enabled {
			created.Enabled = false
			if _, err := s.st.UpdateAgent(*created); err != nil {
				j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "agent", Name: finalKey, ID: created.ID, Detail: err.Error()})
			}
		}
		plan.CreatedAgents = append(plan.CreatedAgents, created.ID)
		keyToID[key] = created.ID
		keyToID[finalKey] = created.ID
		existing[finalKey] = created
		ri := ReportItem{Kind: "agent", Name: finalKey, ID: created.ID}
		if action == "rename" {
			ri.Detail = key
			j.Report.Renamed = append(j.Report.Renamed, ri)
		} else {
			j.Report.Created = append(j.Report.Created, ri)
		}
		pending = append(pending, AgentItem{AgentKey: key, Links: item.Links})
	}
	if dryRun {
		return
	}
	for _, item := range pending {
		from := keyToID[strings.TrimSpace(item.AgentKey)]
		if from == "" {
			continue
		}
		s.syncLinks(from, item.Links, keyToID)
	}
}

func (s *Service) syncLinks(fromID string, keys []string, keyToID map[string]string) {
	if fromID == "" {
		return
	}
	for _, k := range keys {
		toID := keyToID[strings.TrimSpace(k)]
		if toID == "" || toID == fromID {
			continue
		}
		_ = s.st.AddAgentLink(fromID, toID)
	}
}

func (s *Service) importTeams(tenantID string, j *Job, a *Archive, conflict string, dryRun bool, plan *rollbackPlan, keyToID map[string]string) {
	existing := s.teamsByName(tenantID)
	for _, item := range a.Teams {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "team", Name: "", Detail: "name required"})
			continue
		}
		cur, exists := existing[strings.ToLower(name)]
		finalName := name
		action := "create"
		if exists {
			switch conflict {
			case ConflictSkip:
				j.Report.Skipped = append(j.Report.Skipped, ReportItem{Kind: "team", Name: name, ID: cur.ID})
				continue
			case ConflictOverwrite:
				action = "overwrite"
			case ConflictRename:
				finalName = uniqueTeamName(existing, name)
				action = "rename"
			}
		}
		leadID := keyToID[item.LeadAgentKey]
		if dryRun {
			ri := ReportItem{Kind: "team", Name: finalName}
			if action == "overwrite" {
				j.Report.Overwritten = append(j.Report.Overwritten, ri)
			} else if action == "rename" {
				ri.Detail = name
				j.Report.Renamed = append(j.Report.Renamed, ri)
			} else {
				j.Report.Created = append(j.Report.Created, ri)
			}
			continue
		}
		if action == "overwrite" {
			plan.PrevTeams = append(plan.PrevTeams, teamSnap{ID: cur.ID, Name: cur.Name, LeadAgentID: cur.LeadAgentID})
			upd := *cur
			upd.Name = name
			if leadID != "" {
				upd.LeadAgentID = leadID
			}
			if _, err := s.st.UpdateTeam(upd); err != nil {
				j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "team", Name: name, Detail: err.Error()})
				continue
			}
			s.syncMembers(cur.ID, item.Members, keyToID)
			j.Report.Overwritten = append(j.Report.Overwritten, ReportItem{Kind: "team", Name: name, ID: cur.ID})
			continue
		}
		created, err := s.st.CreateTeam(store.Team{TenantID: tenantID, Name: finalName, LeadAgentID: leadID})
		if err != nil {
			j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "team", Name: finalName, Detail: err.Error()})
			continue
		}
		plan.CreatedTeams = append(plan.CreatedTeams, created.ID)
		existing[strings.ToLower(finalName)] = created
		s.syncMembers(created.ID, item.Members, keyToID)
		ri := ReportItem{Kind: "team", Name: finalName, ID: created.ID}
		if action == "rename" {
			ri.Detail = name
			j.Report.Renamed = append(j.Report.Renamed, ri)
		} else {
			j.Report.Created = append(j.Report.Created, ri)
		}
	}
}

func (s *Service) syncMembers(teamID string, members []MemberItem, keyToID map[string]string) {
	for _, m := range members {
		aid := keyToID[strings.TrimSpace(m.AgentKey)]
		if aid == "" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role == "" {
			role = "member"
		}
		_, _ = s.st.AddTeamMember(store.TeamMember{TeamID: teamID, AgentID: aid, Role: role})
	}
}

func (s *Service) importSkills(j *Job, a *Archive, conflict string, dryRun bool, plan *rollbackPlan) {
	if len(a.Skills) == 0 {
		return
	}
	if !skill.Configured() {
		for _, sk := range a.Skills {
			j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "skill", Name: sk.Name, Detail: "not_configured"})
		}
		return
	}
	have := s.skillSet()
	for _, item := range a.Skills {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "skill", Name: "", Detail: "name required"})
			continue
		}
		exists := have[name]
		final := name
		action := "create"
		if exists {
			switch conflict {
			case ConflictSkip:
				j.Report.Skipped = append(j.Report.Skipped, ReportItem{Kind: "skill", Name: name})
				continue
			case ConflictOverwrite:
				action = "overwrite"
			case ConflictRename:
				final = uniqueSkillName(have, name)
				action = "rename"
			}
		}
		if dryRun {
			ri := ReportItem{Kind: "skill", Name: final}
			if action == "overwrite" {
				j.Report.Overwritten = append(j.Report.Overwritten, ri)
			} else if action == "rename" {
				ri.Detail = name
				j.Report.Renamed = append(j.Report.Renamed, ri)
			} else {
				j.Report.Created = append(j.Report.Created, ri)
			}
			continue
		}
		if action == "overwrite" {
			if doc, err := skill.Load(name); err == nil {
				plan.PrevSkills = append(plan.PrevSkills, SkillItem{Name: doc.Name, Body: doc.Body})
			}
		}
		if _, err := skill.Create(final, item.Body); err != nil {
			j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "skill", Name: final, Detail: err.Error()})
			continue
		}
		have[final] = true
		if action == "overwrite" {
			j.Report.Overwritten = append(j.Report.Overwritten, ReportItem{Kind: "skill", Name: final})
		} else if action == "rename" {
			plan.CreatedSkills = append(plan.CreatedSkills, final)
			j.Report.Renamed = append(j.Report.Renamed, ReportItem{Kind: "skill", Name: final, Detail: name})
		} else {
			plan.CreatedSkills = append(plan.CreatedSkills, final)
			j.Report.Created = append(j.Report.Created, ReportItem{Kind: "skill", Name: final})
		}
	}
}

func (s *Service) importMCP(j *Job, a *Archive, conflict string, dryRun bool, plan *rollbackPlan) {
	existing := s.mcpByName()
	for _, item := range a.MCP {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "mcp", Name: "", Detail: "name required"})
			continue
		}
		transport, err := connector.NormalizeTransport(item.Transport)
		if err != nil {
			j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "mcp", Name: name, Detail: err.Error()})
			continue
		}
		cur, exists := existing[name]
		final := name
		action := "create"
		if exists {
			switch conflict {
			case ConflictSkip:
				j.Report.Skipped = append(j.Report.Skipped, ReportItem{Kind: "mcp", Name: name})
				if item.TokenSet {
					j.Report.CredentialsNeeded = append(j.Report.CredentialsNeeded, CredentialNeed{Kind: "mcp", Name: name, Reason: "reenter", EnvName: item.EnvName})
				}
				continue
			case ConflictOverwrite:
				action = "overwrite"
			case ConflictRename:
				final = uniqueMCPName(existing, name)
				action = "rename"
			}
		}
		needCred := item.TokenSet || item.CredentialKind == "secret" || item.EnvOwned
		if dryRun {
			ri := ReportItem{Kind: "mcp", Name: final}
			if action == "overwrite" {
				j.Report.Overwritten = append(j.Report.Overwritten, ri)
			} else if action == "rename" {
				ri.Detail = name
				j.Report.Renamed = append(j.Report.Renamed, ri)
			} else {
				j.Report.Created = append(j.Report.Created, ri)
			}
			if needCred {
				j.Report.CredentialsNeeded = append(j.Report.CredentialsNeeded, CredentialNeed{Kind: "mcp", Name: final, Reason: "reenter", EnvName: item.EnvName})
			}
			continue
		}
		credRef := ""
		if item.EnvOwned && isEnvName(item.EnvName) {
			credRef = item.EnvName
		}
		if action == "overwrite" {
			plan.PrevMCP = append(plan.PrevMCP, mcpSnap{Name: cur.Name, Endpoint: cur.Endpoint, Enabled: cur.Enabled})
			en := item.Enabled
			ep := item.Endpoint
			var cred *string
			if credRef != "" {
				cred = &credRef
			}
			if _, err := s.st.UpdateConnector(cur.Name, &en, &ep, cred); err != nil {
				j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "mcp", Name: name, Detail: err.Error()})
				continue
			}
			j.Report.Overwritten = append(j.Report.Overwritten, ReportItem{Kind: "mcp", Name: name})
			if needCred {
				j.Report.CredentialsNeeded = append(j.Report.CredentialsNeeded, CredentialNeed{Kind: "mcp", Name: name, Reason: "reenter", EnvName: item.EnvName})
			}
			continue
		}
		created, err := s.st.CreateConnector(store.ConnectorRecord{
			Name: final, Transport: transport, Endpoint: item.Endpoint,
			CredentialRef: credRef, SchemaVersion: item.SchemaVersion, Enabled: item.Enabled,
			ManifestURL: item.ManifestURL, TimeoutMS: item.TimeoutMS, Retries: item.Retries,
		})
		if err != nil {
			j.Report.Failed = append(j.Report.Failed, ReportItem{Kind: "mcp", Name: final, Detail: err.Error()})
			continue
		}
		plan.CreatedMCP = append(plan.CreatedMCP, created.Name)
		existing[final] = created
		ri := ReportItem{Kind: "mcp", Name: final}
		if action == "rename" {
			ri.Detail = name
			j.Report.Renamed = append(j.Report.Renamed, ri)
		} else {
			j.Report.Created = append(j.Report.Created, ri)
		}
		if needCred {
			j.Report.CredentialsNeeded = append(j.Report.CredentialsNeeded, CredentialNeed{Kind: "mcp", Name: final, Reason: "reenter", EnvName: item.EnvName})
		}
	}
}

func (s *Service) Rollback(id string) (*Job, error) {
	s.mu.Lock()
	src, ok := s.jobs[strings.TrimSpace(id)]
	if !ok || src == nil {
		s.mu.Unlock()
		return nil, ErrNotFound
	}
	if src.Kind != KindImport {
		s.mu.Unlock()
		return nil, ErrNoRollback
	}
	if src.DryRun {
		s.mu.Unlock()
		return nil, ErrDryRun
	}
	if src.Status == StatusRolledBack || src.Status == StatusRolling {
		s.mu.Unlock()
		return nil, ErrAlreadyRolled
	}
	if src.Status != StatusDone || src.rollback == nil {
		s.mu.Unlock()
		return nil, ErrImportNotDone
	}
	src.Status = StatusRolling
	src.UpdatedAt = time.Now().UTC()
	plan := src.rollback
	s.mu.Unlock()

	j := newJob(KindRollback)
	j.Status = StatusRunning
	var errs []string
	note := func(kind, name string, err error) {
		if err != nil {
			errs = append(errs, kind+" "+name+": "+err.Error())
		}
	}
	ignoreMissing := func(kind, name string, err error) {
		if err == nil || errors.Is(err, store.ErrNotFound) || errors.Is(err, skill.ErrNotFound) {
			return
		}
		note(kind, name, err)
	}
	for _, tid := range plan.CreatedTeams {
		ignoreMissing("team", tid, s.st.DeleteTeam(tid))
	}
	for _, aid := range plan.CreatedAgents {
		ignoreMissing("agent", aid, s.st.DeleteAgent(aid))
	}
	for _, name := range plan.CreatedSkills {
		ignoreMissing("skill", name, skill.Delete(name))
	}
	for _, name := range plan.CreatedMCP {
		off := false
		_, err := s.st.UpdateConnector(name, &off, nil, nil)
		note("mcp", name, err)
	}
	for _, prev := range plan.PrevAgents {
		cur, err := s.st.GetAgent(prev.ID)
		if err != nil {
			note("agent", prev.ID, err)
			continue
		}
		cur.Instructions = prev.Instructions
		cur.Model = prev.Model
		cur.LLMProvider = prev.LLMProvider
		cur.OrchestrationMode = prev.OrchestrationMode
		cur.Enabled = prev.Enabled
		_, err = s.st.UpdateAgent(*cur)
		note("agent", prev.ID, err)
	}
	for _, prev := range plan.PrevTeams {
		cur, err := s.st.GetTeam(prev.ID)
		if err != nil {
			note("team", prev.ID, err)
			continue
		}
		cur.Name = prev.Name
		cur.LeadAgentID = prev.LeadAgentID
		_, err = s.st.UpdateTeam(*cur)
		note("team", prev.ID, err)
	}
	for _, prev := range plan.PrevSkills {
		_, err := skill.Create(prev.Name, prev.Body)
		note("skill", prev.Name, err)
	}
	for _, prev := range plan.PrevMCP {
		en := prev.Enabled
		ep := prev.Endpoint
		_, err := s.st.UpdateConnector(prev.Name, &en, &ep, nil)
		note("mcp", prev.Name, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(errs) > 0 {
		src.Status = StatusDone
		src.UpdatedAt = time.Now().UTC()
		failJob(j, strings.Join(errs, "; "))
		s.putJob(j)
		return PublicJob(j), errors.New(j.Error)
	}
	src.Status = StatusRolledBack
	src.UpdatedAt = time.Now().UTC()
	j.Status = StatusDone
	j.Progress = 100
	j.UpdatedAt = time.Now().UTC()
	markStep(j, "rollback", StatusDone, src.ID)
	s.putJob(j)
	return PublicJob(j), nil
}

func (s *Service) remember(j *Job) {
	if s == nil || j == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putJob(j)
}

func (s *Service) agentsByKey(tenantID string) map[string]*store.Agent {
	out := map[string]*store.Agent{}
	if s == nil || s.st == nil {
		return out
	}
	tenantID = store.NormalizeTenant(tenantID)
	for _, a := range s.st.ListAgents() {
		if a != nil && store.SameTenant(a.TenantID, tenantID) {
			cp := *a
			out[a.AgentKey] = &cp
		}
	}
	return out
}

func (s *Service) agentByKeyAny(key string) *store.Agent {
	if s == nil || s.st == nil {
		return nil
	}
	for _, a := range s.st.ListAgents() {
		if a != nil && a.AgentKey == key {
			cp := *a
			return &cp
		}
	}
	return nil
}

func (s *Service) agentKeyIDs(tenantID string) map[string]string {
	out := map[string]string{}
	for k, a := range s.agentsByKey(tenantID) {
		out[k] = a.ID
	}
	return out
}

func (s *Service) teamsByName(tenantID string) map[string]*store.Team {
	out := map[string]*store.Team{}
	if s == nil || s.st == nil {
		return out
	}
	tenantID = store.NormalizeTenant(tenantID)
	for _, t := range s.st.ListTeams() {
		if t != nil && store.SameTenant(t.TenantID, tenantID) {
			cp := *t
			out[strings.ToLower(t.Name)] = &cp
		}
	}
	return out
}

func (s *Service) skillSet() map[string]bool {
	out := map[string]bool{}
	list, err := skill.List()
	if err != nil {
		return out
	}
	for _, info := range list {
		out[info.Name] = true
	}
	return out
}

func (s *Service) mcpSet() map[string]bool {
	out := map[string]bool{}
	for name := range s.mcpByName() {
		out[name] = true
	}
	return out
}

func (s *Service) mcpByName() map[string]*store.ConnectorRecord {
	out := map[string]*store.ConnectorRecord{}
	if s == nil || s.st == nil {
		return out
	}
	for _, rec := range s.st.ListConnectors() {
		if rec != nil {
			cp := *rec
			out[rec.Name] = &cp
		}
	}
	return out
}

func uniqueAgentKey(have map[string]*store.Agent, key string) string {
	base := key
	for i := 2; i < 1000; i++ {
		n := base + "-" + strconv.Itoa(i)
		if _, ok := have[n]; !ok {
			return n
		}
	}
	return base + "-" + newID()
}

func uniqueTeamName(have map[string]*store.Team, name string) string {
	for i := 2; i < 1000; i++ {
		n := name + " (" + strconv.Itoa(i) + ")"
		if _, ok := have[strings.ToLower(n)]; !ok {
			return n
		}
	}
	return name + " (" + newID() + ")"
}

func uniqueSkillName(have map[string]bool, name string) string {
	for i := 2; i < 1000; i++ {
		n := name + "-" + strconv.Itoa(i)
		if !have[n] {
			return n
		}
	}
	return name + "-imp"
}

func uniqueMCPName(have map[string]*store.ConnectorRecord, name string) string {
	for i := 2; i < 1000; i++ {
		n := name + "-" + strconv.Itoa(i)
		if _, ok := have[n]; !ok {
			return n
		}
	}
	return name + "-imp"
}

func setOf(ids []string) map[string]bool {
	out := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out[id] = true
		}
	}
	return out
}

func newJob(kind string) *Job {
	now := time.Now().UTC()
	return &Job{
		ID: "pe_" + newID(), Kind: kind, Status: StatusPending, Steps: []Step{},
		Report: emptyReport(), CreatedAt: now, UpdatedAt: now,
	}
}

func failJob(j *Job, msg string) {
	j.Status = StatusFailed
	j.Error = msg
	j.UpdatedAt = time.Now().UTC()
	markStep(j, "error", StatusFailed, msg)
}

func markStep(j *Job, name, status, detail string) {
	j.Steps = append(j.Steps, Step{Name: name, Status: status, Detail: detail})
	j.UpdatedAt = time.Now().UTC()
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}
