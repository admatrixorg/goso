// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package kgimport loads GoClaw kg_entities and kg_relations from a JSONL
// export into Goso's existing KG tables.
//
// The file is the only input. This package does not open a GoClaw
// database and does not copy GoClaw code. It does not add a column, a
// customer_key, or a vector.
//
// SPEC 181 writes a row only when user_id is empty. A relation is written
// only when both endpoint entities have an empty user_id and were kept.
// Customer-bound rows are counted and skipped. A later follow-up, after
// SPEC 182 adds customer_key, can admit those rows by changing admit.
package kgimport

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Target is the existing KG store. Inserts use ON CONFLICT (id) DO NOTHING.
// A row that already exists with different contents is left as it is.
type Target interface {
	GetAgent(string) (*store.Agent, error)
	LookupImportedKG(table, id string) (store.KGIDHit, error)
	ReadImportedKGEntity(id string) (*store.KGEntity, error)
	ReadImportedKGRelation(id string) (*store.KGRelation, error)
	InsertImportedKGEntity(store.KGEntity) (bool, error)
	InsertImportedKGRelation(store.KGRelation) (bool, error)
	CountImportedKG(tenant string) (int64, int64, error)
	DeleteImportedKG(tenant string) (int64, int64, error)
}

// Options is one run. SourceTenant and DestTenant are required and must
// match TenantMap. Deny, when it lists either of those ids, stops the run
// before any write. Customers resolves a non-empty user_id. The key is
// not printed.
type Options struct {
	SourceTenant string
	DestTenant   string
	Tenants      TenantMap
	Agents       AgentMap
	Customers    CustomerMap
	Deny         DenyList
	DryRun       bool
}

// Report is counts per source tenant and table. For each section,
// read = will_write + the sum of skipped. DeniedTenants is a count.
// The report does not name a denied tenant or a customer identifier.
type Report struct {
	DryRun        bool      `json:"dry_run"`
	DeniedTenants int       `json:"denied_tenants"`
	Tables        []Section `json:"tables"`
}

// Section is one source tenant and one table.
type Section struct {
	Table        string         `json:"table"`
	SourceTenant string         `json:"source_tenant"`
	DestTenant   string         `json:"dest_tenant,omitempty"`
	Slug         string         `json:"slug,omitempty"`
	Read         int            `json:"read"`
	WillWrite    int            `json:"will_write"`
	Skipped      map[string]int `json:"skipped"`
}

// RemoveReport is a per-tenant gc- deletion.
type RemoveReport struct {
	DestTenant string `json:"dest_tenant"`
	DryRun     bool   `json:"dry_run"`
	Entities   int    `json:"entities"`
	Relations  int    `json:"relations"`
}

// ConfigError is a bad flag or mapping file. The text has no row body.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func errConfig(msg string) error { return &ConfigError{Msg: msg} }

type planned struct {
	section string
	entity  *store.KGEntity
	rel     *store.KGRelation
}

type entityKept struct {
	present bool
	userID  string
}

