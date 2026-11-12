// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package scheduleimport

import (
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// RollbackResult counts import_source_ids rows for one tenant.
// DryRun does not delete. Names from the denylist are not included.
type RollbackResult struct {
	DryRun        bool `json:"dry_run"`
	DeniedTenants int  `json:"denied_tenants"`
	Mappings      int  `json:"mappings"`
}

// Rollback deletes import_source_ids rows for this exact tenant id.
// It does not delete channel config, cron jobs, or another tenant's mappings.
// DryRun counts the rows and writes nothing. A denied tenant is refused
// before any delete.
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
	rows, err := st.ListImportSources(tenant)
	if err != nil {
		return nil, err
	}
	if dryRun {
		return &RollbackResult{DryRun: true, Mappings: len(rows)}, nil
	}
	n, err := st.DeleteImportSources(tenant)
	if err != nil {
		return nil, err
	}
	return &RollbackResult{DryRun: false, Mappings: n}, nil
}
