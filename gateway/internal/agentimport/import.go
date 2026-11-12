// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agentimport

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Options selects the one export tenant to read and the one goso tenant to write.
// Apply false is a dry-run: counts are filled and nothing is written.
type Options struct {
	SourceTenant string
	DestTenant   string
	Apply        bool
}

// Import reads exp and, when Apply is set, inserts new goso rows for --tenant.
// A second call with the same source ids inserts nothing and updates nothing.
func Import(st store.StoreIface, exp *Export, opt Options) (*Report, error) {
	if st == nil || exp == nil {
		return nil, &ConfigError{Msg: "export is empty"}
	}
	opt.SourceTenant = strings.TrimSpace(opt.SourceTenant)
	opt.DestTenant = strings.TrimSpace(opt.DestTenant)
	if opt.SourceTenant == "" || !plainToken(opt.SourceTenant) {
		return nil, &ConfigError{Msg: "source tenant is required"}
	}
	if !store.TenantOK(opt.DestTenant) {
		return nil, &ConfigError{Msg: "dest tenant is invalid"}
	}
	var rep *Report
	err := withStoreLock(st, func() error {
		maps, err := openMapper(st, opt.Apply)
		if err != nil {
			return err
		}
		im := &importer{
			st:       st,
			opt:      opt,
			exp:      exp,
			maps:     maps,
			agentID:  map[string]string{},
			agentKey: map[string]string{},
			teamID:   map[string]string{},
			teamLead: map[string]string{},
			members:  map[string]string{},
			claimMem: map[string]string{},
			links:    map[string]struct{}{},
			seen:     map[string]map[string]bool{},
			keysNow:  map[string]struct{}{},
		}
		for _, a := range st.ListAgents() {
			if a != nil && a.AgentKey != "" {
				im.keysNow[a.AgentKey] = struct{}{}
			}
		}
		if err := im.applyAgents(); err != nil {
			return err
		}
		if err := im.applyTeams(); err != nil {
			return err
		}
		if err := im.loadMembers(); err != nil {
			return err
		}
		if err := im.applyMembers(); err != nil {
			return err
		}
		if err := im.applyLinks(); err != nil {
			return err
		}
		im.countHeld()
		var buildErr error
		rep, buildErr = im.tally.report(Report{
			DryRun:       !opt.Apply,
			SourceTenant: opt.SourceTenant,
			DestTenant:   opt.DestTenant,
		})
		return buildErr
	})
	if err != nil {
		return nil, err
	}
	return rep, nil
}

type importer struct {
	st       store.StoreIface
	opt      Options
	exp      *Export
	maps     mapper
	tally    tally
	agentID  map[string]string
	agentKey map[string]string
	teamID   map[string]string
	teamLead map[string]string
	members  map[string]string
	claimMem map[string]string
	links    map[string]struct{}
	seen     map[string]map[string]bool
	keysNow  map[string]struct{}
}

func (im *importer) skip(table, tenant, reason string) {
	im.tally.add(table, tenant, actionSkipped, reason)
}

func (im *importer) reject(table, tenant, reason string) {
	im.tally.add(table, tenant, actionRejected, reason)
}

func (im *importer) inserted(table, tenant string) {
	im.tally.add(table, tenant, actionInserted, "")
}

func (im *importer) seenID(table, id string) bool {
	set := im.seen[table]
	if set == nil {
		set = map[string]bool{}
		im.seen[table] = set
	}
	if set[id] {
		return true
	}
	set[id] = true
	return false
}

func (im *importer) applyAgents() error {
	for _, row := range im.exp.rowsOf(tableAgents) {
		if err := im.applyAgent(row); err != nil {
			return err
		}
	}
	return nil
}

