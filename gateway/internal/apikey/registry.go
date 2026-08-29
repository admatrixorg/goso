// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrName            = errors.New("name is required")
	ErrScope           = errors.New("scope is required")
	ErrUnknownScope    = errors.New("unknown scope")
	ErrSecret          = errors.New("secret-shaped value is not allowed")
	ErrExpiry          = errors.New("expiry is in the past")
	ErrConfirmRequired = errors.New("confirm is required")
	ErrConfirm         = errors.New("confirm does not match")
	ErrRevoked         = errors.New("already revoked")
	ErrCap             = errors.New("too many api keys")
)

const (
	StatusActive   = "active"
	StatusRevoked  = "revoked"
	StatusExpired  = "expired"
	ScopeAdmin     = "admin"
	ScopeRead      = "read"
	ScopeWrite     = "write"
	ScopeApprovals = "approvals"
	ScopePairing   = "pairing"
	ScopeProvision = "provision"
	secretPrefix   = "gk_"
	prefixLen      = 11
	secretBytes    = 24
	maxName        = 80
	maxTenant      = 64
	maxKeys        = 256
	maxScopes      = 8
)

var knownScopes = map[string]struct{}{
	ScopeAdmin:     {},
	ScopeRead:      {},
	ScopeWrite:     {},
	ScopeApprovals: {},
	ScopePairing:   {},
	ScopeProvision: {},
}

var tokenShape = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|token=)`)

// Public is the GET row. Hash and plaintext secret are never included.
type Public struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	TenantID   string     `json:"tenant_id"`
	Scopes     []string   `json:"scopes"`
	Status     string     `json:"status"`
	UseCount   int64      `json:"use_count"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// Created is Public plus the plaintext secret, returned only from Create.
type Created struct {
	Public
	Secret string `json:"secret"`
}

// Input is the operator create form.
type Input struct {
	Name      string
	TenantID  string
	Scopes    []string
	ExpiresAt *time.Time
}

type record struct {
	ID         string
	Name       string
	Prefix     string
	Hash       string
	TenantID   string
	Scopes     []string
	UseCount   int64
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastUsedAt time.Time
	RevokedAt  time.Time
}

// Registry stores hashed gateway API keys. Plaintext is never retained.
type Registry struct {
	mu     sync.Mutex
	seq    atomic.Int64
	now    func() time.Time
	rows   map[string]*record
	byHash map[string]string
}

var (
	defaultMu   sync.Mutex
	defaultKeys = New()
)

// New returns an empty registry.
func New() *Registry {
	return &Registry{rows: map[string]*record{}, byHash: map[string]string{}}
}

// Default is the process-wide registry used by HTTP and auth.
func Default() *Registry {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	return defaultKeys
}

// SetDefault replaces the process-wide registry (tests).
func SetDefault(r *Registry) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if r == nil {
		r = New()
	}
	defaultKeys = r
}

func (r *Registry) clock() time.Time {
	if r != nil && r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}

func (r *Registry) nextID() string {
	v := r.seq.Add(1)
	return "ak_" + strconv.FormatInt(v, 10)
}

// Create mints a key, stores only hash+prefix, and returns the secret once.
func (r *Registry) Create(in Input) (Created, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Created{}, ErrName
	}
	if len(name) > maxName || secretShaped(name) {
		return Created{}, ErrSecret
	}
	tid := strings.TrimSpace(in.TenantID)
	if tid == "" {
		tid = store.DefaultTenant
	}
	if len(tid) > maxTenant || secretShaped(tid) {
		return Created{}, ErrSecret
	}
	scopes, err := normalizeScopes(in.Scopes)
	if err != nil {
		return Created{}, err
	}
	now := r.clock()
	var exp time.Time
	if in.ExpiresAt != nil && !in.ExpiresAt.IsZero() {
		exp = in.ExpiresAt.UTC()
		if !exp.After(now) {
			return Created{}, ErrExpiry
		}
	}
	secret, prefix, hash, err := mintSecret()
	if err != nil {
		return Created{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.rows) >= maxKeys {
		return Created{}, ErrCap
	}
	id := r.nextID()
	row := &record{
		ID:        id,
		Name:      name,
		Prefix:    prefix,
		Hash:      hash,
		TenantID:  tid,
		Scopes:    scopes,
		CreatedAt: now,
		ExpiresAt: exp,
	}
	r.rows[id] = row
	r.byHash[hash] = id
	out := Created{Public: publicOf(row, now), Secret: secret}
	return out, nil
}