// Import classifies every row, then writes unless DryRun is set.
// Entities are applied before relations. A second run inserts nothing.
func Import(w Target, ex *Export, opt Options) (*Report, error) {
	if w == nil || ex == nil {
		return nil, errConfig("export is required")
	}
	if n, err := opt.screen(); err != nil {
		if n > 0 {
			return &Report{DryRun: opt.DryRun, DeniedTenants: n, Tables: []Section{}}, err
		}
		return nil, err
	}
	rep := &Report{DryRun: opt.DryRun}
	denied := map[string]struct{}{}
	sections := map[string]*Section{}
	agents := map[string]*store.Agent{}
	seenEnt := map[string]string{}
	seenRel := map[string]string{}

	kept := map[string]map[string]entityKept{}
	var entities []planned
	for i := range ex.Entities {
		item := ex.Entities[i]
		if opt.Deny.Has(item.TenantID) {
			noteDenied(denied, item.TenantID)
			continue
		}
		sec := sectionFor(sections, item.TenantID, "kg_entities", opt.Tenants)
		sec.Read++
		plan, hold, reason, err := classifyEntity(w, item, opt, agents, seenEnt)
		if err != nil {
			return nil, errors.New("read kg failed")
		}
		if reason != "" {
			sec.Skipped[reason]++
			keptEntity(kept, item, false)
			continue
		}
		keptEntity(kept, item, true)
		if hold {
			sec.Skipped[reasonExists]++
			continue
		}
		sec.WillWrite++
		entities = append(entities, planned{section: secKey(item.TenantID, "kg_entities"), entity: plan})
	}

	var relations []planned
	for i := range ex.Relations {
		item := ex.Relations[i]
		if opt.Deny.Has(item.TenantID) {
			noteDenied(denied, item.TenantID)
			continue
		}
		sec := sectionFor(sections, item.TenantID, "kg_relations", opt.Tenants)
		sec.Read++
		plan, hold, reason, err := classifyRelation(w, item, opt, agents, kept, seenRel)
		if err != nil {
			return nil, errors.New("read kg failed")
		}
		if reason != "" {
			sec.Skipped[reason]++
			continue
		}
		if hold {
			sec.Skipped[reasonExists]++
			continue
		}
		sec.WillWrite++
		relations = append(relations, planned{section: secKey(item.TenantID, "kg_relations"), rel: plan})
	}

	rep.DeniedTenants = len(denied)
	rep.Tables = finish(sections)
	if err := balanced(rep); err != nil {
		return nil, err
	}
	if opt.DryRun {
		return rep, nil
	}
	for _, p := range entities {
		ok, err := w.InsertImportedKGEntity(*p.entity)
		if err != nil {
			return nil, errors.New("insert kg entity failed")
		}
		if !ok {
			move(sections, p.section, reasonNativeExists)
		}
	}
	for _, p := range relations {
		ok, err := w.InsertImportedKGRelation(*p.rel)
		if err != nil {
			return nil, errors.New("insert kg relation failed")
		}
		if !ok {
			move(sections, p.section, reasonNativeExists)
		}
	}
	rep.DeniedTenants = len(denied)
	rep.Tables = finish(sections)
	if err := balanced(rep); err != nil {
		return nil, err
	}
	return rep, nil
}

func noteDenied(denied map[string]struct{}, id string) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return
	}
	denied[id] = struct{}{}
}

// Remove deletes kg rows in one Goso tenant whose id starts with gc-.
// Live ids are not deleted. A dest on the denylist is refused before
// any delete.
func Remove(w Target, dest string, deny DenyList, dry bool) (*RemoveReport, error) {
	if w == nil {
		return nil, errConfig("database is required")
	}
	dest = strings.TrimSpace(dest)
	if deny.Has(dest) {
		return nil, errConfig("tenant is denied")
	}
	if !store.TenantOK(dest) {
		return nil, errConfig("dest tenant is invalid")
	}
	var ent, rel int64
	var err error
	if dry {
		ent, rel, err = w.CountImportedKG(dest)
	} else {
		ent, rel, err = w.DeleteImportedKG(dest)
	}
	if err != nil {
		return nil, errors.New("remove imported kg failed")
	}
	return &RemoveReport{DestTenant: store.NormalizeTenant(dest), DryRun: dry, Entities: int(ent), Relations: int(rel)}, nil
}