func (im *importer) applyAgent(row rawRow) error {
	tenant, ok := im.rowTenant(row, tableAgents)
	if !ok {
		return nil
	}
	id, ok := im.rowID(row, tableAgents, tenant)
	if !ok {
		return nil
	}
	if hit, done, err := im.alreadyMapped(tableAgents, id, tenant, kindAgent); err != nil || done {
		return err
	} else if hit.Kind == kindAgent {
		if a, ok := im.ownedAgent(hit.Ref); ok {
			im.agentID[id] = a.ID
			im.agentKey[a.AgentKey] = id
			im.keysNow[a.AgentKey] = struct{}{}
		}
		if key, good := agentKey(row); good {
			im.agentKey[key] = id
			im.keysNow[key] = struct{}{}
		}
		im.skip(tableAgents, tenant, reasonUnchanged)
		return nil
	}
	key, good := agentKey(row)
	if !good {
		im.reject(tableAgents, tenant, reasonAgentKeyInvalid)
		return nil
	}
	if _, taken := im.keysNow[key]; taken {
		im.skip(tableAgents, tenant, reasonAgentKeyTaken)
		return nil
	}
	if _, taken := im.agentKey[key]; taken {
		im.skip(tableAgents, tenant, reasonAgentKeyTaken)
		return nil
	}
	deleted, bad := isDeleted(row)
	if bad {
		im.reject(tableAgents, tenant, reasonBadValue)
		return nil
	}
	if deleted {
		im.skip(tableAgents, tenant, reasonDeleted)
		return nil
	}
	enabled, statusOK, statusReason := agentEnabled(row)
	if !statusOK {
		if statusReason == reasonDeleted {
			im.skip(tableAgents, tenant, statusReason)
		} else {
			im.reject(tableAgents, tenant, statusReason)
		}
		return nil
	}
	display, ok := boundedString(row, "display_name", 255)
	if !ok {
		im.reject(tableAgents, tenant, reasonBadValue)
		return nil
	}
	if display == overflowMark {
		im.reject(tableAgents, tenant, reasonTooLong)
		return nil
	}
	provider, ok := boundedString(row, "provider", 50)
	if !ok {
		im.reject(tableAgents, tenant, reasonBadValue)
		return nil
	}
	if provider == overflowMark {
		im.reject(tableAgents, tenant, reasonTooLong)
		return nil
	}
	model, ok := boundedString(row, "model", 200)
	if !ok {
		im.reject(tableAgents, tenant, reasonBadValue)
		return nil
	}
	if model == overflowMark {
		im.reject(tableAgents, tenant, reasonTooLong)
		return nil
	}
	front, present, goodFront := frontmatter(row)
	if !goodFront {
		im.reject(tableAgents, tenant, reasonBadValue)
		return nil
	}
	if present && strings.IndexByte(front, 0) >= 0 {
		im.reject(tableAgents, tenant, reasonBadValue)
		return nil
	}
	if len(front) > MaxFrontmatterBytes {
		im.reject(tableAgents, tenant, reasonTooLarge)
		return nil
	}
	im.agentKey[key] = id
	im.keysNow[key] = struct{}{}
	if !im.opt.Apply {
		im.agentID[id] = "planned:" + id
		im.inserted(tableAgents, tenant)
		return nil
	}
	created, err := im.st.CreateAgent(store.Agent{
		TenantID:     im.opt.DestTenant,
		AgentKey:     key,
		DisplayName:  strings.TrimSpace(display),
		Model:        strings.TrimSpace(model),
		LLMProvider:  strings.TrimSpace(provider),
		Instructions: front,
	})
	if errors.Is(err, store.ErrLiteCap) {
		return &ConfigError{Msg: "lite cap exceeded"}
	}
	if errors.Is(err, store.ErrExists) {
		im.skip(tableAgents, tenant, reasonAgentKeyTaken)
		return nil
	}
	if err != nil {
		return err
	}
	if !enabled {
		created.Enabled = false
		updated, uerr := im.st.UpdateAgent(*created)
		if uerr != nil {
			_ = im.st.DeleteAgent(created.ID)
			return uerr
		}
		created = updated
	}
	if err := im.maps.put(mapRow{
		DestTenant:  im.opt.DestTenant,
		SourceTable: tableAgents,
		SourceKey:   id,
		Kind:        kindAgent,
		Ref:         created.ID,
	}); err != nil {
		_ = im.st.DeleteAgent(created.ID)
		return err
	}
	im.agentID[id] = created.ID
	im.inserted(tableAgents, tenant)
	return nil
}

func (im *importer) applyTeams() error {
	for _, row := range im.exp.rowsOf(tableTeams) {
		if err := im.applyTeam(row); err != nil {
			return err
		}
	}
	return nil
}

