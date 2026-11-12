// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Context-file scopes. One table covers both. These are instruction and note
// documents, not memory rows and not vault knowledge.
const (
	ContextScopeAgent = "agent"
	ContextScopeUser  = "user"
)

// MaxContextFileBytes is the stored body cap for one file (32KiB).
const MaxContextFileBytes = 32 * 1024

// MaxContextFilesPerOwner and MaxContextBytesPerOwner bound one
// (tenant, scope, owner). A write that would pass either limit is refused
// and leaves the previous rows unchanged. Chat refuses the same set instead
// of truncating it.
const (
	MaxContextFilesPerOwner = 16
	MaxContextBytesPerOwner = 64 * 1024
)

// ErrContextInvalid is a rejected scope, name, owner, or empty body.
// ErrContextTooLarge is one body over MaxContextFileBytes.
// ErrContextOwnerCap is too many files or too many bytes for one owner.
// None of these errors carries the body.
var (
	ErrContextInvalid  = errors.New("invalid context file")
	ErrContextTooLarge = errors.New("context file too large")
	ErrContextOwnerCap = errors.New("too_large")
)

// ContextFile is one named instruction, persona, or note file.
// OwnerID is the agent id (scope agent) or the user key (scope user).
// AgentID and UserKey are filled on the way out so callers do not guess.
type ContextFile struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Scope     string    `json:"scope"`
	OwnerID   string    `json:"-"`
	AgentID   string    `json:"agent_id,omitempty"`
	UserKey   string    `json:"user_key,omitempty"`
	Name      string    `json:"name"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UserKeyOK reports an opaque user id: letters, digits, dot, underscore, hyphen; 1–64.
// It is not an email address and not a display name.
func UserKeyOK(id string) bool { return TenantOK(id) }

func normalizeContextFile(in ContextFile) (ContextFile, error) {
	f := in
	f.TenantID = NormalizeTenant(f.TenantID)
	f.Scope = strings.ToLower(strings.TrimSpace(f.Scope))
	f.Name = strings.TrimSpace(f.Name)
	f.Body = strings.TrimSpace(f.Body)
	f.OwnerID = strings.TrimSpace(f.OwnerID)
	if f.OwnerID == "" {
		switch f.Scope {
		case ContextScopeAgent:
			f.OwnerID = strings.TrimSpace(f.AgentID)
		case ContextScopeUser:
			f.OwnerID = strings.TrimSpace(f.UserKey)
		}
	}
	if f.Scope != ContextScopeAgent && f.Scope != ContextScopeUser {
		return ContextFile{}, ErrContextInvalid
	}
	if !contextNameOK(f.Name) || !contextOwnerOK(f.Scope, f.OwnerID) || f.Body == "" {
		return ContextFile{}, ErrContextInvalid
	}
	if len(f.Body) > MaxContextFileBytes {
		return ContextFile{}, ErrContextTooLarge
	}
	f.AgentID = ""
	f.UserKey = ""
	return f, nil
}

func contextNameOK(name string) bool {
	if name == "" || len(name) > 64 || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return false
	}
	for i, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		if i > 0 && (r == '.' || r == '_' || r == '-') {
			continue
		}
		return false
	}
	return true
}

func contextOwnerOK(scope, owner string) bool {
	if owner == "" || len(owner) > 128 {
		return false
	}
	if scope == ContextScopeUser {
		return UserKeyOK(owner)
	}
	return !strings.ContainsAny(owner, "\x00")
}

func contextFileKey(tenant, scope, owner, name string) string {
	return tenant + "\x00" + scope + "\x00" + owner + "\x00" + name
}

// contextOwnerFits reports whether storing body at name keeps this owner
// within the file count and the byte total. An existing name is replaced,
// not added. The caller writes nothing when this returns an error.
func contextOwnerFits(files []ContextFile, name, body string) error {
	count := 0
	total := 0
	found := false
	for _, f := range files {
		count++
		if f.Name == name {
			found = true
			total += len(body)
			continue
		}
		total += len(f.Body)
	}
	if !found {
		count++
		total += len(body)
	}
	if count > MaxContextFilesPerOwner || total > MaxContextBytesPerOwner {
		return ErrContextOwnerCap
	}
	return nil
}

// ContextFilesOverCap reports a set that already exceeds the per-owner cap.
// Chat uses it so legacy rows are not trimmed into the prompt.
func ContextFilesOverCap(files []*ContextFile) error {
	count := 0
	total := 0
	for _, f := range files {
		if f == nil {
			continue
		}
		count++
		total += len(f.Body)
	}
	if count > MaxContextFilesPerOwner || total > MaxContextBytesPerOwner {
		return ErrContextOwnerCap
	}
	return nil
}

func presentContext(f *ContextFile) *ContextFile {
	if f == nil {
		return nil
	}
	cp := *f
	cp.AgentID = ""
	cp.UserKey = ""
	switch cp.Scope {
	case ContextScopeAgent:
		cp.AgentID = cp.OwnerID
	case ContextScopeUser:
		cp.UserKey = cp.OwnerID
	}
	return &cp
}

func (s *Store) contextOwnerLocked(f ContextFile) error {
	if f.Scope != ContextScopeAgent {
		return nil
	}
	a, ok := s.agents[f.OwnerID]
	if !ok || a == nil || !SameTenant(a.TenantID, f.TenantID) {
		return ErrNotFound
	}
	return nil
}

// PutContextFile inserts or replaces the body for one tenant, scope, owner, and name.
func (s *Store) PutContextFile(in ContextFile) (*ContextFile, error) {
	f, err := normalizeContextFile(in)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.contextOwnerLocked(f); err != nil {
		return nil, err
	}
	if s.contextFiles == nil {
		s.contextFiles = map[string]*ContextFile{}
	}
	var existing []ContextFile
	for _, cur := range s.contextFiles {
		if cur == nil || cur.Scope != f.Scope || cur.OwnerID != f.OwnerID || !SameTenant(cur.TenantID, f.TenantID) {
			continue
		}
		existing = append(existing, *cur)
	}
	if err := contextOwnerFits(existing, f.Name, f.Body); err != nil {
		return nil, err
	}
	key := contextFileKey(f.TenantID, f.Scope, f.OwnerID, f.Name)
	now := time.Now().UTC()
	updated := now
	if !f.UpdatedAt.IsZero() {
		updated = f.UpdatedAt.UTC()
	}
	if cur := s.contextFiles[key]; cur != nil {
		cur.Body = f.Body
		cur.UpdatedAt = updated
		// A zero CreatedAt keeps the original row. The importer passes the
		// source clock; the HTTP handler leaves it zero.
		if !f.CreatedAt.IsZero() {
			cur.CreatedAt = f.CreatedAt.UTC()
		}
		return presentContext(cur), nil
	}
	f.ID = s.nextID()
	f.CreatedAt = now
	if !in.CreatedAt.IsZero() {
		f.CreatedAt = in.CreatedAt.UTC()
	}
	f.UpdatedAt = updated
	stored := f
	s.contextFiles[key] = &stored
	return presentContext(&stored), nil
}

// GetContextFile returns one file in the given tenant. A different tenant is not found.
func (s *Store) GetContextFile(tenant, scope, ownerID, name string) (*ContextFile, error) {
	f, err := normalizeContextFile(ContextFile{
		TenantID: tenant, Scope: scope, OwnerID: ownerID, Name: name, Body: "x",
	})
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	cur := s.contextFiles[contextFileKey(f.TenantID, f.Scope, f.OwnerID, f.Name)]
	if cur == nil || !SameTenant(cur.TenantID, f.TenantID) {
		return nil, ErrNotFound
	}
	return presentContext(cur), nil
}

// ListContextFiles returns one owner's files in one tenant, ordered by name.
// An empty owner is rejected so a caller cannot list every user in the tenant.
func (s *Store) ListContextFiles(tenant, scope, ownerID string) ([]*ContextFile, error) {
	probe, err := normalizeContextFile(ContextFile{
		TenantID: tenant, Scope: scope, OwnerID: ownerID, Name: "list.md", Body: "x",
	})
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []*ContextFile{}
	for _, cur := range s.contextFiles {
		if cur == nil || cur.Scope != probe.Scope || cur.OwnerID != probe.OwnerID || !SameTenant(cur.TenantID, probe.TenantID) {
			continue
		}
		out = append(out, presentContext(cur))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// DeleteContextFile removes one file. The wrong tenant or owner returns ErrNotFound
// and leaves the row in place.
func (s *Store) DeleteContextFile(tenant, scope, ownerID, name string) error {
	f, err := normalizeContextFile(ContextFile{
		TenantID: tenant, Scope: scope, OwnerID: ownerID, Name: name, Body: "x",
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := contextFileKey(f.TenantID, f.Scope, f.OwnerID, f.Name)
	if s.contextFiles[key] == nil {
		return ErrNotFound
	}
	delete(s.contextFiles, key)
	return nil
}