func classifyEntity(w Target, item row, opt Options, agents map[string]*store.Agent, seen map[string]string) (*store.KGEntity, bool, string, error) {
	dest, reason := tenantGate(item, opt)
	if reason != "" {
		return nil, false, reason, nil
	}
	if reason = admit(item.userID(), item.teamID(), item.customScope()); reason != "" {
		if err := requireCustomerKey(reason, item.userID(), opt.Customers); err != nil {
			return nil, false, "", err
		}
		return nil, false, reason, nil
	}
	id, ok := gcKGID(item.ID)
	prov, pok := provenanceID(item.ID)
	if !ok || !pok || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.EntityType) == "" {
		return nil, false, reasonInvalid, nil
	}
	if _, dup := seen[id]; dup {
		return nil, false, reasonExists, nil
	}
	gosoAgent, reason, err := resolveAgent(w, item, dest.Dest, opt.Agents, agents)
	if err != nil {
		return nil, false, "", err
	}
	if reason != "" {
		return nil, false, reason, nil
	}
	props, err := compactProperties(item.Properties)
	if err != nil {
		return nil, false, reasonInvalid, nil
	}
	from, created, until, err := stamps(item)
	if err != nil {
		return nil, false, reasonInvalid, nil
	}
	ent := &store.KGEntity{
		ID:         id,
		TenantID:   dest.Dest,
		Name:       strings.TrimSpace(item.Name),
		Kind:       strings.TrimSpace(item.EntityType),
		Body:       entityBody(item.Description, props),
		AgentID:    gosoAgent,
		Source:     prov,
		ValidFrom:  from,
		ValidUntil: until,
		CreatedAt:  created,
	}
	cur, err := w.ReadImportedKGEntity(id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, false, "", err
	}
	if cur != nil {
		seen[id] = cur.TenantID
		if sameEntity(cur, ent) {
			return ent, true, "", nil
		}
		// Existing contents stay. This id is not an imported endpoint.
		return nil, false, reasonNativeExists, nil
	}
	seen[id] = dest.Dest
	return ent, false, "", nil
}

func classifyRelation(w Target, item row, opt Options, agents map[string]*store.Agent, kept map[string]map[string]entityKept, seen map[string]string) (*store.KGRelation, bool, string, error) {
	dest, reason := tenantGate(item, opt)
	if reason != "" {
		return nil, false, reason, nil
	}
	// The relation's own user_id is counted here. A tenant-wide relation
	// whose endpoint was held back is missing relation endpoint, below.
	if reason = admit(item.userID(), item.teamID(), item.customScope()); reason != "" {
		if err := requireCustomerKey(reason, item.userID(), opt.Customers); err != nil {
			return nil, false, "", err
		}
		return nil, false, reason, nil
	}
	id, ok := gcKGID(item.ID)
	prov, pok := provenanceID(item.ID)
	fromID, fok := gcKGID(item.SourceEntityID)
	toID, tok := gcKGID(item.TargetEntityID)
	if !ok || !pok || !fok || !tok || strings.TrimSpace(item.RelationType) == "" {
		return nil, false, reasonInvalid, nil
	}
	if _, dup := seen[id]; dup {
		return nil, false, reasonExists, nil
	}
	if _, reason, err := resolveAgent(w, item, dest.Dest, opt.Agents, agents); err != nil {
		return nil, false, "", err
	} else if reason != "" {
		return nil, false, reason, nil
	}
	if !endpointKept(kept, sourceKey(item.TenantID), item.SourceEntityID) || !endpointKept(kept, sourceKey(item.TenantID), item.TargetEntityID) {
		return nil, false, reasonMissingEndpoint, nil
	}
	props, err := compactProperties(item.Properties)
	if err != nil {
		return nil, false, reasonInvalid, nil
	}
	from, _, until, err := stamps(item)
	if err != nil {
		return nil, false, reasonInvalid, nil
	}
	rel := &store.KGRelation{
		ID:         id,
		TenantID:   dest.Dest,
		FromID:     fromID,
		ToID:       toID,
		Rel:        strings.TrimSpace(item.RelationType),
		Body:       props,
		Source:     prov,
		ValidFrom:  from,
		ValidUntil: until,
	}
	cur, err := w.ReadImportedKGRelation(id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, false, "", err
	}
	if cur != nil {
		seen[id] = cur.TenantID
		if sameRelation(cur, rel) {
			return rel, true, "", nil
		}
		return nil, false, reasonNativeExists, nil
	}
	seen[id] = dest.Dest
	return rel, false, "", nil
}

