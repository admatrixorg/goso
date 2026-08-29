// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package tenant

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/store"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrExists          = errors.New("tenant already exists")
	ErrSlug            = errors.New("slug is required")
	ErrName            = errors.New("name is required")
	ErrStatus          = errors.New("status must be active or deactivated")
	ErrMaster          = errors.New("cannot deactivate master tenant")
	ErrConfirm         = errors.New("confirm does not match")
	ErrConfirmRequired = errors.New("confirm is required")
	ErrSubject         = errors.New("subject is required")
	ErrRole            = errors.New("role must be owner, admin, member, or viewer")
	ErrSecret          = errors.New("secret-shaped value is not allowed")
	ErrMemberExists    = errors.New("member already exists")
	ErrCap             = errors.New("too many tenants")
)

const (
	StatusActive      = "active"
	StatusDeactivated = "deactivated"
	RoleOwner         = "owner"
	RoleAdmin         = "admin"
	RoleMember        = "member"
	RoleViewer        = "viewer"
	maxName           = 80
	maxSubject        = 128
	maxTenants        = 256
	maxMembers        = 64
	masterDisplayName = "Master"
)

var tokenShape = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|token=)`)

// Member is a tenant access row. Subject is a label, never a credential.
type Member struct {
	ID        string    `json:"id"`
	Subject   string    `json:"subject"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// Public is the GET row. No token/secret fields.
type Public struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Master    bool      `json:"master"`
	CreatedAt time.Time `json:"created_at"`
	Members   []Member  `json:"members,omitempty"`
}

type record struct {
	ID        string
	Name      string
	Status    string
	Master    bool
	CreatedAt time.Time
	Members   []Member
}

// Registry is an in-memory tenant lifecycle store. Master (default) is always present.
type Registry struct {
	mu   sync.Mutex
	seq  atomic.Int64
	now  func() time.Time
	rows map[string]*record
}

var (
	defaultMu  sync.Mutex
	defaultReg = New()
)

// New returns a registry that always contains the master tenant.
func New() *Registry {
	r := &Registry{rows: map[string]*record{}}
	r.rows[Default] = &record{
		ID:        Default,
		Name:      masterDisplayName,
		Status:    StatusActive,
		Master:    true,
		CreatedAt: r.clock(),
		Members:   []Member{},
	}
	return r
}

// Default is the process-wide registry used by HTTP when none is injected.
func DefaultRegistry() *Registry {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	return defaultReg
}

// SetDefault replaces the process-wide registry (tests).
func SetDefault(r *Registry) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if r == nil {
		r = New()
	}
	defaultReg = r
}

func (r *Registry) clock() time.Time {
	if r != nil && r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}

func (r *Registry) nextMemberID() string {
	v := r.seq.Add(1)
	return "tm_" + itoa(v)
}

func itoa(v int64) string {
	if v <= 0 {
		return "0"
	}
	b := make([]byte, 0, 20)
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if max > 0 && len(s) > max {
		return s[:max]
	}
	return s
}

func looksSecret(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if tokenShape.MatchString(s) {
		return true
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "authorization") && strings.Contains(lower, "bearer") {
		return true
	}
	return false
}

func roleOK(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleMember, RoleViewer:
		return true
	default:
		return false
	}
}

func statusOK(st string) bool {
	return st == StatusActive || st == StatusDeactivated
}

func (rec *record) public(withMembers bool) Public {
	p := Public{
		ID:        rec.ID,
		Name:      rec.Name,
		Status:    rec.Status,
		Master:    rec.Master,
		CreatedAt: rec.CreatedAt,
	}
	if withMembers {
		mem := make([]Member, len(rec.Members))
		copy(mem, rec.Members)
		p.Members = mem
	} else {
		p.Members = nil
	}
	return p
}

// ConfirmOK is true when typed matches id (slug) or name.
func ConfirmOK(typed string, row Public) bool {
	v := strings.TrimSpace(typed)
	if v == "" {
		return false
	}
	return v == row.ID || v == row.Name
}

