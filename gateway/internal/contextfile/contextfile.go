// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package contextfile loads tenant-scoped agent and user context files into
// the chat prompt. The feature flag defaults off. This package does not import
// channel, MCP, vault, or contact code.
package contextfile

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// ErrUnavailable is the prompt-stage failure returned to chat. The handler
// maps it to HTTP 500 with a fixed error class and does not include the cause,
// which may be a driver error.
var ErrUnavailable = errors.New("context files unavailable")

// UserHeader is the optional opaque user key for one chat turn.
const UserHeader = "X-Goso-User"

type ctxUserKey struct{}

// logPrintf is the only log line for this package. Tests replace it.
var logPrintf = log.Printf

// Enabled reports GOSO_CONTEXT_FILES=1/true/yes/on. Unset and every other value are off.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_CONTEXT_FILES"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// WithUser returns a context that carries an already-validated user key.
func WithUser(ctx context.Context, key string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxUserKey{}, strings.TrimSpace(key))
}

// UserFrom returns the user key stored by WithUser, or empty.
func UserFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	key, _ := ctx.Value(ctxUserKey{}).(string)
	return strings.TrimSpace(key)
}

// ResolveUserKey picks the header or the chat body field. Both empty is no
// user file. Both set and different, or a key that is not an opaque id, is invalid.
func ResolveUserKey(header, body string) (string, error) {
	h := strings.TrimSpace(header)
	b := strings.TrimSpace(body)
	if h != "" && b != "" && h != b {
		return "", store.ErrContextInvalid
	}
	key := h
	if key == "" {
		key = b
	}
	if key == "" {
		return "", nil
	}
	if !store.UserKeyOK(key) {
		return "", store.ErrContextInvalid
	}
	return key, nil
}

// Prompt loads agent files, then the user's files, as labeled blocks
// (name + body). User files for agent A and user U are A's per-user files
// (scope agent_user) first; a tenant-wide file of U (scope user) is added only
// for a name A has no per-user file for. Each set goes through the cap check.
// A store error is returned. A row whose tenant or owner does not match the
// request is a store error, not a skipped row.
func Prompt(st store.StoreIface, tenant, agentID, userKey string) (string, error) {
	if st == nil {
		return "", errors.New("store required")
	}
	var parts []string
	id := strings.TrimSpace(agentID)
	if id != "" {
		files, err := st.ListContextFiles(tenant, store.ContextScopeAgent, id)
		if err != nil {
			return "", err
		}
		if err := store.ContextFilesOverCap(files); err != nil {
			return "", err
		}
		blocks, err := blocksFor(files, tenant, store.ContextScopeAgent, id, "")
		if err != nil {
			return "", err
		}
		parts = append(parts, blocks...)
	}
	if key := strings.TrimSpace(userKey); key != "" {
		if !store.UserKeyOK(key) {
			return "", store.ErrContextInvalid
		}
		blocks, err := userBlocks(st, tenant, id, key)
		if err != nil {
			return "", err
		}
		parts = append(parts, blocks...)
	}
	return strings.Join(parts, "\n\n"), nil
}

// userBlocks merges agent_user files over tenant-wide user files by name.
func userBlocks(st store.StoreIface, tenant, agentID, key string) ([]string, error) {
	var own []*store.ContextFile
	if agentID != "" && !strings.Contains(agentID, "/") {
		files, err := st.ListContextFiles(tenant, store.ContextScopeAgentUser, store.AgentUserOwner(agentID, key))
		if err != nil {
			return nil, err
		}
		if err := store.ContextFilesOverCap(files); err != nil {
			return nil, err
		}
		own = files
	}
	shared, err := st.ListContextFiles(tenant, store.ContextScopeUser, key)
	if err != nil {
		return nil, err
	}
	if err := store.ContextFilesOverCap(shared); err != nil {
		return nil, err
	}
	have := make(map[string]struct{}, len(own))
	for _, f := range own {
		if f != nil {
			have[strings.TrimSpace(f.Name)] = struct{}{}
		}
	}
	fallback := make([]*store.ContextFile, 0, len(shared))
	for _, f := range shared {
		if f == nil {
			continue
		}
		if _, ok := have[strings.TrimSpace(f.Name)]; ok {
			continue
		}
		fallback = append(fallback, f)
	}
	merged := make([]*store.ContextFile, 0, len(own)+len(fallback))
	merged = append(merged, own...)
	merged = append(merged, fallback...)
	if err := store.ContextFilesOverCap(merged); err != nil {
		return nil, err
	}
	out, err := blocksFor(own, tenant, store.ContextScopeAgentUser, agentID, key)
	if err != nil {
		return nil, err
	}
	rest, err := blocksFor(fallback, tenant, store.ContextScopeUser, "", key)
	if err != nil {
		return nil, err
	}
	return append(out, rest...), nil
}

// blocksFor checks each row against the requested tenant, scope, agent, and
// user, then renders it. An empty agent or user is not checked for scopes
// that do not carry it.
func blocksFor(files []*store.ContextFile, tenant, scope, agentID, userKey string) ([]string, error) {
	out := make([]string, 0, len(files))
	for _, f := range files {
		if f == nil {
			continue
		}
		if !store.SameTenant(f.TenantID, tenant) || f.Scope != scope {
			return nil, errors.New("tenant mismatch")
		}
		switch scope {
		case store.ContextScopeAgent:
			if f.AgentID != agentID {
				return nil, errors.New("owner mismatch")
			}
		case store.ContextScopeUser:
			if f.UserKey != userKey {
				return nil, errors.New("owner mismatch")
			}
		case store.ContextScopeAgentUser:
			if f.AgentID != agentID || f.UserKey != userKey {
				return nil, errors.New("owner mismatch")
			}
		}
		name := strings.TrimSpace(f.Name)
		body := strings.TrimSpace(f.Body)
		if name == "" || body == "" {
			continue
		}
		out = append(out, name+":\n"+body)
	}
	return out, nil
}

// Class is the log token for an error. It is never the error text.
func Class(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, store.ErrNotFound):
		return "not_found"
	case errors.Is(err, store.ErrContextInvalid):
		return "invalid"
	case errors.Is(err, store.ErrContextTooLarge), errors.Is(err, store.ErrContextOwnerCap):
		return "too_large"
	default:
		return "store"
	}
}

// LogFailure writes operation and error class only. The error string is not logged.
func LogFailure(op string, err error) {
	if err == nil {
		return
	}
	op = strings.TrimSpace(op)
	if op == "" {
		op = "unknown"
	}
	logPrintf("goso context-files: op=%s class=%s", op, Class(err))
}