func (opt Options) screen() (int, error) {
	if strings.TrimSpace(opt.SourceTenant) == "" || strings.TrimSpace(opt.DestTenant) == "" {
		return 0, errConfig("source tenant and tenant are required")
	}
	if len(opt.Tenants.bySource) == 0 || len(opt.Agents.byID) == 0 {
		return 0, errConfig("tenant map and agent map are required")
	}
	if len(opt.Deny.ids) == 0 {
		return 0, errConfig("deny file is empty")
	}
	n := 0
	if opt.Deny.Has(opt.SourceTenant) {
		n++
	}
	if opt.Deny.Has(opt.DestTenant) && !strings.EqualFold(strings.TrimSpace(opt.SourceTenant), strings.TrimSpace(opt.DestTenant)) {
		n++
	}
	if n > 0 {
		return n, errConfig("tenant is denied")
	}
	dest, ok := opt.Tenants.lookup(opt.SourceTenant)
	if !ok || !store.SameTenant(dest.Dest, opt.DestTenant) {
		return 0, errConfig("source tenant is not mapped to tenant")
	}
	return 0, nil
}

func requireCustomerKey(reason, userID string, customers CustomerMap) error {
	if reason != reasonCustomerPrivate {
		return nil
	}
	if ResolveCustomerKey(userID, customers) == "" {
		return errors.New("customer key is empty")
	}
	return nil
}

func tenantGate(item row, opt Options) (tenantDest, string) {
	src := sourceKey(item.TenantID)
	if src == "" || src != sourceKey(opt.SourceTenant) {
		return tenantDest{}, reasonUnmapped
	}
	dest, ok := opt.Tenants.lookup(src)
	if !ok || !store.SameTenant(dest.Dest, opt.DestTenant) {
		return tenantDest{}, reasonUnmapped
	}
	return dest, ""
}

func sameEntity(cur, want *store.KGEntity) bool {
	if cur == nil || want == nil {
		return false
	}
	if !store.SameTenant(cur.TenantID, want.TenantID) || cur.Name != want.Name || cur.Kind != want.Kind || cur.Body != want.Body || cur.AgentID != want.AgentID || cur.Source != want.Source {
		return false
	}
	if !cur.ValidFrom.Equal(want.ValidFrom) || !cur.CreatedAt.Equal(want.CreatedAt) || !sameUntil(cur.ValidUntil, want.ValidUntil) {
		return false
	}
	return true
}

func sameRelation(cur, want *store.KGRelation) bool {
	if cur == nil || want == nil {
		return false
	}
	if !store.SameTenant(cur.TenantID, want.TenantID) || cur.FromID != want.FromID || cur.ToID != want.ToID || cur.Rel != want.Rel || cur.Body != want.Body || cur.Source != want.Source {
		return false
	}
	if !cur.ValidFrom.Equal(want.ValidFrom) || !sameUntil(cur.ValidUntil, want.ValidUntil) {
		return false
	}
	return true
}

func sameUntil(a, b *time.Time) bool {
	if a == nil || a.IsZero() {
		return b == nil || b.IsZero()
	}
	if b == nil || b.IsZero() {
		return false
	}
	return a.UTC().Equal(b.UTC())
}

func resolveAgent(w Target, item row, dest string, agents AgentMap, cache map[string]*store.Agent) (string, string, error) {
	src := sourceKey(item.TenantID)
	if id, ok := canonUUID(src); ok {
		src = id
	}
	mapped, ok := agents.lookup(item.AgentID, src)
	if !ok {
		return "", reasonUnmapped, nil
	}
	ag, hit := cache[mapped.GosoID]
	if !hit {
		var err error
		ag, err = w.GetAgent(mapped.GosoID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && ag == nil) {
			cache[mapped.GosoID] = nil
			return "", reasonUnmapped, nil
		}
		if err != nil {
			return "", "", err
		}
		cache[mapped.GosoID] = ag
	}
	if ag == nil || !store.SameTenant(ag.TenantID, dest) {
		return "", reasonUnmapped, nil
	}
	return ag.ID, "", nil
}