// List returns GET-safe inventory, newest first. q matches name/prefix/status/tenant/scope.
func (r *Registry) List(q string) []Public {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.clock()
	needle := strings.ToLower(strings.TrimSpace(q))
	out := make([]Public, 0, len(r.rows))
	for _, row := range r.rows {
		p := publicOf(row, now)
		if needle != "" && !matchPublic(p, needle) {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// Get returns one GET-safe row.
func (r *Registry) Get(id string) (Public, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[strings.TrimSpace(id)]
	if row == nil {
		return Public{}, ErrNotFound
	}
	return publicOf(row, r.clock()), nil
}

// Revoke marks a key unusable. confirm must equal id, name, or prefix.
func (r *Registry) Revoke(id, confirm string) (Public, error) {
	id = strings.TrimSpace(id)
	confirm = strings.TrimSpace(confirm)
	if confirm == "" {
		return Public{}, ErrConfirmRequired
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[id]
	if row == nil {
		return Public{}, ErrNotFound
	}
	if confirm != row.ID && confirm != row.Name && confirm != row.Prefix {
		return Public{}, ErrConfirm
	}
	now := r.clock()
	if !row.RevokedAt.IsZero() {
		return publicOf(row, now), ErrRevoked
	}
	row.RevokedAt = now
	delete(r.byHash, row.Hash)
	return publicOf(row, now), nil
}

// Accept looks up a presented bearer by hash. Valid keys bump usage.
func (r *Registry) Accept(token string) (auth.Grant, bool) {
	token = strings.TrimSpace(token)
	if token == "" || r == nil {
		return auth.Grant{}, false
	}
	sum := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(sum[:])
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.byHash[hash]
	if id == "" {
		return auth.Grant{}, false
	}
	row := r.rows[id]
	if row == nil || row.Hash != hash {
		return auth.Grant{}, false
	}
	now := r.clock()
	if !row.RevokedAt.IsZero() {
		return auth.Grant{}, false
	}
	if !row.ExpiresAt.IsZero() && !row.ExpiresAt.After(now) {
		return auth.Grant{}, false
	}
	row.UseCount++
	row.LastUsedAt = now
	scopes := append([]string(nil), row.Scopes...)
	return auth.Grant{ID: row.ID, Prefix: row.Prefix, Scopes: scopes, TenantID: row.TenantID}, true
}

func publicOf(row *record, now time.Time) Public {
	p := Public{
		ID:        row.ID,
		Name:      row.Name,
		Prefix:    row.Prefix,
		TenantID:  row.TenantID,
		Scopes:    append([]string(nil), row.Scopes...),
		Status:    StatusActive,
		UseCount:  row.UseCount,
		CreatedAt: row.CreatedAt,
	}
	if !row.ExpiresAt.IsZero() {
		exp := row.ExpiresAt
		p.ExpiresAt = &exp
	}
	if !row.LastUsedAt.IsZero() {
		used := row.LastUsedAt
		p.LastUsedAt = &used
	}
	if !row.RevokedAt.IsZero() {
		rev := row.RevokedAt
		p.RevokedAt = &rev
		p.Status = StatusRevoked
	} else if p.ExpiresAt != nil && !p.ExpiresAt.After(now) {
		p.Status = StatusExpired
	}
	return p
}

func matchPublic(p Public, needle string) bool {
	hay := strings.ToLower(strings.Join([]string{p.ID, p.Name, p.Prefix, p.TenantID, p.Status, strings.Join(p.Scopes, " ")}, " "))
	return strings.Contains(hay, needle)
}

func normalizeScopes(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, ErrScope
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.ToLower(strings.TrimSpace(raw))
		if s == "" {
			continue
		}
		if secretShaped(s) {
			return nil, ErrSecret
		}
		if _, ok := knownScopes[s]; !ok {
			return nil, ErrUnknownScope
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
		if len(out) > maxScopes {
			return nil, ErrUnknownScope
		}
	}
	if len(out) == 0 {
		return nil, ErrScope
	}
	sort.Strings(out)
	return out, nil
}

func mintSecret() (secret, prefix, hash string, err error) {
	buf := make([]byte, secretBytes)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	secret = secretPrefix + hex.EncodeToString(buf)
	if len(secret) < prefixLen {
		return "", "", "", errors.New("short secret")
	}
	prefix = secret[:prefixLen]
	sum := sha256.Sum256([]byte(secret))
	hash = hex.EncodeToString(sum[:])
	return secret, prefix, hash, nil
}

func secretShaped(v string) bool {
	if v == "" {
		return false
	}
	if strings.Contains(strings.ToLower(v), "bearer ") {
		return true
	}
	return tokenShape.MatchString(v)
}
