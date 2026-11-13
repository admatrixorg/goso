// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package skillimport loads one tenant of skills from an offline GoClaw
// export into Goso's existing SKILL.md folders. Skill text is data: this
// package never executes it. Grants, version rows, and tenant config rows
// have no Goso table; they are counted and the grant names are rewritten
// in the rename-map file.
package skillimport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NotImported lists GoClaw fields that have no home in the Goso skill folder.
func NotImported() []string {
	return []string{
		"skill_versions (no version table; latest file only)",
		"skill_agent_grants (no grant table; names rewritten in the rename map only)",
		"skill_user_grants (no grant table; names rewritten in the rename map only)",
		"skill_tenant_configs (no per-tenant skill switch; enabled=false skips the file)",
		"skills.embedding",
		"skills.owner_id",
		"skills.visibility",
		"skills.tags",
		"skills.deps",
		"skills.frontmatter",
		"skills.description",
		"skills.file_hash",
		"skills.file_size",
		"skills.created_at",
		"skills.updated_at",
		"skill_versions.changed_files",
		"skill_versions.content_hash",
		"skill_agent_grants.pinned_version",
		"skill_agent_grants.can_manage",
		"skill_agent_grants.agent_id",
		"skill_agent_grants.granted_by",
		"skill_user_grants.user_id",
		"skill_user_grants.granted_by",
	}
}

// Options is one import run. Apply false is a dry-run: the skills directory
// is not created or changed. The rename-map file is still written.
type Options struct {
	Tenant        string
	SourceTenant  string
	DenyFile      string
	SkillsDir     string
	SrcDir        string
	RenameMapPath string
	Apply         bool
	Log           Logger
}

// Logger receives reason lines. It must not be given skill bodies.
type Logger interface {
	Printf(format string, args ...any)
}

// ConfigError is a bad flag or an unreadable local map.
type ConfigError struct{ msg string }

func (e *ConfigError) Error() string { return e.msg }

func errConfig(msg string) error { return &ConfigError{msg: msg} }

// Rename is one old-to-new skill name. Apply is true when this run writes
// that folder or already owns it.
type Rename struct {
	TenantID string `json:"tenant_id"`
	SourceID string `json:"source_id"`
	Old      string `json:"old"`
	New      string `json:"new"`
	Apply    bool   `json:"apply"`
	Reason   string `json:"reason,omitempty"`
}

// GrantRename is a grant row with the skill name rewritten. The grant itself
// is not stored.
type GrantRename struct {
	TenantID      string `json:"tenant_id"`
	Kind          string `json:"kind"`
	GrantID       string `json:"grant_id"`
	SkillSourceID string `json:"skill_source_id"`
	Old           string `json:"old"`
	New           string `json:"new"`
	Reason        string `json:"reason,omitempty"`
}

// RenameFile is the file --dry-run writes and --apply checks.
type RenameFile struct {
	Renames []Rename      `json:"renames"`
	Grants  []GrantRename `json:"grants"`
}

// TenantCount is one source tenant. skills_input = inserted + skipped + rejected.
type TenantCount struct {
	TenantID            string         `json:"tenant_id"`
	SkillsInput         int            `json:"skills_input"`
	Inserted            int            `json:"inserted"`
	Skipped             int            `json:"skipped"`
	Rejected            int            `json:"rejected"`
	VersionsInput       int            `json:"versions_input"`
	VersionsNotImported int            `json:"versions_not_imported"`
	GrantsInput         int            `json:"grants_input"`
	GrantsRewritten     int            `json:"grants_rewritten"`
	GrantsNotImported   int            `json:"grants_not_imported"`
	ConfigsInput        int            `json:"configs_input"`
	ConfigsNotImported  int            `json:"configs_not_imported"`
	Reasons             map[string]int `json:"reasons,omitempty"`
}

// Report is the dry-run and apply stdout.
type Report struct {
	DryRun        bool          `json:"dry_run"`
	Tenant        string        `json:"tenant,omitempty"`
	SourceTenant  string        `json:"source_tenant,omitempty"`
	DeniedTenants int           `json:"denied_tenants"`
	Tenants       []TenantCount `json:"tenants"`
	Renames       []Rename      `json:"renames"`
	Grants        []GrantRename `json:"grants"`
	NotImported   []string      `json:"not_imported"`
}

type planned struct {
	row    SkillRow
	old    string
	new    string
	body   []byte
	sum    string
	action string // insert, skip, reject
	reason string
	mapped bool
}

