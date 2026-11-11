// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vaultimport

import (
	"net/url"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/vault"
)

// Path reasons are stable report codes. They describe the path, never a body.
const (
	reasonAbsolute    = "absolute"
	reasonTraversal   = "traversal"
	reasonEncoded     = "encoded-path"
	reasonSymlink     = "symlink"
	reasonIllegal     = "illegal-path"
	reasonOtherTenant = "other-tenant"
	reasonOutside     = "outside-root"
	reasonEmpty       = "empty-path"
	reasonLogical     = "logical-path"
)

// decision is the confinement result for one export path.
type decision struct {
	Logical string
	Reason  string
}

// cleanRoot checks the operator-supplied tenant root. An unsafe root is a
// configuration error, not a per-document rejection.
func cleanRoot(root string) (string, error) {
	root = strings.TrimSpace(strings.ReplaceAll(root, "\\", "/"))
	root = strings.Trim(root, "/")
	if root == "" {
		return "", errConfig("tenant root is required")
	}
	if reason := unsafePath(root); reason != "" {
		return "", errConfig("tenant root is not a relative folder (" + reason + ")")
	}
	cleaned, ok := lexical(root)
	if !ok {
		return "", errConfig("tenant root is not a relative folder")
	}
	return cleaned, nil
}

func tenantSlug(root string) string {
	parts := strings.Split(root, "/")
	for i := 0; i < len(parts)-1; i++ {
		if strings.EqualFold(parts[i], "tenants") {
			return parts[i+1]
		}
	}
	return ""
}

// classifyPath keeps a path only when it sits inside root and does not enter
// another tenants/<slug> folder. The logged path is the original string; the
// returned logical path is relative to root.
func classifyPath(raw, root string) decision {
	original := strings.TrimSpace(raw)
	if original == "" {
		return decision{Reason: reasonEmpty}
	}
	slashed := strings.ReplaceAll(original, "\\", "/")
	if reason := unsafePath(slashed); reason != "" {
		return decision{Reason: reason}
	}
	cleaned, ok := lexical(slashed)
	if !ok {
		return decision{Reason: reasonTraversal}
	}
	if foreignTenant(slashed, tenantSlug(root)) || foreignTenant(cleaned, tenantSlug(root)) {
		return decision{Reason: reasonOtherTenant}
	}
	if cleaned != root && !strings.HasPrefix(cleaned, root+"/") {
		return decision{Reason: reasonOutside}
	}
	if cleaned == root {
		return decision{Reason: reasonOutside}
	}
	logical := strings.TrimPrefix(cleaned, root+"/")
	logical = withTextExt(logical)
	checked, err := vault.LogicalPath(logical)
	if err != nil || checked == "" {
		return decision{Reason: reasonLogical}
	}
	return decision{Logical: checked}
}

// unsafePath reports a path that must not be cleaned before rejection.
// Cleaning would hide ".." and would turn a symlink-like token into a normal path.
func unsafePath(p string) string {
	if p == "" {
		return reasonEmpty
	}
	if strings.ContainsRune(p, 0) || strings.ContainsAny(p, "\r\n") {
		return reasonIllegal
	}
	if strings.Contains(p, "->") {
		return reasonSymlink
	}
	if strings.Contains(p, "%") {
		if decoded, err := url.PathUnescape(p); err != nil || decoded != p {
			return reasonEncoded
		}
	}
	lower := strings.ToLower(p)
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "~") || strings.HasPrefix(lower, "file:") {
		return reasonAbsolute
	}
	if len(p) >= 2 && p[1] == ':' {
		return reasonAbsolute
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return reasonTraversal
		}
	}
	return ""
}

// lexical drops empty and "." segments. A ".." segment is not collapsed; the
// caller has already rejected those.
func lexical(p string) (string, bool) {
	var parts []string
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." {
			return "", false
		}
		parts = append(parts, seg)
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "/"), true
}

// foreignTenant is true when any tenants/<slug> pair names a folder other than
// the one this run is allowed to read. A root that is not under tenants/
// allows no tenants/<slug> segment at all.
func foreignTenant(p, allowedSlug string) bool {
	parts := strings.Split(p, "/")
	for i := 0; i < len(parts)-1; i++ {
		if !strings.EqualFold(parts[i], "tenants") {
			continue
		}
		slug := parts[i+1]
		if slug == "" || slug == "." || slug == ".." {
			return true
		}
		if allowedSlug == "" || !strings.EqualFold(slug, allowedSlug) {
			return true
		}
	}
	return false
}

// withTextExt keeps .md and .txt so a later scoped sync, which only walks
// those extensions, still sees the file. Other paths gain a .md suffix.
func withTextExt(logical string) string {
	ext := strings.ToLower(extOf(logical))
	if ext == ".md" || ext == ".txt" {
		return logical
	}
	return logical + ".md"
}

func extOf(p string) string {
	i := strings.LastIndex(p, "/")
	base := p
	if i >= 0 {
		base = p[i+1:]
	}
	j := strings.LastIndex(base, ".")
	if j <= 0 {
		return ""
	}
	return base[j:]
}