func (im *importer) applyTeam(row rawRow) error {
	tenant, ok := im.rowTenant(row, tableTeams)
	if !ok {
		return nil
	}
	id, ok := im.rowID(row, tableTeams, tenant)
	if !ok {
		return nil
	}
	if hit, done, err := im.alreadyMapped(tableTeams, id, tenant, kindTeam); err != nil || done {
		return err
	} else if hit.Kind == kindTeam {
		if tm, ok := im.ownedTeam(hit.Ref); ok {
			im.teamID[id] = tm.ID
			im.teamLead[id] = tm.LeadAgentID
		}
		im.skip(tableTeams, tenant, reasonUnchanged)
		return nil
	}
	name, ok := boundedString(row, "name", 255)
	if !ok {
		im.reject(tableTeams, tenant, reasonBadValue)
		return nil
	}
	name = strings.TrimSpace(name)
	if name == overflowMark {
		im.reject(tableTeams, tenant, reasonTooLong)
		return nil
	}
	if name == "" {
		im.reject(tableTeams, tenant, reasonNameMissing)
		return nil
	}
	if !activeStatus(row) {
		if badStatus(row) {
			im.reject(tableTeams, tenant, reasonBadValue)
			return nil
		}
		im.skip(tableTeams, tenant, reasonInactive)
		return nil
	}
	leadSrc, present := jsonString(row.fields, "lead_agent_id")
	if !present {
		im.reject(tableTeams, tenant, reasonBadValue)
		return nil
	}
	leadSrc = strings.TrimSpace(leadSrc)
	leadGoso := im.agentID[leadSrc]
	if leadSrc == "" || leadGoso == "" {
		im.skip(tableTeams, tenant, reasonLeadMissing)
		return nil
	}
	if !im.opt.Apply {
		planned := "planned:" + id
		im.teamID[id] = planned
		im.teamLead[id] = leadGoso
		im.noteMember(planned, leadGoso, "lead")
		im.inserted(tableTeams, tenant)
		return nil
	}
	created, err := im.st.CreateTeam(store.Team{
		TenantID:    im.opt.DestTenant,
		Name:        name,
		LeadAgentID: leadGoso,
	})
	if errors.Is(err, store.ErrLiteCap) {
		return &ConfigError{Msg: "lite cap exceeded"}
	}
	if err != nil {
		return err
	}
	if err := im.maps.put(mapRow{
		DestTenant:  im.opt.DestTenant,
		SourceTable: tableTeams,
		SourceKey:   id,
		Kind:        kindTeam,
		Ref:         created.ID,
	}); err != nil {
		_ = im.st.DeleteTeam(created.ID)
		return err
	}
	im.teamID[id] = created.ID
	im.teamLead[id] = created.LeadAgentID
	im.noteMember(created.ID, created.LeadAgentID, "lead")
	im.inserted(tableTeams, tenant)
	return nil
}

func (im *importer) loadMembers() error {
	for _, gid := range im.teamID {
		if strings.HasPrefix(gid, "planned:") {
			continue
		}
		members, err := im.st.ListTeamMembers(gid)
		if err != nil {
			return err
		}
		for _, m := range members {
			if m == nil {
				continue
			}
			im.noteMember(gid, m.AgentID, m.Role)
		}
	}
	return nil
}

func (im *importer) applyMembers() error {
	for _, row := range im.exp.rowsOf(tableMembers) {
		if err := im.applyMember(row); err != nil {
			return err
		}
	}
	return nil
}

