// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package scheduleimport

import (
	"errors"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// RollbackResult counts channel import mappings for one tenant.
// DryRun does not delete. Names from the denylist are not included.
type RollbackResult struct {
	DryRun        bool `json:"dry_run"`
	DeniedTenants int  `json:"denied_tenants"`
	Mappings      int  `json:"mappings"`
	Channels      int  `json:"channels"`
}

// Rollback deletes channel_instance mappings for this exact tenant and the
// channel_config rows those mappings name. A channel_config that is not named
// by one of those mappings is left in place, including a native catalog row
// and another tenant's imported channel. DryRun counts the rows and writes
// nothing. A denied tenant is refused before any delete.
func Rollback(st store.StoreIface, tenant string, deny map[string]struct{}, dryRun bool) (*RollbackResult, error) {
	if st == nil {
		return nil, errConfig("store is required")
	}
	tenant = strings.TrimSpace(tenant)
	if tenant == "" || !store.TenantOK(tenant) {
		return nil, errConfig("tenant id is required")
	}
	if len(deny) == 0 {
		return nil, errConfig("deny file is empty")
	}
	if n := DenialCount(deny, tenant); n > 0 {
		return &RollbackResult{DryRun: dryRun, DeniedTenants: n}, &DenyError{Count: n}
	}
	rows, err := st.ListImportSources(tenant, kindChannel)
	if err != nil {
		return nil, err
	}
	channels := 0
	for _, row := range rows {
		if importedChannelName(row.GosoID) {
			channels++
		}
	}
	if dryRun {
		return &RollbackResult{DryRun: true, Mappings: len(rows), Channels: channels}, nil
	}
	for _, row := range rows {
		if !importedChannelName(row.GosoID) {
			continue
		}
		if err := st.DeleteChannelConfig(row.GosoID); err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
	}
	n, err := st.DeleteImportSources(tenant, kindChannel)
	if err != nil {
		return nil, err
	}
	return &RollbackResult{DryRun: false, Mappings: n, Channels: channels}, nil
}

// importedChannelName is the qualified name this importer writes. A catalog
// name such as "telegram" is a native row and is not deleted.
func importedChannelName(name string) bool {
	name = strings.TrimSpace(name)
	return strings.Count(name, "/") >= 2 && !strings.HasPrefix(name, "/") && !strings.HasSuffix(name, "/")
}
