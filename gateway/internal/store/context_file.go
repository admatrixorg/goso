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

// MaxContextFileBytes is the stored body cap (32KiB), matching bootstrap files.
const MaxContextFileBytes = 32 * 1024

// ErrContextInvalid is a rejected scope, name, owner, or empty body.
// ErrContextTooLarge is a body over MaxContextFileBytes. Neither carries the body.
var (
	ErrContextInvalid  = errors.New("invalid context file")
	ErrContextTooLarge = errors.New("context file too large")
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
	key := contextFileKey(f.TenantID, f.Scope, f.OwnerID, f.Name)
	now := time.Now().UTC()
	if cur := s.contextFiles[key]; cur != nil {
		cur.Body = f.Body
		cur.UpdatedAt = now
		return presentContext(cur), nil
	}
	f.ID = s.nextID()
	f.CreatedAt = now
	f.UpdatedAt = now
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