// Import plans every row and writes the rename map on a dry-run.
// --apply is refused until skills are stored per tenant, so this function
// does not create skill folders. The folder writer remains for that later spec.
func Import(ex *Export, opt Options) (Report, error) {
	if ex == nil {
		ex = &Export{}
	}
	if err := validate(opt); err != nil {
		return Report{}, err
	}
	ids, err := loadDenyFile(opt.DenyFile)
	if err != nil {
		return Report{}, err
	}
	if n := deniedCount(ids, opt.Tenant, opt.SourceTenant); n > 0 {
		return Report{DryRun: !opt.Apply, DeniedTenants: n}, errConfig("tenant is denied")
	}
	if opt.Apply {
		return Report{}, errConfig("apply refused until skills are stored per tenant")
	}
	if n := deniedExportCount(ids, ex); n > 0 {
		return Report{DryRun: !opt.Apply, DeniedTenants: n}, errConfig("tenant is denied")
	}
	sm, err := loadSourceMap(opt.SkillsDir)
	if err != nil {
		return Report{}, err
	}
	names, err := existingNames(opt.SkillsDir)
	if err != nil {
		return Report{}, err
	}
	plans := planSkills(ex, opt, sm, names)
	renames, grants := buildMaps(ex, plans)
	counts := tally(ex, plans, grants)
	if err := checkBalance(counts); err != nil {
		return Report{}, err
	}
	if opt.Log != nil {
		for _, p := range plans {
			if p.reason == "" {
				continue
			}
			opt.Log.Printf("%s", MaskIdentifiers(fmt.Sprintf("skill-import source=%s reason=%s", p.row.ID, p.reason)))
		}
	}
	file := RenameFile{Renames: renames, Grants: grants}
	if opt.Apply {
		if err := matchExistingMap(opt.RenameMapPath, file); err != nil {
			return Report{}, err
		}
	}
	if err := writeRenameFile(opt.RenameMapPath, file); err != nil {
		return Report{}, err
	}
	rep := Report{
		DryRun:       !opt.Apply,
		Tenant:       strings.TrimSpace(opt.Tenant),
		SourceTenant: strings.TrimSpace(opt.SourceTenant),
		Tenants:      counts,
		Renames:      renames,
		Grants:       grants,
		NotImported:  NotImported(),
	}
	if !opt.Apply {
		return rep, nil
	}
	if err := applyPlans(opt, &sm, plans); err != nil {
		return Report{}, err
	}
	return rep, nil
}

func validate(opt Options) error {
	if strings.TrimSpace(opt.Tenant) == "" {
		return errConfig("--tenant is required")
	}
	if strings.TrimSpace(opt.SourceTenant) == "" {
		return errConfig("--source-tenant is required")
	}
	if strings.TrimSpace(opt.DenyFile) == "" {
		return errConfig("deny file is missing")
	}
	if strings.TrimSpace(opt.SkillsDir) == "" {
		return errConfig("--skills-dir is required")
	}
	if strings.TrimSpace(opt.SrcDir) == "" {
		return errConfig("--src-dir is required")
	}
	if strings.TrimSpace(opt.RenameMapPath) == "" {
		return errConfig("--rename-map is required")
	}
	info, err := os.Lstat(opt.SrcDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errConfig("source dir is not a directory")
	}
	if info, err := os.Lstat(opt.SkillsDir); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errConfig("skills dir is not a directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errConfig("skills dir is not a directory")
	}
	skillsAbs, err := filepath.Abs(opt.SkillsDir)
	if err != nil {
		return errConfig("skills dir is not a directory")
	}
	mapAbs, err := filepath.Abs(opt.RenameMapPath)
	if err != nil {
		return errConfig("rename map path is invalid")
	}
	rel, err := filepath.Rel(skillsAbs, mapAbs)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errConfig("rename map must not be inside the skills dir")
	}
	return nil
}

