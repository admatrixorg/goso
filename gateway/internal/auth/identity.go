package auth

import (
	"context"
	"strings"
)

// Identity is the signed-in user a session token resolves to. TenantID is
// the tenant that user chose at login; Owner allows switching to another
// tenant through the tenant header, the way GoClaw's master scope does.
type Identity struct {
	UserID   string
	Email    string
	TenantID string
	Role     string
	Owner    bool
}

type identityKey struct{}

// WithIdentity stores the signed-in identity on ctx.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// IdentityFrom reads the signed-in identity, if any.
func IdentityFrom(ctx context.Context) (Identity, bool) {
	if ctx == nil {
		return Identity{}, false
	}
	id, ok := ctx.Value(identityKey{}).(Identity)
	if !ok {
		return Identity{}, false
	}
	id.TenantID = strings.TrimSpace(id.TenantID)
	return id, true
}

// IsOwner reports a role that may read every tenant.
func (i Identity) IsOwner() bool {
	return i.Owner || strings.EqualFold(strings.TrimSpace(i.Role), "owner")
}

// SessionGate resolves a session token into an Identity.
type SessionGate interface {
	Accept(token string) (Identity, bool)
}