func (im *importer) applyMember(row rawRow) error {
	tenant, ok := im.rowTenant(row, tableMembers)
	if !ok {
		return nil
	}
	id, ok := im.rowID(row, tableMembers, tenant)
	if !ok {
		return nil
	}
	if hit, done, err := im.alreadyMapped(tableMembers, id, tenant, kindMember); err != nil || done {
		return err
	} else if hit.Kind == kindMember {
		im.skip(tableMembers, tenant, reasonUnchanged)
		return nil
	}
	role, present := jsonString(row.fields, "role")
	if !present {
		if raw, exists := row.fields["role"]; exists && !nullRaw(raw) {
			im.reject(tableMembers, tenant, reasonBadValue)
			return nil
		}
		role = "member"
	} else {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "" {
			role = "member"
		}
	}
	if role != "lead" && role != "member" {
		im.reject(tableMembers, tenant, reasonRoleUnsupported)
		return nil
	}
	teamSrc, tOK := jsonString(row.fields, "team_id")
	agentSrc, aOK := jsonString(row.fields, "agent_id")
	if !tOK || !aOK {
		im.reject(tableMembers, tenant, reasonBadValue)
		return nil
	}
	teamSrc = strings.TrimSpace(teamSrc)
	agentSrc = strings.TrimSpace(agentSrc)
	gosoTeam := im.teamID[teamSrc]
	gosoAgent := im.agentID[agentSrc]
	if teamSrc == "" || gosoTeam == "" {
		im.skip(tableMembers, tenant, reasonTeamMissing)
		return nil
	}
	if agentSrc == "" || gosoAgent == "" {
		im.skip(tableMembers, tenant, reasonAgentNotImported)
		return nil
	}
	memKey := memberRef(gosoTeam, gosoAgent)
	if have, exists := im.members[memKey]; exists {
		if claim := im.claimMem[memKey]; claim != "" {
			if have == role {
				im.skip(tableMembers, tenant, reasonUnchanged)
			} else {
				im.skip(tableMembers, tenant, reasonRoleDiffers)
			}
			return nil
		}
		if have != role {
			im.skip(tableMembers, tenant, reasonRoleDiffers)
			return nil
		}
		im.claimMem[memKey] = id
		if im.opt.Apply {
			if err := im.maps.put(mapRow{
				DestTenant:  im.opt.DestTenant,
				SourceTable: tableMembers,
				SourceKey:   id,
				Kind:        kindMember,
				Ref:         memKey,
			}); err != nil {
				return err
			}
		}
		im.inserted(tableMembers, tenant)
		return nil
	}
	if role == "lead" && im.teamLead[teamSrc] != "" && im.teamLead[teamSrc] != gosoAgent {
		im.skip(tableMembers, tenant, reasonLeadDiffers)
		return nil
	}
	if !im.opt.Apply {
		im.noteMember(gosoTeam, gosoAgent, role)
		im.claimMem[memKey] = id
		im.inserted(tableMembers, tenant)
		return nil
	}
	if _, err := im.st.AddTeamMember(store.TeamMember{TeamID: gosoTeam, AgentID: gosoAgent, Role: role}); err != nil {
		return err
	}
	if err := im.maps.put(mapRow{
		DestTenant:  im.opt.DestTenant,
		SourceTable: tableMembers,
		SourceKey:   id,
		Kind:        kindMember,
		Ref:         memKey,
	}); err != nil {
		_ = im.st.RemoveTeamMember(gosoTeam, gosoAgent)
		return err
	}
	im.noteMember(gosoTeam, gosoAgent, role)
	im.claimMem[memKey] = id
	if role == "lead" {
		im.teamLead[teamSrc] = gosoAgent
	}
	im.inserted(tableMembers, tenant)
	return nil
}

func (im *importer) applyLinks() error {
	for _, row := range im.exp.rowsOf(tableLinks) {
		if err := im.applyLink(row); err != nil {
			return err
		}
	}
	return nil
}

