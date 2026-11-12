// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package memdocimport loads GoClaw memory_documents from a JSONL export into
// goso. It does not open a live database.
//
// A row with an empty user_id whose path is memory/ or a root file is written
// through vault.PutTenantAt at memory/goclaw/<goso agent>/<path>. The same
// bytes are not written twice (content hash, and a deterministic gc-vdoc id).
// A row with a non-empty user_id is a memories note of kind durable: the body
// starts with the source path, the session is archived, and customer_key is
// the mapped key or goclaw:<user_id>, never empty.
//
// _system/ is not imported. memory_chunks are not imported. team_id and
// custom_scope, when set, are counted and not imported. A tenant on the
// local denylist stops the run before any write. A target that already
// exists and differs is native-exists and is not updated.
package memdocimport

import (
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

// Store is the goso store the importer writes. PostgresStore embeds the
// SQLite methods, so it satisfies this interface.
type Store interface {
	store.StoreIface
	InsertSessionIfNew(store.Session) (bool, error)
	InsertMemoryIfNew(store.Memory) (bool, error)
	ListImportedMemdocVault(tenant string) ([]store.VaultDoc, error)
	DeleteImportedMemdocs(tenant string) (store.GCCounts, error)
}

// Options selects the mapping and whether this run writes.
// SourceTenant is required. It is the GoClaw tenant id or its slug.
// Deny, when set, refuses that tenant and any source tenant in the export
// before a write. Customers comes from a channel_contacts file.
type Options struct {
	Mapping      Mapping
	SourceTenant string
	Deny         *DenyList
	Customers    *CustomerIndex
	DryRun       bool
}

type planned struct {
	durable bool
	dest    string
	agent   string
	session string
	memory  string
	logical string
	id      string
	body    string
	when    time.Time
	key     string
}

// Import reads a JSONL export. A second call with the same export inserts
// nothing. svc is required when DryRun is false.
func Import(st Store, svc *vault.Service, r io.Reader, opt Options) (*Report, error) {
	if st == nil || r == nil {
		return nil, errConfig("store and export are required")
	}
	if !opt.DryRun && svc == nil {
		return nil, errConfig("vault dir is required")
	}
	if opt.Deny == nil {
		return nil, errConfig("denylist is missing")
	}
	maps, err := compileMap(opt.Mapping)
	if err != nil {
		return nil, err
	}
	rows, err := ReadJSONL(r)
	if err != nil {
		return nil, err
	}
	source, err := maps.resolve(opt.SourceTenant)
	if err != nil {
		if opt.Deny.Hit(opt.SourceTenant) {
			return nil, &DeniedError{Count: 1}
		}
		return nil, err
	}
	if n := countDenied(opt.Deny, source, maps, rows); n > 0 {
		return nil, &DeniedError{Count: n}
	}
	if opt.Customers == nil {
		opt.Customers = &CustomerIndex{byUser: map[string]string{}, bySender: map[string]string{}}
	}
	p := &planner{
		st:        st,
		maps:      maps,
		sourceID:  source.source,
		customers: opt.Customers,
		by:        map[string]*TenantReport{},
		names:     map[string][]string{},
		sessions:  map[string]*TenantReport{},
	}
	var want []planned
	for _, item := range rows {
		act, err := p.consider(item)
		if err != nil {
			return nil, err
		}
		if act != nil {
			want = append(want, *act)
		}
	}
	if !opt.DryRun {
		if err := apply(st, svc, want); err != nil {
			return nil, err
		}
	}
	rep := p.finish(opt.DryRun)
	if err := checkReport(rep); err != nil {
		return nil, err
	}
	return rep, nil
}

func apply(st Store, svc *vault.Service, want []planned) error {
	seen := map[string]struct{}{}
	for _, w := range want {
		if w.durable {
			if _, ok := seen[w.session]; !ok {
				seen[w.session] = struct{}{}
				if _, err := st.InsertSessionIfNew(store.Session{
					ID:        w.session,
					TenantID:  w.dest,
					AgentID:   w.agent,
					Archived:  true,
					CreatedAt: w.when,
				}); err != nil {
					return err
				}
			}
			if _, err := st.InsertMemoryIfNew(store.Memory{
				ID:          w.memory,
				TenantID:    w.dest,
				SessionID:   w.session,
				Kind:        store.KindDurable,
				Body:        w.body,
				CreatedAt:   w.when,
				CustomerKey: w.key,
			}); err != nil {
				return err
			}
			continue
		}
		if _, err := svc.PutTenantAtID(w.dest, w.logical, w.id, []byte(w.body)); err != nil {
			return err
		}
	}
	return nil
}

// CountDenied reports how many distinct tenants in this run are on the
// denylist. The number is safe to print. Tenant ids are not.
func CountDenied(deny *DenyList, token string, m Mapping, r io.Reader) (int, error) {
	if deny == nil {
		return 0, errConfig("denylist is missing")
	}
	maps, err := compileMap(m)
	if err != nil {
		return 0, err
	}
	rows, err := ReadJSONL(r)
	if err != nil {
		return 0, err
	}
	source, err := maps.resolve(token)
	if err != nil {
		if deny.Hit(token) {
			return 1, nil
		}
		return 0, err
	}
	return countDenied(deny, source, maps, rows), nil
}

func countDenied(deny *DenyList, source tenantBind, maps *loadedMap, rows []row) int {
	if deny == nil {
		return 0
	}
	seen := map[string]struct{}{}
	note := func(id string) {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" {
			seen[id] = struct{}{}
		}
	}
	if deny.Hit(source.source, source.slug, source.dest) {
		note(source.source)
	}
	for _, item := range rows {
		id := strings.TrimSpace(item.tenantID)
		if tb, ok := maps.tenants[id]; ok {
			if deny.Hit(tb.source, tb.slug, tb.dest) {
				note(tb.source)
			}
			continue
		}
		if deny.Hit(id) {
			note(id)
		}
	}
	return len(seen)
}

