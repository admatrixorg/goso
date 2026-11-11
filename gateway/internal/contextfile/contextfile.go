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

// Prompt loads agent files, then user files, as labeled blocks (name + body).
// A store error is returned. A row whose tenant or owner does not match the
// request is a store error, not a skipped row.
func Prompt(st store.StoreIface, tenant, agentID, userKey string) (string, error) {
	if st == nil {
		return "", errors.New("store required")
	}
	var parts []string
	if id := strings.TrimSpace(agentID); id != "" {
		files, err := st.ListContextFiles(tenant, store.ContextScopeAgent, id)
		if err != nil {
			return "", err
		}
		if err := store.ContextFilesOverCap(files); err != nil {
			return "", err
		}
		blocks, err := blocksFor(files, tenant, store.ContextScopeAgent, id)
		if err != nil {
			return "", err
		}
		parts = append(parts, blocks...)
	}
	if key := strings.TrimSpace(userKey); key != "" {
		if !store.UserKeyOK(key) {
			return "", store.ErrContextInvalid
		}
		files, err := st.ListContextFiles(tenant, store.ContextScopeUser, key)
		if err != nil {
			return "", err
		}
		if err := store.ContextFilesOverCap(files); err != nil {
			return "", err
		}
		blocks, err := blocksFor(files, tenant, store.ContextScopeUser, key)
		if err != nil {
			return "", err
		}
		parts = append(parts, blocks...)
	}
	return strings.Join(parts, "\n\n"), nil
}

func blocksFor(files []*store.ContextFile, tenant, scope, owner string) ([]string, error) {
	out := make([]string, 0, len(files))
	for _, f := range files {
		if f == nil {
			continue
		}
		if !store.SameTenant(f.TenantID, tenant) || f.Scope != scope {
			return nil, errors.New("tenant mismatch")
		}
		gotOwner := f.AgentID
		if scope == store.ContextScopeUser {
			gotOwner = f.UserKey
		}
		if gotOwner != owner {
			return nil, errors.New("owner mismatch")
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