func (im *importer) applyLink(row rawRow) error {
	tenant, ok := im.rowTenant(row, tableLinks)
	if !ok {
		return nil
	}
	id, ok := im.rowID(row, tableLinks, tenant)
	if !ok {
		return nil
	}
	if hit, done, err := im.alreadyMapped(tableLinks, id, tenant, kindLink); err != nil || done {
		return err
	} else if hit.Kind == kindLink {
		im.skip(tableLinks, tenant, reasonUnchanged)
		return nil
	}
	if !activeStatus(row) {
		if badStatus(row) {
			im.reject(tableLinks, tenant, reasonBadValue)
			return nil
		}
		im.skip(tableLinks, tenant, reasonInactive)
		return nil
	}
	src, sOK := jsonString(row.fields, "source_agent_id")
	dst, dOK := jsonString(row.fields, "target_agent_id")
	if !sOK || !dOK {
		im.reject(tableLinks, tenant, reasonBadValue)
		return nil
	}
	src = strings.TrimSpace(src)
	dst = strings.TrimSpace(dst)
	if src == "" || dst == "" {
		im.reject(tableLinks, tenant, reasonBadValue)
		return nil
	}
	if src == dst {
		im.reject(tableLinks, tenant, reasonSelfLink)
		return nil
	}
	dir, dPresent := jsonString(row.fields, "direction")
	if !dPresent {
		if raw, exists := row.fields["direction"]; exists && !nullRaw(raw) {
			im.reject(tableLinks, tenant, reasonBadValue)
			return nil
		}
		dir = ""
	} else {
		dir = strings.ToLower(strings.TrimSpace(dir))
	}
	var pairs [][2]string
	switch dir {
	case "", "outbound":
		pairs = [][2]string{{src, dst}}
	case "inbound":
		pairs = [][2]string{{dst, src}}
	case "both", "bidirectional":
		pairs = [][2]string{{src, dst}, {dst, src}}
	default:
		im.reject(tableLinks, tenant, reasonDirectionUnknown)
		return nil
	}
	gosoPairs := make([][2]string, 0, len(pairs))
	for _, p := range pairs {
		from := im.agentID[p[0]]
		to := im.agentID[p[1]]
		if from == "" || to == "" {
			im.skip(tableLinks, tenant, reasonEndpointMissing)
			return nil
		}
		gosoPairs = append(gosoPairs, [2]string{from, to})
	}
	missing := make([][2]string, 0, len(gosoPairs))
	for _, p := range gosoPairs {
		k := memberRef(p[0], p[1])
		if _, planned := im.links[k]; planned {
			continue
		}
		if !strings.HasPrefix(p[0], "planned:") && im.st.HasAgentLink(p[0], p[1]) {
			continue
		}
		missing = append(missing, p)
	}
	if len(missing) == 0 {
		im.skip(tableLinks, tenant, reasonLinkExists)
		return nil
	}
	if !im.opt.Apply {
		for _, p := range missing {
			im.links[memberRef(p[0], p[1])] = struct{}{}
		}
		im.inserted(tableLinks, tenant)
		return nil
	}
	var created [][2]string
	for _, p := range missing {
		if err := im.st.AddAgentLink(p[0], p[1]); err != nil {
			for _, c := range created {
				_ = im.st.RemoveAgentLink(c[0], c[1])
			}
			return err
		}
		created = append(created, p)
		im.links[memberRef(p[0], p[1])] = struct{}{}
	}
	if err := im.maps.put(mapRow{
		DestTenant:  im.opt.DestTenant,
		SourceTable: tableLinks,
		SourceKey:   id,
		Kind:        kindLink,
		Ref:         linkRef(created),
	}); err != nil {
		for _, c := range created {
			_ = im.st.RemoveAgentLink(c[0], c[1])
		}
		return err
	}
	im.inserted(tableLinks, tenant)
	return nil
}

func (im *importer) countHeld() {
	for _, row := range im.exp.rows {
		switch row.table {
		case tableAgents, tableTeams, tableMembers, tableLinks:
			continue
		}
		tenant, _, bad := rowTenant(row.fields)
		if bad {
			tenant = ""
		}
		reason := reasonNotThisSpec
		switch row.table {
		case tablePermissions, tableOverrides, tableProfiles, tableShares:
			reason = reasonNeedsDecision
		}
		im.tally.hold(row.table, tenant, reason)
	}
}

func (im *importer) rowTenant(row rawRow, table string) (string, bool) {
	tenant, present, bad := rowTenant(row.fields)
	if bad {
		im.reject(table, "", reasonBadValue)
		return "", false
	}
	if !present || tenant == "" {
		im.reject(table, "", reasonTenantMissing)
		return "", false
	}
	if tenant != im.opt.SourceTenant {
		im.skip(table, tenant, reasonTenantMismatch)
		return "", false
	}
	return tenant, true
}

func (im *importer) rowID(row rawRow, table, tenant string) (string, bool) {
	id, present := jsonString(row.fields, "id")
	if !present {
		if raw, exists := row.fields["id"]; exists && !nullRaw(raw) {
			im.reject(table, tenant, reasonBadValue)
			return "", false
		}
		im.reject(table, tenant, reasonIDMissing)
		return "", false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		im.reject(table, tenant, reasonIDMissing)
		return "", false
	}
	if im.seenID(table, id) {
		im.skip(table, tenant, reasonDuplicate)
		return "", false
	}
	return id, true
}

