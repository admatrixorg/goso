// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package episodicimport loads GoClaw episodic_summaries from a JSONL export
// into goso memories (kind episodic). It does not open a GoClaw database,
// does not read channel_contacts, and does not change the goso schema.
//
// Each kept row is stored under its own archived session, id
// gc-sess:<sha256> of the Goso agent and the GoClaw session_key, because
// SaveSummary overwrites the latest episodic note in a session. The memory
// id is gc-ep:<source uuid>. Inserts use ON CONFLICT (id) DO NOTHING.
//
// customer_key is goclaw:<user_id> when user_id is non-empty, and empty only
// when user_id is empty. A non-empty user_id is never stored with an empty
// key. Contact matching is a later command.
package episodicimport

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Skip reason keys printed by --dry-run. team/scope, customer-private, and
// missing-relation-endpoint stay in the report. This table has no team_id
// or custom_scope column; a JSON object that carries one is still counted.
const (
	ReasonExpired   = "expired"
	ReasonCustomer  = "customer-private"
	ReasonTeamScope = "team/scope"
	ReasonUnmapped  = "unmapped"
	ReasonRelation  = "missing-relation-endpoint"
	ReasonInvalid   = "invalid"
	ReasonNative    = "native-exists"
)

// ErrDenied is returned before any write when the source tenant or the
// destination tenant is on the denylist. The error text has no tenant name.
var ErrDenied = errors.New("tenant is denied")

var uuidRE = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// DB is the goso store surface this importer writes. PostgresStore embeds
// SQLiteStore, so both satisfy it. There is no live GoClaw connection.
type DB interface {
	GetAgent(string) (*store.Agent, error)
	GetSession(string) (*store.Session, error)
	GetMemory(string) (*store.Memory, error)
	InsertGCSession(store.Session) (bool, error)
	InsertGCMemory(store.Memory) (bool, error)
}

// Options is one run. SourceTenant and DestTenant are required. Deny is the
// local denylist and must be non-empty. Now is the expiry clock. Zero means
// time.Now. Customers is the shared user_id to customer_key map.
type Options struct {
	Tenants      TenantMap
	Agents       AgentMap
	Customers    map[string]string
	Deny         Denylist
	SourceTenant string
	DestTenant   string
	Now          time.Time
	DryRun       bool
}

// SkipCounts is one tenant's skip tally. Every key is always present.
type SkipCounts struct {
	Expired                 int `json:"expired"`
	CustomerPrivate         int `json:"customer-private"`
	TeamScope               int `json:"team/scope"`
	Unmapped                int `json:"unmapped"`
	MissingRelationEndpoint int `json:"missing-relation-endpoint"`
	Invalid                 int `json:"invalid"`
	NativeExists            int `json:"native-exists"`
}

func (s SkipCounts) total() int {
	return s.Expired + s.CustomerPrivate + s.TeamScope + s.Unmapped + s.MissingRelationEndpoint + s.Invalid + s.NativeExists
}

func (s *SkipCounts) add(reason string) {
	switch reason {
	case ReasonExpired:
		s.Expired++
	case ReasonCustomer:
		s.CustomerPrivate++
	case ReasonTeamScope:
		s.TeamScope++
	case ReasonUnmapped:
		s.Unmapped++
	case ReasonRelation:
		s.MissingRelationEndpoint++
	case ReasonNative:
		s.NativeExists++
	default:
		s.Invalid++
	}
}

// TenantReport is one source tenant. Sessions is how many archived sessions
// this run will create (dry-run) or did create (apply).
type TenantReport struct {
	SourceTenant string     `json:"source_tenant"`
	DestTenant   string     `json:"dest_tenant,omitempty"`
	Slug         string     `json:"slug,omitempty"`
	Table        string     `json:"table"`
	Read         int        `json:"read"`
	Write        int        `json:"write"`
	Already      int        `json:"already"`
	Sessions     int        `json:"sessions"`
	Skipped      SkipCounts `json:"skipped"`
}

// Report is the whole run. Tenants is sorted by source tenant.
// DeniedTenants is a count. Denylist names are not fields of this struct.
type Report struct {
	DryRun        bool           `json:"dry_run"`
	Table         string         `json:"table,omitempty"`
	DeniedTenants int            `json:"denied_tenants"`
	Tenants       []TenantReport `json:"tenants,omitempty"`
}

// ConfigError is a bad export or mapping file. The text does not include a body.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func config(msg string) error { return &ConfigError{Msg: msg} }

