// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package scheduleimport

import (
	"errors"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// RollbackResult counts channel import mappings for one tenant.
// DryRun does not delete. Names from the denylist are not included.
// Skipped lists reason codes only, never channel names.
type RollbackResult struct {
	DryRun        bool     `json:"dry_run"`
	DeniedTenants int      `json:"denied_tenants"`
	Mappings      int      `json:"mappings"`
	Channels      int      `json:"channels"`
	Kept          int      `json:"kept"`
	Skipped       []string `json:"skipped,omitempty"`
}

// Rollback deletes channel_instance mappings for this exact tenant and the
// channel_config rows that are still the untouched import: disabled, and with
// no channel secret stored. A row that was enabled, or that has a secret, is
// left in place and its mapping is kept. A native catalog name is not deleted.
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
	rows, err := st.ListImportSources(tenant, kindChannel)
	if err != nil {
		return nil, err
	}
	rep := &RollbackResult{DryRun: dryRun}
	for _, row := range rows {
		reason, drop, err := rollbackChannel(st, row.GosoID)
		if err != nil {
			return nil, err
		}
		if !drop {
			rep.Kept++
			rep.Skipped = append(rep.Skipped, reason)
			continue
		}
		if !dryRun {
			if importedChannelName(row.GosoID) {
				if err := st.DeleteChannelConfig(row.GosoID); err != nil && !errors.Is(err, store.ErrNotFound) {
					return nil, err
				}
			}
			if err := st.DeleteImportSource(tenant, kindChannel, row.SourceID); err != nil && !errors.Is(err, store.ErrNotFound) {
				return nil, err
			}
		}
		rep.Mappings++
		if importedChannelName(row.GosoID) {
			rep.Channels++
		}
	}
	return rep, nil
}

// rollbackChannel reports whether this mapped channel_config may be removed.
// drop is false when the row was enabled, has a stored channel secret, or is
// a native catalog name. A missing row can still drop the mapping.
func rollbackChannel(st store.StoreIface, name string) (reason string, drop bool, err error) {
	name = strings.TrimSpace(name)
	if !importedChannelName(name) {
		return "native-exists", false, nil
	}
	cfg, err := st.GetChannelConfig(name)
	if errors.Is(err, store.ErrNotFound) {
		return "", true, nil
	}
	if err != nil {
		return "", false, err
	}
	if cfg.Enabled || channelSecretSet(st, name) {
		return "edited", false, nil
	}
	return "", true, nil
}

// channelSecretSet reports a ciphertext stored for this channel name.
// The importer never writes these. Kinds match the channel secret box.
func channelSecretSet(st store.StoreIface, name string) bool {
	for _, kind := range []string{"bot", "access", "secret", "session"} {
		row, err := st.GetSecret("channel:" + name + ":" + kind)
		if err == nil && row != nil && len(row.CT) > 0 {
			return true
		}
	}
	return false
}

// importedChannelName is the qualified name this importer writes. A catalog
// name such as "telegram" is a native row and is not deleted.
func importedChannelName(name string) bool {
	name = strings.TrimSpace(name)
	return strings.Count(name, "/") >= 2 && !strings.HasPrefix(name, "/") && !strings.HasSuffix(name, "/")
}
