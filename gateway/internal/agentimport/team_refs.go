// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agentimport

import "github.com/mqglobal/goso/gateway/internal/store"

// TeamRefs returns the teams this tool imported into dest: GoClaw team id to
// goso team id. It only reads. A store without the import map returns an
// empty map. The vault importer uses it to keep team-scoped documents.
func TeamRefs(st store.StoreIface, dest string) (map[string]string, error) {
	out := map[string]string{}
	if st == nil {
		return out, nil
	}
	var rows []mapRow
	err := withStoreLock(st, func() error {
		m, err := openMapper(st, false)
		if err != nil {
			return err
		}
		rows, err = m.list(dest)
		return err
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.Kind == kindTeam && row.SourceTable == tableTeams && row.Ref != "" {
			out[row.SourceKey] = row.Ref
		}
	}
	return out, nil
}
