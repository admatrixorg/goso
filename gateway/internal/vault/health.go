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
}

// Health reports index vs disk. allow limits registry rows (tenant). Disk files
// that are not in the allowed set count as unindexed.
func (s *Service) Health(allow func(*store.VaultDoc) bool) (*Health, error) {
	files, err := s.collectFiles()
	if err != nil {
		return nil, err
	}
	byRel := make(map[string]diskFile, len(files))
	for _, f := range files {
		byRel[f.Rel] = f
	}
	h := &Health{DiskFiles: len(files)}
	seen := make(map[string]struct{})
	for _, d := range s.List() {
		if d == nil {
			continue
		}
		if allow != nil && !allow(d) {
			continue
		}
		h.Docs++
		seen[d.Path] = struct{}{}
		f, ok := byRel[d.Path]
		if !ok {
			h.MissingOnDisk++
			continue
		}
		if f.Hash != d.SHA256 {
			h.HashMismatch++
		}
	}
	for rel := range byRel {
		if _, ok := seen[rel]; !ok {
			h.Unindexed++
		}
	}
	h.Stale = h.MissingOnDisk > 0 || h.Unindexed > 0 || h.HashMismatch > 0
	return h, nil
}