type planner struct {
	st        Store
	maps      *loadedMap
	sourceID  string
	customers *CustomerIndex
	by        map[string]*TenantReport
	order     []*TenantReport
	names     map[string][]string
	sessions  map[string]*TenantReport
}

func (p *planner) consider(item row) (*planned, error) {
	source := sourceKey(item.tenantID)
	sec := p.section(source)
	table := tableDocs
	if item.chunk {
		table = tableChunks
	}
	tab := sec.Tables[table]
	tab.Read++

	if strings.TrimSpace(item.tenantID) != p.sourceID {
		tab.Skipped.OtherTenant++
		return nil, nil
	}
	tb, mapped := p.maps.tenants[strings.TrimSpace(item.tenantID)]
	if !mapped {
		tab.Skipped.Unmapped++
		return nil, nil
	}
	sec.Slug = tb.slug
	sec.Dest = tb.dest
	if item.chunk {
		tab.Skipped.Chunks++
		return nil, nil
	}
	if item.teamSet || item.scopeSet {
		tab.Skipped.TeamScope++
		return nil, nil
	}
	clean, kind, ok := classify(item.path)
	if !ok {
		tab.Skipped.Path++
		return nil, nil
	}
	if kind == kindSystem {
		tab.Skipped.System++
		if !item.userSet {
			p.names[source] = append(p.names[source], systemFileName(clean))
		}
		return nil, nil
	}
	agentID := strings.TrimSpace(item.agentID)
	ab, found := p.maps.agents[agentKey(strings.TrimSpace(item.tenantID), agentID)]
	if agentID == "" || !found {
		tab.Skipped.Unmapped++
		return nil, nil
	}
	ag, err := p.st.GetAgent(ab.dest)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if ag == nil || !store.SameTenant(ag.TenantID, tb.dest) {
		tab.Skipped.Unmapped++
		return nil, nil
	}
	if !textOK(item.content) {
		tab.Skipped.Binary++
		return nil, nil
	}
	when := parseTime(item.createdAt)
	if when.IsZero() {
		when = parseTime(item.updatedAt)
	}
	if item.userSet {
		return p.planDurable(sec, tab, tb, ab.dest, clean, item, when)
	}
	return p.planVault(tab, tb, ab.dest, clean, item)
}

