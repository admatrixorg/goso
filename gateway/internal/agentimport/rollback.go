// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agentimport

import (
	"errors"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Rollback removes rows this importer mapped for dest.
// Apply false counts those rows and writes nothing.
// Rows that were never mapped are not deleted. A mapped agent that is still
// the lead of some other team stops the rollback and is left in place.
// DeleteAgent also drops sessions and links that point at a removed agent.
func Rollback(st store.StoreIface, dest string, apply bool) (*Report, error) {
	if st == nil {
		return nil, &ConfigError{Msg: "dest tenant is invalid"}
	}
	dest = strings.TrimSpace(dest)
	if !store.TenantOK(dest) {
		return nil, &ConfigError{Msg: "dest tenant is invalid"}
	}
	var rep *Report
	err := withStoreLock(st, func() error {
		// Read the map that already exists. Do not create it on an empty rollback.
		maps, err := openMapper(st, false)
		if err != nil {
			return err
		}
		rows, err := maps.list(dest)
		if err != nil {
			return err
		}
		removed := &Removed{}
		if !apply {
			removed = countLive(st, dest, rows)
			rep = &Report{DryRun: true, Rollback: true, DestTenant: dest, Removed: removed}
			return nil
		}
		if err := removeMapped(st, maps, dest, rows, removed); err != nil {
			return err
		}
		rep = &Report{DryRun: false, Rollback: true, DestTenant: dest, Removed: removed}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rep, nil
}

func countLive(st store.StoreIface, dest string, rows []mapRow) *Removed {
	out := &Removed{}
	for _, row := range rows {
		switch row.Kind {
		case kindAgent:
			if a, err := st.GetAgent(row.Ref); err == nil && a != nil && store.SameTenant(a.TenantID, dest) {
				out.Agents++
			}
		case kindTeam:
			if tm, err := st.GetTeam(row.Ref); err == nil && tm != nil && store.SameTenant(tm.TenantID, dest) {
				out.Teams++
			}
		case kindMember:
			teamID, agentID, ok := parseMemberRef(row.Ref)
			if !ok {
				continue
			}
			if tm, err := st.GetTeam(teamID); err != nil || tm == nil || !store.SameTenant(tm.TenantID, dest) {
				continue
			}
			members, err := st.ListTeamMembers(teamID)
			if err != nil {
				continue
			}
			for _, m := range members {
				if m != nil && m.AgentID == agentID {
					out.TeamMembers++
					break
				}
			}
		case kindLink:
			for _, e := range parseLinkRef(row.Ref) {
				if !edgeInDest(st, dest, e[0], e[1]) {
					continue
				}
				if st.HasAgentLink(e[0], e[1]) {
					out.AgentLinks++
				}
			}
		}
	}
	return out
}

func removeMapped(st store.StoreIface, maps mapper, dest string, rows []mapRow, removed *Removed) error {
	var links, members, teams, agents []mapRow
	for _, row := range rows {
		switch row.Kind {
		case kindLink:
			links = append(links, row)
		case kindMember:
			members = append(members, row)
		case kindTeam:
			teams = append(teams, row)
		case kindAgent:
			agents = append(agents, row)
		default:
			if err := maps.del(row.DestTenant, row.SourceTable, row.SourceKey); err != nil {
				return err
			}
		}
	}
	for _, row := range links {
		guard, n, err := removeLinkRow(st, dest, row)
		if err != nil {
			return err
		}
		removed.AgentLinks += n
		if guard {
			continue
		}
		if err := maps.del(row.DestTenant, row.SourceTable, row.SourceKey); err != nil {
			return err
		}
	}
	droppedTeams := map[string]struct{}{}
	for _, row := range teams {
		tm, err := st.GetTeam(row.Ref)
		if errors.Is(err, store.ErrNotFound) || tm == nil {
			if err := maps.del(row.DestTenant, row.SourceTable, row.SourceKey); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !store.SameTenant(tm.TenantID, dest) {
			continue
		}
		if err := st.DeleteTeam(tm.ID); err != nil {
			return err
		}
		droppedTeams[tm.ID] = struct{}{}
		removed.Teams++
		if err := maps.del(row.DestTenant, row.SourceTable, row.SourceKey); err != nil {
			return err
		}
	}
	for _, row := range members {
		teamID, agentID, ok := parseMemberRef(row.Ref)
		if !ok {
			if err := maps.del(row.DestTenant, row.SourceTable, row.SourceKey); err != nil {
				return err
			}
			continue
		}
		if _, dropped := droppedTeams[teamID]; dropped {
			removed.TeamMembers++
			if err := maps.del(row.DestTenant, row.SourceTable, row.SourceKey); err != nil {
				return err
			}
			continue
		}
		tm, err := st.GetTeam(teamID)
		if errors.Is(err, store.ErrNotFound) || tm == nil {
			if err := maps.del(row.DestTenant, row.SourceTable, row.SourceKey); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !store.SameTenant(tm.TenantID, dest) {
			continue
		}
		live := false
		list, err := st.ListTeamMembers(teamID)
		if err != nil {
			return err
		}
		for _, m := range list {
			if m != nil && m.AgentID == agentID {
				live = true
				break
			}
		}
		if live {
			if err := st.RemoveTeamMember(teamID, agentID); err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			removed.TeamMembers++
		}
		if err := maps.del(row.DestTenant, row.SourceTable, row.SourceKey); err != nil {
			return err
		}
	}
	for _, row := range agents {
		a, err := st.GetAgent(row.Ref)
		if errors.Is(err, store.ErrNotFound) || a == nil {
			if err := maps.del(row.DestTenant, row.SourceTable, row.SourceKey); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !store.SameTenant(a.TenantID, dest) {
			continue
		}
		if err := st.DeleteAgent(a.ID); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return &ConfigError{Msg: "rollback stopped: imported agent is still a team lead"}
			}
			return err
		}
		removed.Agents++
		if err := maps.del(row.DestTenant, row.SourceTable, row.SourceKey); err != nil {
			return err
		}
	}
	return nil
}

func removeLinkRow(st store.StoreIface, dest string, row mapRow) (bool, int, error) {
	n := 0
	guard := false
	for _, e := range parseLinkRef(row.Ref) {
		from, ferr := st.GetAgent(e[0])
		to, terr := st.GetAgent(e[1])
		fromGone := errors.Is(ferr, store.ErrNotFound) || from == nil
		toGone := errors.Is(terr, store.ErrNotFound) || to == nil
		if ferr != nil && !errors.Is(ferr, store.ErrNotFound) {
			return false, n, ferr
		}
		if terr != nil && !errors.Is(terr, store.ErrNotFound) {
			return false, n, terr
		}
		if !fromGone && !store.SameTenant(from.TenantID, dest) {
			guard = true
			continue
		}
		if !toGone && !store.SameTenant(to.TenantID, dest) {
			guard = true
			continue
		}
		if fromGone || toGone || !st.HasAgentLink(e[0], e[1]) {
			continue
		}
		if err := st.RemoveAgentLink(e[0], e[1]); err != nil && !errors.Is(err, store.ErrNotFound) {
			return false, n, err
		}
		n++
	}
	return guard, n, nil
}

func edgeInDest(st store.StoreIface, dest, fromID, toID string) bool {
	from, err := st.GetAgent(fromID)
	if err != nil || from == nil || !store.SameTenant(from.TenantID, dest) {
		return false
	}
	to, err := st.GetAgent(toID)
	if err != nil || to == nil || !store.SameTenant(to.TenantID, dest) {
		return false
	}
	return true
}