// alreadyMapped reports a prior source id. done is true when the caller must stop.
// The returned row is set when the id was mapped and not yet reported.
func (im *importer) alreadyMapped(table, id, tenant, kind string) (mapRow, bool, error) {
	hit, ok, err := im.maps.get(im.opt.DestTenant, table, id)
	if err != nil {
		return mapRow{}, false, err
	}
	if !ok {
		return mapRow{}, false, nil
	}
	if hit.Kind != kind {
		im.skip(table, tenant, reasonUnchanged)
		return mapRow{}, true, nil
	}
	return hit, false, nil
}

func (im *importer) ownedAgent(id string) (*store.Agent, bool) {
	if id == "" || strings.HasPrefix(id, "planned:") {
		return nil, false
	}
	a, err := im.st.GetAgent(id)
	if err != nil || a == nil || !store.SameTenant(a.TenantID, im.opt.DestTenant) {
		return nil, false
	}
	return a, true
}

func (im *importer) ownedTeam(id string) (*store.Team, bool) {
	if id == "" || strings.HasPrefix(id, "planned:") {
		return nil, false
	}
	tm, err := im.st.GetTeam(id)
	if err != nil || tm == nil || !store.SameTenant(tm.TenantID, im.opt.DestTenant) {
		return nil, false
	}
	return tm, true
}

func (im *importer) noteMember(teamID, agentID, role string) {
	if teamID == "" || agentID == "" {
		return
	}
	im.members[memberRef(teamID, agentID)] = role
}

func rowTenant(fields map[string]json.RawMessage) (string, bool, bool) {
	raw, ok := fields["tenant_id"]
	if !ok {
		return "", false, false
	}
	s, ok := jsonString(fields, "tenant_id")
	if !ok {
		_ = raw
		return "", true, true
	}
	return strings.TrimSpace(s), true, false
}

const overflowMark = "\x00overflow"

func boundedString(row rawRow, key string, maxRunes int) (string, bool) {
	s, present := jsonString(row.fields, key)
	if !present {
		if _, exists := row.fields[key]; exists {
			raw := strings.TrimSpace(string(row.fields[key]))
			if raw == "null" || raw == "" {
				return "", true
			}
			return "", false
		}
		return "", true
	}
	if runeLen(s) > maxRunes {
		return overflowMark, true
	}
	return s, true
}

func agentKey(row rawRow) (string, bool) {
	s, present := jsonString(row.fields, "agent_key")
	if !present {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" || runeLen(s) > 100 {
		return "", false
	}
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r == 0 || r == '/' || r == '\\' {
			return "", false
		}
	}
	return s, true
}

func isDeleted(row rawRow) (bool, bool) {
	raw, exists := row.fields["deleted_at"]
	if !exists {
		return false, false
	}
	s, ok := jsonString(row.fields, "deleted_at")
	if !ok {
		trim := strings.TrimSpace(string(raw))
		if trim == "null" {
			return false, false
		}
		return false, true
	}
	return strings.TrimSpace(s) != "", false
}

func agentEnabled(row rawRow) (bool, bool, string) {
	s, present := jsonString(row.fields, "status")
	if !present {
		if _, exists := row.fields["status"]; exists {
			return false, false, reasonBadValue
		}
		return true, true, ""
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "active":
		return true, true, ""
	case "inactive", "disabled", "paused", "archived":
		return false, true, ""
	case "deleted":
		return false, false, reasonDeleted
	default:
		return false, false, reasonStatusUnknown
	}
}

func activeStatus(row rawRow) bool {
	s, present := jsonString(row.fields, "status")
	if !present {
		_, exists := row.fields["status"]
		return !exists
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "active":
		return true
	default:
		return false
	}
}

func badStatus(row rawRow) bool {
	if _, exists := row.fields["status"]; !exists {
		return false
	}
	_, ok := jsonString(row.fields, "status")
	return !ok
}

func frontmatter(row rawRow) (string, bool, bool) {
	s, present := jsonString(row.fields, "frontmatter")
	if present {
		return s, s != "", true
	}
	raw, exists := row.fields["frontmatter"]
	if !exists || nullRaw(raw) {
		return "", false, true
	}
	return "", false, false
}

func nullRaw(raw json.RawMessage) bool {
	trim := strings.TrimSpace(string(raw))
	return trim == "" || trim == "null"
}

func plainToken(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == ' ' {
			return false
		}
	}
	return true
}

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