func configf(format string, args ...any) error {
	return &ConfigError{Msg: fmt.Sprintf(format, args...)}
}

type decision struct {
	source   string
	dest     string
	slug     string
	reason   string
	write    bool
	already  bool
	session  string
	memory   string
	body     string
	customer string
	agent    string
	created  time.Time
}

// Import classifies every row, then inserts sessions and memories unless
// DryRun is set. A second call inserts nothing.
func Import(db DB, rows []Parsed, opt Options) (*Report, error) {
	if db == nil {
		return nil, errors.New("store is required")
	}
	if strings.TrimSpace(opt.SourceTenant) == "" || strings.TrimSpace(opt.DestTenant) == "" {
		return nil, config("source tenant and tenant are required")
	}
	if len(opt.Deny.ids) == 0 {
		return nil, errDenylistEmpty
	}
	entry, ok := opt.Tenants.Lookup(opt.SourceTenant)
	if !ok {
		return nil, config("source tenant is not in the tenant map")
	}
	if !strings.EqualFold(strings.TrimSpace(entry.DestTenant), strings.TrimSpace(opt.DestTenant)) {
		return nil, config("tenant does not match the tenant map")
	}
	aliases := []string{opt.SourceTenant, entry.SourceTenant, entry.Slug}
	denied := opt.Deny.DeniedCount(aliases, opt.DestTenant)
	if denied > 0 {
		return &Report{DryRun: opt.DryRun, DeniedTenants: denied}, ErrDenied
	}
	now := opt.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	seenMem := map[string]bool{}
	decided := make([]decision, 0, len(rows))
	for _, row := range rows {
		d, err := classify(db, row, opt, now, seenMem)
		if err != nil {
			return nil, err
		}
		decided = append(decided, d)
		if d.write {
			seenMem[d.memory] = true
		}
	}

	sections := map[string]*TenantReport{}
	var order []string
	touch := func(d decision) *TenantReport {
		sec, ok := sections[d.source]
		if !ok {
			sec = &TenantReport{SourceTenant: d.source, Table: tableName}
			sections[d.source] = sec
			order = append(order, d.source)
		}
		if d.dest != "" {
			sec.DestTenant = d.dest
		}
		if d.slug != "" {
			sec.Slug = d.slug
		}
		return sec
	}

	var writes []decision
	for _, d := range decided {
		sec := touch(d)
		sec.Read++
		switch {
		case d.reason != "":
			sec.Skipped.add(d.reason)
		case d.already:
			sec.Already++
		case d.write:
			writes = append(writes, d)
		default:
			sec.Skipped.Invalid++
		}
	}

	sessionAt := map[string]time.Time{}
	sessionDest := map[string]string{}
	sessionAgent := map[string]string{}
	sessionSource := map[string]string{}
	for _, d := range writes {
		cur, ok := sessionAt[d.session]
		if !ok || d.created.Before(cur) {
			sessionAt[d.session] = d.created
		}
		sessionDest[d.session] = d.dest
		sessionAgent[d.session] = d.agent
		sessionSource[d.session] = d.source
	}
	create := map[string]bool{}
	for id := range sessionAt {
		_, err := db.GetSession(id)
		if errors.Is(err, store.ErrNotFound) {
			create[id] = true
			continue
		}
		if err != nil {
			return nil, err
		}
	}

	inserted := map[string]int{}
	conflicted := map[string]int{}
	made := map[string]int{}
	if opt.DryRun {
		for _, d := range writes {
			inserted[d.source]++
		}
		for id, ok := range create {
			if ok {
				made[sessionSource[id]]++
			}
		}
	} else {
		ids := make([]string, 0, len(sessionAt))
		for id := range sessionAt {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if !create[id] {
				continue
			}
			ok, err := db.InsertGCSession(store.Session{
				ID:        id,
				TenantID:  sessionDest[id],
				AgentID:   sessionAgent[id],
				CreatedAt: sessionAt[id],
			})
			if err != nil {
				return nil, err
			}
			if ok {
				made[sessionSource[id]]++
			}
		}
		sort.Slice(writes, func(i, j int) bool { return writes[i].memory < writes[j].memory })
		for _, d := range writes {
			ok, err := db.InsertGCMemory(store.Memory{
				ID:          d.memory,
				TenantID:    d.dest,
				SessionID:   d.session,
				Kind:        store.KindEpisodic,
				Body:        d.body,
				CreatedAt:   d.created,
				CustomerKey: d.customer,
			})
			if err != nil {
				return nil, err
			}
			if ok {
				inserted[d.source]++
			} else {
				conflicted[d.source]++
			}
		}
	}

	rep := &Report{DryRun: opt.DryRun, Table: tableName, DeniedTenants: 0}
	sort.Strings(order)
	for _, source := range order {
		sec := sections[source]
		sec.Write = inserted[source]
		sec.Already += conflicted[source]
		sec.Sessions = made[source]
		if sec.Read != sec.Write+sec.Already+sec.Skipped.total() {
			return nil, errors.New("report tally does not balance")
		}
		rep.Tenants = append(rep.Tenants, *sec)
	}
	if rep.Tenants == nil {
		rep.Tenants = []TenantReport{}
	}
	return rep, nil
}

