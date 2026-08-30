// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package backup

import (
	"path/filepath"
	"strings"
)

// RestorePlan is the operator preview for a snapshot restore. No credentials.
type RestorePlan struct {
	Valid               bool           `json:"valid"`
	File                string         `json:"file"`
	Integrity           string         `json:"integrity"`
	Scope               string         `json:"scope"`
	Tenant              string         `json:"tenant,omitempty"`
	SecretPolicy        string         `json:"secret_policy"`
	CredentialsExcluded bool           `json:"credentials_excluded"`
	LiveApplyCLIOnly    bool           `json:"live_apply_cli_only"`
	Errors              []string       `json:"errors"`
	Warnings            []string       `json:"warnings"`
	ArchiveCounts       map[string]int `json:"archive_counts,omitempty"`
	LiveCounts          map[string]int `json:"live_counts,omitempty"`
	Actions             []string       `json:"actions"`
	Recovery            Recovery       `json:"recovery"`
	ConfirmRequired     bool           `json:"confirm_required"`
	ConfirmTarget       string         `json:"confirm_target,omitempty"`
}

// PlanRestore validates an archive and describes what restore would do.
func PlanRestore(name string) RestorePlan {
	plan := RestorePlan{
		File:                strings.TrimSpace(name),
		SecretPolicy:        SecretExcluded,
		CredentialsExcluded: true,
		LiveApplyCLIOnly:    true,
		ConfirmRequired:     true,
		ConfirmTarget:       filepath.Base(strings.TrimSpace(name)),
		Actions:             []string{},
		Errors:              []string{},
		Warnings:            []string{},
		Recovery: Recovery{
			Strategy:         "pre_restore_rename",
			PreRestoreSuffix: ".pre-restore",
			TempCleanup:      true,
			LiveApplyCLIOnly: true,
		},
	}
	man, err := ValidateArchive(name)
	if err != nil {
		plan.Valid = false
		plan.Integrity = "fail"
		plan.Errors = append(plan.Errors, err.Error())
		return plan
	}
	plan.Valid = true
	plan.Integrity = "ok"
	plan.Scope = man.Scope
	plan.Tenant = man.Tenant
	plan.ArchiveCounts = man.Counts
	plan.Recovery = man.Recovery
	plan.Actions = append(plan.Actions, "verify_temp_copy")
	plan.Actions = append(plan.Actions, "integrity_check")
	if man.Scope == ScopeTenant {
		plan.Actions = append(plan.Actions, "tenant_filter:"+man.Tenant)
	}
	plan.Warnings = append(plan.Warnings, "live apply remains CLI-only; HTTP restore is a temp drill")
	if src := DBPath(); src != "" {
		if counts, err := TableCounts(src); err == nil {
			plan.LiveCounts = counts
		}
	}
	if man.Counts != nil && man.Counts["secrets"] > 0 {
		plan.Valid = false
		plan.CredentialsExcluded = false
		plan.Errors = append(plan.Errors, "archive contains credential rows")
	}
	return plan
}

// ConfirmApply reports whether confirm matches the snapshot basename.
func ConfirmApply(file, confirm string) error {
	want := filepath.Base(strings.TrimSpace(file))
	if want == "" || strings.TrimSpace(confirm) != want {
		return ErrConfirm
	}
	return nil
}
