// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import "github.com/mqglobal/goso/gateway/internal/store"

// Health compares the vault registry with files on disk. It does not write.
type Health struct {
	Docs          int  `json:"docs"`
	DiskFiles     int  `json:"disk_files"`
	MissingOnDisk int  `json:"missing_on_disk"`
	Unindexed     int  `json:"unindexed"`
	HashMismatch  int  `json:"hash_mismatch"`
	Stale         bool `json:"stale"`
	// QuotaRequired is set only when vault upload is on and the per-tenant
	// disk quota is missing or not a positive byte count.
	QuotaRequired bool `json:"quota_required,omitempty"`
}

// Health reports one tenant's index vs that tenant's namespace. Other tenants'
// files are not counted.
func (s *Service) Health(tenant string) (*Health, error) {
	tenant = store.NormalizeTenant(tenant)
	files, err := s.collectTenant(tenant)
	if err != nil {
		return nil, err
	}
	byRel := make(map[string]diskFile, len(files))
	for _, f := range files {
		byRel[f.Rel] = f
	}
	h := &Health{}
	seen := make(map[string]struct{})
	hidden := make(map[string]struct{})
	for _, d := range s.Store.ListVaultDocs() {
		if d == nil || !store.SameTenant(d.TenantID, tenant) {
			continue
		}
		if !served(d) {
			hidden[d.Path] = struct{}{}
			continue
		}
		h.Docs++
		seen[d.Path] = struct{}{}
		f, ok := byRel[d.Path]
		if !ok && s.legacyOwned(d) {
			h.DiskFiles++
			continue
		}
		if !ok {
			h.MissingOnDisk++
			continue
		}
		if f.Hash != d.SHA256 {
			h.HashMismatch++
		}
	}
	for rel := range byRel {
		if _, skip := hidden[rel]; skip {
			continue
		}
		if _, ok := seen[rel]; !ok {
			h.Unindexed++
		}
		h.DiskFiles++
	}
	h.Stale = h.MissingOnDisk > 0 || h.Unindexed > 0 || h.HashMismatch > 0
	h.QuotaRequired = QuotaStatus() == "quota_required"
	return h, nil
}