func classify(db DB, row parsed, opt Options, now time.Time, seen map[string]bool) (decision, error) {
	source := strings.TrimSpace(row.TenantID)
	d := decision{source: source}
	if source == "" {
		d.reason = ReasonInvalid
		return d, nil
	}
	selected, ok := opt.Tenants.Lookup(opt.SourceTenant)
	if !ok {
		d.reason = ReasonUnmapped
		return d, nil
	}
	n := normID(source)
	if n != normID(selected.SourceTenant) && n != normID(selected.Slug) && n != normID(opt.SourceTenant) {
		d.reason = ReasonUnmapped
		return d, nil
	}
	d.slug = selected.Slug
	d.dest = strings.TrimSpace(opt.DestTenant)
	if !store.TenantOK(d.dest) {
		d.reason = ReasonInvalid
		d.dest = ""
		return d, nil
	}
	if row.scopeSet {
		d.reason = ReasonTeamScope
		return d, nil
	}
	if row.badExpires || row.badUser {
		d.reason = ReasonInvalid
		return d, nil
	}
	if row.expires != nil && row.expires.Before(now) {
		d.reason = ReasonExpired
		return d, nil
	}
	if !uuidRE.MatchString(strings.TrimSpace(row.ID)) || strings.TrimSpace(row.SessionKey) == "" || row.created.IsZero() {
		d.reason = ReasonInvalid
		return d, nil
	}
	body := Body(row.Summary, row.topics)
	if strings.TrimSpace(body) == "" {
		d.reason = ReasonInvalid
		return d, nil
	}
	agent, ok := opt.Agents.Lookup(row.AgentID)
	if !ok || agent.DestTenant != d.dest || strings.TrimSpace(agent.DestAgent) == "" {
		d.reason = ReasonUnmapped
		return d, nil
	}
	ag, err := db.GetAgent(agent.DestAgent)
	if errors.Is(err, store.ErrNotFound) || (err == nil && ag == nil) {
		d.reason = ReasonUnmapped
		return d, nil
	}
	if err != nil {
		return d, err
	}
	if !store.SameTenant(ag.TenantID, d.dest) {
		d.reason = ReasonUnmapped
		return d, nil
	}
	key := CustomerKey(row.userID, opt.Customers)
	if row.userID != "" && key == "" {
		return d, errors.New("refusing empty customer_key")
	}
	d.customer = key
	d.agent = ag.ID
	d.session = SessionID(ag.ID, strings.TrimSpace(row.SessionKey))
	d.memory = MemoryID(row.ID)
	d.body = body
	d.created = row.created.UTC()
	sess, err := db.GetSession(d.session)
	if err == nil && sess != nil && !store.SameTenant(sess.TenantID, d.dest) {
		d.reason = ReasonUnmapped
		return d, nil
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return d, err
	}
	if seen[d.memory] {
		d.already = true
		return d, nil
	}
	existing, err := db.GetMemory(d.memory)
	if err == nil && existing != nil {
		if sameMemory(existing, d) {
			d.already = true
		} else {
			d.reason = ReasonNative
		}
		return d, nil
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return d, err
	}
	d.write = true
	return d, nil
}

func sameMemory(existing *store.Memory, d decision) bool {
	if existing == nil {
		return false
	}
	return existing.Body == d.body &&
		existing.CustomerKey == d.customer &&
		existing.SessionID == d.session &&
		existing.Kind == store.KindEpisodic &&
		store.SameTenant(existing.TenantID, d.dest) &&
		existing.CreatedAt.Equal(d.created)
}