func planSkills(ex *Export, opt Options, sm sourceMap, taken map[string]struct{}) []planned {
	bySource := map[string]sourceEntry{}
	for _, e := range sm.Entries {
		bySource[e.TenantID+"\x00"+e.SourceID] = e
		taken[e.Name] = struct{}{}
	}
	disabled := map[string]bool{}
	for _, c := range ex.TenantConfigs {
		if c.Enabled != nil && !*c.Enabled {
			disabled[c.TenantID+"\x00"+c.SkillID] = true
		}
	}
	seen := map[string]bool{}
	tenant := strings.TrimSpace(opt.SourceTenant)
	var ready []planned
	var done []planned
	for _, row := range ex.Skills {
		p := planned{row: row, action: "reject"}
		p.old = MaskIdentifiers(oldName(row))
		idKey := row.TenantID + "\x00" + row.ID
		switch {
		case strings.TrimSpace(row.TenantID) == "":
			p.reason = "tenant-missing"
		case strings.TrimSpace(row.ID) == "":
			p.reason = "invalid-id"
		case seen[idKey]:
			p.reason = "duplicate"
		case row.TenantID != tenant:
			p.reason = "foreign-tenant"
			if slug, ok := slugify(p.old); ok {
				p.new = MaskIdentifiers(slug)
			}
		case row.IsSystem:
			p.reason = "system-skill"
			p.new, _ = slugify(p.old)
			p.new = MaskIdentifiers(p.new)
		case row.Status != "" && row.Status != "active":
			p.reason = "inactive"
			p.new, _ = slugify(p.old)
			p.new = MaskIdentifiers(p.new)
		case row.Enabled != nil && !*row.Enabled:
			p.reason = "disabled"
			p.new, _ = slugify(p.old)
			p.new = MaskIdentifiers(p.new)
		case disabled[idKey]:
			p.reason = "disabled"
			p.new, _ = slugify(p.old)
			p.new = MaskIdentifiers(p.new)
		default:
			seen[idKey] = true
			slug, ok := slugify(p.old)
			if !ok {
				p.reason = "invalid-name"
				break
			}
			p.new = MaskIdentifiers(slug)
			if e, ok := bySource[idKey]; ok {
				p.mapped = true
				p.new = e.Name
				p.old = e.Old
			}
			body, reason, err := readSkillBytes(opt.SrcDir, row.FilePath)
			if err != nil {
				p.reason = "read-failed"
				break
			}
			if reason != "" {
				p.reason = reason
				break
			}
			p.body = body
			p.sum = sumBytes(body)
			p.action = "ready"
		}
		if p.action == "reject" && p.reason != "duplicate" && p.reason != "invalid-id" && p.reason != "tenant-missing" {
			if strings.TrimSpace(row.ID) != "" {
				seen[row.TenantID+"\x00"+row.ID] = true
			}
		}
		if p.action == "ready" {
			ready = append(ready, p)
		} else {
			done = append(done, p)
		}
	}
	prior := map[string]struct{}{}
	for name := range taken {
		prior[name] = struct{}{}
	}
	ready = assign(ready, taken, prior)
	for i := range ready {
		p := &ready[i]
		if p.reason != "" && p.new == "" {
			p.action = "reject"
			continue
		}
		if p.reason == "native-exists" && !p.mapped {
			p.action = "skip"
			continue
		}
		entry, owned := bySource[p.row.TenantID+"\x00"+p.row.ID]
		if owned {
			got, err := skillFileBytes(opt.SkillsDir, entry.Name)
			switch {
			case err == nil && sumBytes(got) == entry.Sum && bytes.Equal(got, p.body):
				p.action = "skip"
				p.reason = "unchanged"
				p.new = entry.Name
			case err == nil:
				p.action = "skip"
				p.reason = "native-exists"
				p.new = entry.Name
			case errors.Is(err, os.ErrNotExist):
				p.action = "insert"
				p.reason = ""
				p.new = entry.Name
			default:
				p.action = "reject"
				p.reason = "name-taken"
				p.new = entry.Name
			}
			continue
		}
		p.action = "insert"
		p.reason = ""
	}
	done = append(done, ready...)
	return done
}

func oldName(row SkillRow) string {
	if strings.TrimSpace(row.Slug) != "" {
		return strings.TrimSpace(row.Slug)
	}
	return strings.TrimSpace(row.Name)
}

