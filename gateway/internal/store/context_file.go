// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Context-file scopes. One table covers all three. These are instruction and
// note documents, not memory rows and not vault knowledge.
//
// ContextScopeAgentUser is one user's file for one agent (GoClaw's
// user_context_files key: agent, user, name). Its owner_id is
// AgentUserOwner(agentID, userKey). ContextScopeUser is the tenant-wide file
// for a user key; a turn uses it only for names the agent has no per-user
// file for.
const (
	ContextScopeAgent     = "agent"
	ContextScopeUser      = "user"
	ContextScopeAgentUser = "agent_user"
)

// agentUserSep joins the agent id and the user key in an agent_user owner_id.
// UserKeyOK never accepts it, and an agent id that contains it is refused.
const agentUserSep = "/"

// AgentUserOwner is the owner_id of an agent_user file.
func AgentUserOwner(agentID, userKey string) string {
	return strings.TrimSpace(agentID) + agentUserSep + strings.TrimSpace(userKey)
}

// SplitAgentUserOwner returns the agent id and user key of an agent_user
// owner_id. ok is false for anything AgentUserOwner would not build from a
// valid agent id and user key.
func SplitAgentUserOwner(owner string) (agentID, userKey string, ok bool) {
	i := strings.LastIndex(owner, agentUserSep)
	if i <= 0 || i == len(owner)-1 {
		return "", "", false
	}
	agentID, userKey = owner[:i], owner[i+1:]
	if !contextAgentIDOK(agentID) || strings.Contains(agentID, agentUserSep) || !UserKeyOK(userKey) {
		return "", "", false
	}
	return agentID, userKey, true
}

// Context caps. GoClaw keeps context files in TEXT columns and puts every
// stored agent and per-user file into the prompt without a count or total
// limit; its only bound on one file is the 512 KiB RPC frame. Goso matches
// that by default and lets an operator tighten it:
//
//	GOSO_CONTEXT_FILE_MAX_BYTES  one body (default 512 KiB; 0 or invalid = default)
//	GOSO_CONTEXT_MAX_FILES       files per owner (default 0 = no limit)
//	GOSO_CONTEXT_MAX_BYTES       body bytes per owner (default 0 = no limit)
//
// An owner is one (tenant, scope, owner_id). A write that would pass a limit
// is refused and leaves the previous rows unchanged. Chat refuses the same set
// instead of truncating it.
const (
	DefaultContextFileMaxBytes  = 512 << 10
	DefaultContextFilesPerOwner = 0
	DefaultContextBytesPerOwner = 0
)

// ContextCaps is the limit set in force. Zero FilesPerOwner or BytesPerOwner
// is no limit. FileBytes is always positive.
type ContextCaps struct {
	FileBytes     int
	FilesPerOwner int
	BytesPerOwner int
}

// ContextLimits reads the caps from the environment on each call.
func ContextLimits() ContextCaps {
	c := ContextCaps{
		FileBytes:     envCap("GOSO_CONTEXT_FILE_MAX_BYTES", DefaultContextFileMaxBytes),
		FilesPerOwner: envCap("GOSO_CONTEXT_MAX_FILES", DefaultContextFilesPerOwner),
		BytesPerOwner: envCap("GOSO_CONTEXT_MAX_BYTES", DefaultContextBytesPerOwner),
	}
	if c.FileBytes <= 0 {
		c.FileBytes = DefaultContextFileMaxBytes
	}
	return c
}

func envCap(name string, def int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func (c ContextCaps) over(count, total int) bool {
	if c.FilesPerOwner > 0 && count > c.FilesPerOwner {
		return true
	}
	return c.BytesPerOwner > 0 && total > c.BytesPerOwner
}

// ErrContextInvalid is a rejected scope, name, owner, or empty body.
// ErrContextTooLarge is one body over ContextLimits().FileBytes.
// ErrContextOwnerCap is too many files or too many bytes for one owner.
// None of these errors carries the body.
var (
	ErrContextInvalid  = errors.New("invalid context file")
	ErrContextTooLarge = errors.New("context file too large")
	ErrContextOwnerCap = errors.New("too_large")
)

// ContextFile is one named instruction, persona, or note file.
// OwnerID is the agent id (scope agent), the user key (scope user), or
// AgentUserOwner (scope agent_user). AgentID and UserKey are filled on the
// way out so callers do not guess.
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

// UserKeyOK reports an opaque user id or a customer key from the import map.
// Letters, digits, dot, underscore, hyphen, and colon; 1–128. It is not an
// email address and not a display name. Tenant ids stay on TenantOK.
func UserKeyOK(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return false
	}
	return true
}

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
		case ContextScopeAgentUser:
			a, u := strings.TrimSpace(f.AgentID), strings.TrimSpace(f.UserKey)
			if a != "" && u != "" {
				f.OwnerID = AgentUserOwner(a, u)
			}
		}
	}
	if !ContextScopeOK(f.Scope) {
		return ContextFile{}, ErrContextInvalid
	}
	if !contextNameOK(f.Name) || !contextOwnerOK(f.Scope, f.OwnerID) || f.Body == "" {
		return ContextFile{}, ErrContextInvalid
	}
	if len(f.Body) > ContextLimits().FileBytes {
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

// ContextScopeOK reports agent, user, or agent_user.
func ContextScopeOK(scope string) bool {
	switch scope {
	case ContextScopeAgent, ContextScopeUser, ContextScopeAgentUser:
		return true
	default:
		return false
	}
}

func contextAgentIDOK(id string) bool {
	return id != "" && len(id) <= 128 && !strings.ContainsAny(id, "\x00")
}

func contextOwnerOK(scope, owner string) bool {
	switch scope {
	case ContextScopeUser:
		return UserKeyOK(owner)
	case ContextScopeAgentUser:
		_, _, ok := SplitAgentUserOwner(owner)
		return ok
	default:
		return contextAgentIDOK(owner)
	}
}

// contextOwnerAgent is the agent that must exist in the file's tenant before
// a write: the owner for scope agent, the agent half for scope agent_user.
func contextOwnerAgent(f ContextFile) string {
	switch f.Scope {
	case ContextScopeAgent:
		return f.OwnerID
	case ContextScopeAgentUser:
		a, _, _ := SplitAgentUserOwner(f.OwnerID)
		return a
	default:
		return ""
	}
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
	if ContextLimits().over(count, total) {
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
	if ContextLimits().over(count, total) {
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
	case ContextScopeAgentUser:
		cp.AgentID, cp.UserKey, _ = SplitAgentUserOwner(cp.OwnerID)
	}
	return &cp
}

func (s *Store) contextOwnerLocked(f ContextFile) error {
	id := contextOwnerAgent(f)
	if id == "" {
		return nil
	}
	a, ok := s.agents[id]
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
