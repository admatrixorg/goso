// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package skillimport

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// RollbackReport is the rollback stdout. It counts rows. It does not name tenants.
type RollbackReport struct {
	DryRun        bool `json:"dry_run"`
	DeniedTenants int  `json:"denied_tenants"`
	Matched       int  `json:"matched"`
	Removed       int  `json:"removed"`
}

// Rollback deletes skill folders recorded in the import map for opt.Tenant.
// Without Apply it only counts. Other tenants and folders that are not in
// the map are left in place.
func Rollback(opt Options) (RollbackReport, error) {
	if err := validateRollback(opt); err != nil {
		return RollbackReport{}, err
	}
	ids, err := loadDenyFile(opt.DenyFile)
	if err != nil {
		return RollbackReport{}, err
	}
	n := deniedCount(ids, opt.Tenant)
	if n > 0 {
		return RollbackReport{DryRun: !opt.Apply, DeniedTenants: n}, errConfig("tenant is denied")
	}
	sm, err := loadSourceMap(opt.SkillsDir)
	if err != nil {
		return RollbackReport{}, err
	}
	var keep []sourceEntry
	var drop []sourceEntry
	for _, e := range sm.Entries {
		if e.TenantID == strings.TrimSpace(opt.Tenant) {
			drop = append(drop, e)
			continue
		}
		keep = append(keep, e)
	}
	rep := RollbackReport{DryRun: !opt.Apply, Matched: len(drop)}
	if !opt.Apply {
		return rep, nil
	}
	for _, e := range drop {
		if err := removeMappedSkill(opt.SkillsDir, e.Name); err != nil {
			return RollbackReport{}, err
		}
		rep.Removed++
	}
	sm.Entries = keep
	if err := saveSourceMap(opt.SkillsDir, sm); err != nil {
		return RollbackReport{}, err
	}
	return rep, nil
}

func validateRollback(opt Options) error {
	if strings.TrimSpace(opt.Tenant) == "" {
		return errConfig("--tenant is required")
	}
	if strings.TrimSpace(opt.SkillsDir) == "" {
		return errConfig("--skills-dir is required")
	}
	if strings.TrimSpace(opt.DenyFile) == "" {
		return errConfig("deny file is missing")
	}
	info, err := os.Lstat(opt.SkillsDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errConfig("skills dir is not a directory")
	}
	return nil
}

func removeMappedSkill(root, name string) error {
	if name == "" || name != strings.TrimSpace(name) || strings.Contains(name, "..") {
		return errConfig("refusing to remove a skill path")
	}
	dir := filepath.Join(root, name)
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errConfig("refusing to remove a skill path")
	}
	return os.RemoveAll(dir)
}