func assign(ready []planned, taken, prior map[string]struct{}) []planned {
	sort.SliceStable(ready, func(i, j int) bool {
		if ready[i].row.TenantID != ready[j].row.TenantID {
			return ready[i].row.TenantID < ready[j].row.TenantID
		}
		return ready[i].row.ID < ready[j].row.ID
	})
	for i := range ready {
		if ready[i].mapped {
			taken[ready[i].new] = struct{}{}
			continue
		}
		base := ready[i].new
		if _, exists := prior[base]; exists {
			ready[i].reason = "native-exists"
			ready[i].new = base
			continue
		}
		placed := false
		for n := 1; n <= 1000; n++ {
			name, ok := withSuffix(base, n)
			if !ok {
				break
			}
			if _, used := taken[name]; used {
				continue
			}
			ready[i].new = name
			taken[name] = struct{}{}
			placed = true
			break
		}
		if !placed {
			ready[i].new = ""
			ready[i].reason = "name-taken"
		}
	}
	return ready
}

func buildMaps(ex *Export, plans []planned) ([]Rename, []GrantRename) {
	byID := map[string]planned{}
	renames := make([]Rename, 0, len(plans))
	for _, p := range plans {
		byID[p.row.TenantID+"\x00"+p.row.ID] = p
		renames = append(renames, Rename{
			TenantID: p.row.TenantID,
			SourceID: p.row.ID,
			Old:      p.old,
			New:      p.new,
			Apply:    p.action == "insert" || p.reason == "unchanged",
			Reason:   reasonFor(p),
		})
	}
	sort.Slice(renames, func(i, j int) bool {
		if renames[i].TenantID != renames[j].TenantID {
			return renames[i].TenantID < renames[j].TenantID
		}
		return renames[i].SourceID < renames[j].SourceID
	})
	grants := make([]GrantRename, 0, len(ex.AgentGrants)+len(ex.UserGrants))
	for _, g := range ex.AgentGrants {
		grants = append(grants, grantRename(g.TenantID, "agent", g.ID, g.SkillID, byID))
	}
	for _, g := range ex.UserGrants {
		grants = append(grants, grantRename(g.TenantID, "user", g.ID, g.SkillID, byID))
	}
	sort.Slice(grants, func(i, j int) bool {
		if grants[i].TenantID != grants[j].TenantID {
			return grants[i].TenantID < grants[j].TenantID
		}
		if grants[i].Kind != grants[j].Kind {
			return grants[i].Kind < grants[j].Kind
		}
		return grants[i].GrantID < grants[j].GrantID
	})
	return renames, grants
}

func reasonFor(p planned) string {
	if p.action == "insert" {
		return ""
	}
	return p.reason
}

func grantRename(tenant, kind, grantID, skillID string, byID map[string]planned) GrantRename {
	g := GrantRename{TenantID: tenant, Kind: kind, GrantID: grantID, SkillSourceID: skillID}
	p, ok := byID[tenant+"\x00"+skillID]
	if !ok {
		g.Reason = "skill-missing"
		return g
	}
	g.Old = p.old
	if p.action == "insert" || p.reason == "unchanged" {
		g.New = p.new
		return g
	}
	g.Reason = p.reason
	if p.reason == "foreign-tenant" && p.new != "" {
		g.New = p.new
	}
	return g
}

func tally(ex *Export, plans []planned, grants []GrantRename) []TenantCount {
	idx := map[string]*TenantCount{}
	var order []string
	get := func(id string) *TenantCount {
		if id == "" {
			id = "(none)"
		}
		if c, ok := idx[id]; ok {
			return c
		}
		c := &TenantCount{TenantID: id, Reasons: map[string]int{}}
		idx[id] = c
		order = append(order, id)
		return c
	}
	for _, p := range plans {
		c := get(p.row.TenantID)
		c.SkillsInput++
		switch p.action {
		case "insert":
			c.Inserted++
		case "skip":
			c.Skipped++
			if p.reason != "" {
				c.Reasons[p.reason]++
			}
		default:
			c.Rejected++
			if p.reason != "" {
				c.Reasons[p.reason]++
			}
		}
	}
	for _, v := range ex.Versions {
		c := get(v.TenantID)
		c.VersionsInput++
		c.VersionsNotImported++
	}
	for _, g := range grants {
		c := get(g.TenantID)
		c.GrantsInput++
		c.GrantsNotImported++
		if g.New != "" {
			c.GrantsRewritten++
		}
	}
	for _, cfg := range ex.TenantConfigs {
		c := get(cfg.TenantID)
		c.ConfigsInput++
		c.ConfigsNotImported++
	}
	sort.Strings(order)
	out := make([]TenantCount, 0, len(order))
	for _, id := range order {
		c := idx[id]
		if len(c.Reasons) == 0 {
			c.Reasons = nil
		}
		out = append(out, *c)
	}
	return out
}