// List returns registered tenants, newest-last by name, filtered by q on id/name/status.
func (r *Registry) List(q string) []Public {
	if r == nil {
		return []Public{}
	}
	q = strings.ToLower(strings.TrimSpace(q))
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Public, 0, len(r.rows))
	for _, rec := range r.rows {
		p := rec.public(false)
		if q != "" {
			hay := strings.ToLower(p.ID + " " + p.Name + " " + p.Status)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Master != out[j].Master {
			return out[i].Master
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Get returns a registered tenant with members.
func (r *Registry) Get(id string) (Public, error) {
	if r == nil {
		return Public{}, ErrNotFound
	}
	id = store.NormalizeTenant(id)
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.rows[id]
	if rec == nil {
		return Public{}, ErrNotFound
	}
	return rec.public(true), nil
}

// Context returns the public view for a request tenant id.
// Unregistered ids stay writable (SPEC 071 implicit isolation) and are not listed.
func (r *Registry) Context(id string) Public {
	id = store.NormalizeTenant(id)
	if r != nil {
		r.mu.Lock()
		rec := r.rows[id]
		r.mu.Unlock()
		if rec != nil {
			return rec.public(false)
		}
	}
	return Public{
		ID:     id,
		Name:   id,
		Status: StatusActive,
		Master: id == Default,
	}
}

// Master returns the master tenant public row.
func (r *Registry) Master() Public {
	p, err := r.Get(Default)
	if err != nil {
		return Public{ID: Default, Name: masterDisplayName, Status: StatusActive, Master: true}
	}
	p.Members = nil
	return p
}

// Writable reports whether mutations for tid should proceed.
// Unregistered tenants stay writable so 071 header isolation still works.
func (r *Registry) Writable(id string) bool {
	if r == nil {
		return true
	}
	id = store.NormalizeTenant(id)
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.rows[id]
	if rec == nil {
		return true
	}
	return rec.Status == StatusActive
}

// Create registers a tenant. Slug is the isolation id (X-Goso-Tenant).
func (r *Registry) Create(slug, name string) (Public, error) {
	if r == nil {
		return Public{}, ErrNotFound
	}
	slug = strings.TrimSpace(slug)
	name = clip(name, maxName)
	if looksSecret(slug) || looksSecret(name) {
		return Public{}, ErrSecret
	}
	if !store.TenantOK(slug) {
		return Public{}, ErrSlug
	}
	if name == "" {
		return Public{}, ErrName
	}
	slug = store.NormalizeTenant(slug)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.rows) >= maxTenants {
		return Public{}, ErrCap
	}
	if r.rows[slug] != nil {
		return Public{}, ErrExists
	}
	rec := &record{
		ID:        slug,
		Name:      name,
		Status:    StatusActive,
		Master:    slug == Default,
		CreatedAt: r.clock(),
		Members:   []Member{},
	}
	r.rows[slug] = rec
	return rec.public(true), nil
}

// SetStatus changes active/deactivated. Deactivating master is refused.
// Confirm must match slug or name.
func (r *Registry) SetStatus(id, status, confirm string) (Public, error) {
	if r == nil {
		return Public{}, ErrNotFound
	}
	status = strings.TrimSpace(strings.ToLower(status))
	if !statusOK(status) {
		return Public{}, ErrStatus
	}
	id = store.NormalizeTenant(id)
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.rows[id]
	if rec == nil {
		return Public{}, ErrNotFound
	}
	if status == StatusDeactivated && rec.Master {
		return Public{}, ErrMaster
	}
	if status == StatusDeactivated {
		if strings.TrimSpace(confirm) == "" {
			return Public{}, ErrConfirmRequired
		}
		if !ConfirmOK(confirm, rec.public(false)) {
			return Public{}, ErrConfirm
		}
	}
	rec.Status = status
	return rec.public(true), nil
}

// AddMember records an access row. Subject is a label, not a token.
func (r *Registry) AddMember(id, subject, role string) (Public, Member, error) {
	if r == nil {
		return Public{}, Member{}, ErrNotFound
	}
	subject = clip(subject, maxSubject)
	role = strings.TrimSpace(strings.ToLower(role))
	if looksSecret(subject) {
		return Public{}, Member{}, ErrSecret
	}
	if subject == "" || !subjectOK(subject) {
		return Public{}, Member{}, ErrSubject
	}
	if role == "" {
		role = RoleMember
	}
	if !roleOK(role) {
		return Public{}, Member{}, ErrRole
	}
	id = store.NormalizeTenant(id)
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.rows[id]
	if rec == nil {
		return Public{}, Member{}, ErrNotFound
	}
	if len(rec.Members) >= maxMembers {
		return Public{}, Member{}, ErrCap
	}
	for _, m := range rec.Members {
		if strings.EqualFold(m.Subject, subject) {
			return Public{}, Member{}, ErrMemberExists
		}
	}
	mem := Member{ID: r.nextMemberID(), Subject: subject, Role: role, CreatedAt: r.clock()}
	rec.Members = append(rec.Members, mem)
	return rec.public(true), mem, nil
}

// SetMemberRole updates an access row.
func (r *Registry) SetMemberRole(id, memberID, role string) (Public, Member, error) {
	if r == nil {
		return Public{}, Member{}, ErrNotFound
	}
	role = strings.TrimSpace(strings.ToLower(role))
	if !roleOK(role) {
		return Public{}, Member{}, ErrRole
	}
	id = store.NormalizeTenant(id)
	memberID = strings.TrimSpace(memberID)
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.rows[id]
	if rec == nil {
		return Public{}, Member{}, ErrNotFound
	}
	for i := range rec.Members {
		if rec.Members[i].ID == memberID {
			rec.Members[i].Role = role
			return rec.public(true), rec.Members[i], nil
		}
	}
	return Public{}, Member{}, ErrNotFound
}

// RemoveMember deletes an access row. Confirm must match subject or member id.
func (r *Registry) RemoveMember(id, memberID, confirm string) (Public, Member, error) {
	if r == nil {
		return Public{}, Member{}, ErrNotFound
	}
	id = store.NormalizeTenant(id)
	memberID = strings.TrimSpace(memberID)
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := r.rows[id]
	if rec == nil {
		return Public{}, Member{}, ErrNotFound
	}
	for i, m := range rec.Members {
		if m.ID != memberID {
			continue
		}
		if strings.TrimSpace(confirm) == "" {
			return Public{}, Member{}, ErrConfirmRequired
		}
		if strings.TrimSpace(confirm) != m.ID && strings.TrimSpace(confirm) != m.Subject {
			return Public{}, Member{}, ErrConfirm
		}
		rec.Members = append(rec.Members[:i], rec.Members[i+1:]...)
		return rec.public(true), m, nil
	}
	return Public{}, Member{}, ErrNotFound
}

func subjectOK(s string) bool {
	if s == "" || len(s) > maxSubject {
		return false
	}
	for _, ru := range s {
		if unicode.IsLetter(ru) || unicode.IsDigit(ru) || ru == '@' || ru == '.' || ru == '_' || ru == '-' || ru == '+' {
			continue
		}
		return false
	}
	return true
}