// stamps applies null valid_from -> created_at. When created_at is null
// and valid_from is set, created_at is that same instant. Both null is
// invalid. valid_until null stays null.
func stamps(item row) (from, created time.Time, until *time.Time, err error) {
	created, createdOK, err := parseStamp(item.CreatedAt)
	if err != nil {
		return time.Time{}, time.Time{}, nil, err
	}
	from, fromOK, err := parseStamp(item.ValidFrom)
	if err != nil {
		return time.Time{}, time.Time{}, nil, err
	}
	untilT, untilOK, err := parseStamp(item.ValidUntil)
	if err != nil {
		return time.Time{}, time.Time{}, nil, err
	}
	if !fromOK {
		if !createdOK {
			return time.Time{}, time.Time{}, nil, errors.New("time")
		}
		from = created
	}
	if !createdOK {
		created = from
	}
	if untilOK {
		until = &untilT
	}
	return from, created, until, nil
}

func sourceKey(s string) string {
	if id, ok := canonUUID(s); ok {
		return id
	}
	return strings.TrimSpace(s)
}

func keptEntity(kept map[string]map[string]entityKept, item row, present bool) {
	src := sourceKey(item.TenantID)
	orig, ok := canonUUID(item.ID)
	if src == "" || !ok {
		return
	}
	if kept[src] == nil {
		kept[src] = map[string]entityKept{}
	}
	// A later duplicate does not erase a row that was already kept.
	if prev, ok := kept[src][orig]; ok && prev.present {
		return
	}
	kept[src][orig] = entityKept{present: present, userID: item.userID()}
}

// endpointKept is true only when this run kept the entity and its user_id
// is empty. A follow-up that admits customer-bound entities after SPEC 182
// has to change this check together with admit. Until then a relation
// whose endpoint was held as customer-private is missing relation endpoint.
func endpointKept(kept map[string]map[string]entityKept, sourceTenant, orig string) bool {
	id, ok := canonUUID(orig)
	if !ok {
		return false
	}
	bucket := kept[sourceTenant]
	if bucket == nil {
		return false
	}
	e, ok := bucket[id]
	return ok && e.present && strings.TrimSpace(e.userID) == ""
}

func sectionFor(sections map[string]*Section, source, table string, tenants TenantMap) *Section {
	key := secKey(source, table)
	if s, ok := sections[key]; ok {
		return s
	}
	src := sourceKey(source)
	sec := &Section{
		Table:        table,
		SourceTenant: src,
		Skipped:      zeroSkipped(),
	}
	if d, ok := tenants.lookup(src); ok {
		sec.Slug = d.Slug
		sec.DestTenant = d.Dest
	}
	sections[key] = sec
	return sec
}

func secKey(source, table string) string {
	return sourceKey(source) + "\x00" + table
}

func zeroSkipped() map[string]int {
	out := make(map[string]int, len(reasonKeys))
	for _, k := range reasonKeys {
		out[k] = 0
	}
	return out
}

func finish(sections map[string]*Section) []Section {
	out := make([]Section, 0, len(sections))
	for _, s := range sections {
		if s.Skipped == nil {
			s.Skipped = zeroSkipped()
		}
		for _, k := range reasonKeys {
			if _, ok := s.Skipped[k]; !ok {
				s.Skipped[k] = 0
			}
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SourceTenant != out[j].SourceTenant {
			return out[i].SourceTenant < out[j].SourceTenant
		}
		return tableOrder(out[i].Table) < tableOrder(out[j].Table)
	})
	return out
}

func tableOrder(table string) int {
	switch table {
	case "kg_entities":
		return 0
	case "kg_relations":
		return 1
	default:
		return 2
	}
}

func balanced(rep *Report) error {
	for _, s := range rep.Tables {
		sum := s.WillWrite
		for _, n := range s.Skipped {
			sum += n
		}
		if sum != s.Read {
			return fmt.Errorf("count mismatch for %s %s", s.Table, s.SourceTenant)
		}
	}
	return nil
}

func move(sections map[string]*Section, key, reason string) {
	s := sections[key]
	if s == nil || s.WillWrite < 1 {
		return
	}
	s.WillWrite--
	s.Skipped[reason]++
}