func checkBalance(counts []TenantCount) error {
	for _, c := range counts {
		if c.SkillsInput != c.Inserted+c.Skipped+c.Rejected {
			return errConfig("tenant skill counts do not balance")
		}
		if c.VersionsNotImported != c.VersionsInput {
			return errConfig("version counts do not balance")
		}
		if c.GrantsNotImported != c.GrantsInput || c.GrantsRewritten > c.GrantsInput {
			return errConfig("grant counts do not balance")
		}
		if c.ConfigsNotImported != c.ConfigsInput {
			return errConfig("config counts do not balance")
		}
		sum := 0
		for _, n := range c.Reasons {
			sum += n
		}
		if sum != c.Skipped+c.Rejected {
			return errConfig("reason counts do not balance")
		}
	}
	return nil
}

func matchExistingMap(path string, file RenameFile) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errConfig("rename map is unreadable")
	}
	var got RenameFile
	if err := json.Unmarshal(raw, &got); err != nil {
		return errConfig("rename map does not match this export")
	}
	wantRaw, err := json.Marshal(stableRename(file))
	if err != nil {
		return err
	}
	gotRaw, err := json.Marshal(stableRename(got))
	if err != nil {
		return err
	}
	if string(wantRaw) != string(gotRaw) {
		return errConfig("rename map does not match this export")
	}
	return nil
}

func normalizeRename(file RenameFile) RenameFile {
	if file.Renames == nil {
		file.Renames = []Rename{}
	}
	if file.Grants == nil {
		file.Grants = []GrantRename{}
	}
	return file
}

// stableRename drops per-run reasons so a second apply still matches the
// map written by dry-run. Names and the apply bit stay.
func stableRename(file RenameFile) RenameFile {
	file = normalizeRename(file)
	renames := append([]Rename(nil), file.Renames...)
	for i := range renames {
		renames[i].Reason = ""
	}
	grants := append([]GrantRename(nil), file.Grants...)
	for i := range grants {
		grants[i].Reason = ""
	}
	file.Renames = renames
	file.Grants = grants
	return file
}

func writeRenameFile(path string, file RenameFile) error {
	file = stableRename(file)
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return writeAtomic(path, raw)
}

func applyPlans(opt Options, sm *sourceMap, plans []planned) error {
	needDir := false
	for _, p := range plans {
		if p.action == "insert" {
			needDir = true
			break
		}
	}
	if !needDir {
		return nil
	}
	if err := os.MkdirAll(opt.SkillsDir, 0o755); err != nil {
		return err
	}
	owned := map[string]sourceEntry{}
	for _, e := range sm.Entries {
		owned[e.TenantID+"\x00"+e.SourceID] = e
	}
	changed := false
	for _, p := range plans {
		if p.action != "insert" {
			continue
		}
		key := p.row.TenantID + "\x00" + p.row.ID
		if _, ok := owned[key]; !ok {
			sm.Entries = append(sm.Entries, sourceEntry{
				TenantID: p.row.TenantID,
				SourceID: p.row.ID,
				Old:      p.old,
				Name:     p.new,
				Sum:      p.sum,
			})
			changed = true
		}
	}
	if changed {
		sort.Slice(sm.Entries, func(i, j int) bool {
			if sm.Entries[i].TenantID != sm.Entries[j].TenantID {
				return sm.Entries[i].TenantID < sm.Entries[j].TenantID
			}
			return sm.Entries[i].SourceID < sm.Entries[j].SourceID
		})
		if err := saveSourceMap(opt.SkillsDir, *sm); err != nil {
			return err
		}
	}
	for _, p := range plans {
		if p.action != "insert" {
			continue
		}
		if err := writeNewSkill(opt.SkillsDir, p.new, p.body); err != nil {
			return err
		}
		if opt.Log != nil {
			opt.Log.Printf("%s", MaskIdentifiers(fmt.Sprintf("skill-import source=%s reason=inserted", p.row.ID)))
		}
	}
	return nil
}

// FormatReport indents a report for stdout.
func FormatReport(rep Report) ([]byte, error) {
	if rep.Renames == nil {
		rep.Renames = []Rename{}
	}
	if rep.Grants == nil {
		rep.Grants = []GrantRename{}
	}
	if rep.Tenants == nil {
		rep.Tenants = []TenantCount{}
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return nil, err
	}
	return []byte(MaskIdentifiers(string(append(raw, '\n')))), nil
}