func (p *planner) planDurable(sec *TenantReport, tab *TableCount, tb tenantBind, agent, clean string, item row, when time.Time) (*planned, error) {
	body := clean + "\n" + item.content
	if !textOK(body) {
		tab.Skipped.Binary++
		return nil, nil
	}
	if len(item.content) > security.MaxAPIBody || len(body) > security.MaxAPIBody {
		tab.Skipped.TooLarge++
		return nil, nil
	}
	key := p.customers.Key(item.userID)
	if key == "" {
		return nil, errConfig("customer_key is empty")
	}
	id := MemoryID(agent, item.userID, clean)
	existing, err := p.st.GetMemory(id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		if existing.Body == body && existing.CustomerKey == key && store.NormalizeMemoryKind(existing.Kind) == store.KindDurable {
			tab.Skipped.Hash++
		} else {
			tab.Skipped.NativeExists++
		}
		return nil, nil
	}
	sid := SessionID(agent, item.userID)
	if _, ok := p.sessions[sid]; !ok {
		p.sessions[sid] = sec
		sess, err := p.st.GetSession(sid)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		if sess == nil {
			sec.Sessions++
		}
	}
	tab.WillWrite++
	tab.Durable++
	return &planned{
		durable: true,
		dest:    tb.dest,
		agent:   agent,
		session: sid,
		memory:  id,
		body:    body,
		when:    when,
		key:     key,
	}, nil
}

func (p *planner) planVault(tab *TableCount, tb tenantBind, agent, clean string, item row) (*planned, error) {
	if len(item.content) > security.MaxAPIBody {
		tab.Skipped.TooLarge++
		return nil, nil
	}
	logical, err := VaultPath(agent, clean)
	if err != nil {
		tab.Skipped.Path++
		return nil, nil
	}
	sum := sha256Hex([]byte(item.content))
	id := VaultID(tb.dest, agent, logical)
	doc, err := p.st.GetVaultDoc(id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if doc != nil {
		if vaultSame(doc, item.content, sum, item.hash) {
			tab.Skipped.Hash++
		} else {
			tab.Skipped.NativeExists++
		}
		return nil, nil
	}
	byPath, err := p.st.FindVaultDocByPath(tb.dest, logical)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if byPath != nil {
		if vaultSame(byPath, item.content, sum, item.hash) {
			tab.Skipped.Hash++
		} else {
			tab.Skipped.NativeExists++
		}
		return nil, nil
	}
	tab.WillWrite++
	tab.Vault++
	return &planned{
		dest:    tb.dest,
		agent:   agent,
		logical: logical,
		id:      id,
		body:    item.content,
	}, nil
}

func vaultSame(doc *store.VaultDoc, body, sum, exportHash string) bool {
	if doc == nil {
		return false
	}
	if doc.Body == body || strings.EqualFold(doc.SHA256, sum) {
		return true
	}
	exportHash = strings.TrimSpace(exportHash)
	return exportHash != "" && strings.EqualFold(doc.SHA256, exportHash)
}

func (p *planner) section(source string) *TenantReport {
	if s, ok := p.by[source]; ok {
		return s
	}
	s := newTenant(source, "", "")
	p.by[source] = s
	p.order = append(p.order, s)
	return s
}

func (p *planner) finish(dry bool) *Report {
	sort.Slice(p.order, func(i, j int) bool { return p.order[i].Source < p.order[j].Source })
	rep := &Report{DryRun: dry, Tenants: make([]TenantReport, 0, len(p.order))}
	for _, sec := range p.order {
		if dry {
			names := append([]string(nil), p.names[sec.Source]...)
			sort.Strings(names)
			sec.SystemNames = names
		}
		rep.Inserted += sec.Tables[tableDocs].WillWrite
		rep.Sessions += sec.Sessions
		rep.Tenants = append(rep.Tenants, *sec)
	}
	if rep.Tenants == nil {
		rep.Tenants = []TenantReport{}
	}
	return rep
}

func sourceKey(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "_unknown"
	}
	return id
}

var (
	_ Store = (*store.Store)(nil)
	_ Store = (*store.SQLiteStore)(nil)
	_ Store = (*store.PostgresStore)(nil)
)
