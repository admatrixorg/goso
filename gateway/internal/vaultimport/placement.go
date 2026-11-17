// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vaultimport

import (
	"sort"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

const (
	reasonPathCollision = "path-collision"
	reasonPathTaken     = "path-taken"
)

// GoClaw keys a vault row on (tenant, agent, team, scope, path), so one path
// can hold a shared row, a team row and personal rows of several agents.
// goso keys a row on (tenant, path) and keeps its file at t/<tenant>/<path>.
// placeAll keeps every row: the widest audience keeps the plain path, and
// each narrower row moves to a qualified sibling path
// (notes/a.md -> notes/a~personal-<agent key>.md). The qualifier is built
// from the GoClaw scope and the goso owner, so it is the same on every run
// and two GoClaw rows never share it.
//
// Rows that still land on the same path (two export paths that clean to the
// same logical path, or a qualified path that is already a real path) are
// path-collision. The first one by source id keeps the path.
func placeAll(items []*keptDoc, owner func(docAccess) string) []scopeLoser {
	groups := map[string][]*keptDoc{}
	var order []string
	for _, item := range items {
		if _, ok := groups[item.baseLogical]; !ok {
			order = append(order, item.baseLogical)
		}
		groups[item.baseLogical] = append(groups[item.baseLogical], item)
	}
	sort.Strings(order)
	taken := map[string]*keptDoc{}
	var losers []scopeLoser
	claim := func(item *keptDoc, logical string) {
		if prev, ok := taken[logical]; ok && prev != item {
			losers = append(losers, scopeLoser{item: item, reason: reasonPathCollision})
			item.drop = true
			return
		}
		taken[logical] = item
		item.logical = logical
	}
	// Plain paths first, so a qualified path never displaces a real one.
	var qualified []*keptDoc
	for _, base := range order {
		group := append([]*keptDoc(nil), groups[base]...)
		sort.SliceStable(group, func(i, j int) bool {
			ri, rj := breadth(group[i].access), breadth(group[j].access)
			if ri != rj {
				return ri < rj
			}
			return strings.TrimSpace(group[i].src.ID) < strings.TrimSpace(group[j].src.ID)
		})
		claim(group[0], base)
		qualified = append(qualified, group[1:]...)
	}
	for _, item := range qualified {
		q := qualifier(item, owner)
		logical, ok := qualifyLogical(item.baseLogical, q, item.binary != nil)
		if !ok {
			losers = append(losers, scopeLoser{item: item, reason: reasonPathCollision})
			item.drop = true
			continue
		}
		claim(item, logical)
	}
	return losers
}

// breadth orders audiences from widest (0) to narrowest. The widest row of a
// path keeps the plain path.
func breadth(a docAccess) int {
	switch a.scope {
	case store.VaultScopeTenant:
		return 0
	case store.VaultScopeTeam:
		return 1
	case store.VaultScopeAgent:
		return 2
	default:
		return 3
	}
}

// qualifier is "<goclaw scope>-<owner>": the source scope keeps a personal
// row and a legacy shared row of the same agent apart, and the owner keeps
// two agents (or two teams) apart.
func qualifier(item *keptDoc, owner func(docAccess) string) string {
	scope := pathToken(item.src.Scope)
	if scope == "" {
		scope = "shared"
	}
	if o := pathToken(owner(item.access)); o != "" {
		return scope + "-" + o
	}
	return scope
}

// pathToken keeps [a-z0-9_-] and maps every other rune to "-". It never
// returns "." or "..", so the result is safe inside a path segment.
func pathToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// qualifyLogical inserts "~<q>" before the extension of the last segment.
// For an attachment the extension is the one before the trailing ".md", so
// the stored file keeps its own extension (a.pdf.md -> a~q.pdf.md).
func qualifyLogical(logical, q string, binary bool) (string, bool) {
	if q == "" {
		return "", false
	}
	target := logical
	suffix := ""
	if binary {
		target = strings.TrimSuffix(logical, ".md")
		suffix = ".md"
	}
	dir, base := "", target
	if i := strings.LastIndex(target, "/"); i >= 0 {
		dir, base = target[:i+1], target[i+1:]
	}
	ext := extOf(base)
	name := strings.TrimSuffix(base, ext)
	out := dir + name + "~" + q + ext + suffix
	checked, err := vault.LogicalPath(out)
	if err != nil || checked != out {
		return "", false
	}
	return out, true
}

// covers reports that every reader of the stored row b can also read access
// a. Only then may the file be rewritten before the row: while the new text
// sits under the old row, b's readers are all allowed to see it.
func covers(a docAccess, b *store.VaultDoc) bool {
	if a.scope == store.VaultScopeTenant {
		return true
	}
	if b == nil || strings.TrimSpace(b.CustomerKey) != "" {
		return false
	}
	scope := b.Scope
	if scope == "" {
		scope = store.VaultScopeTenant
	}
	if a.same(b) {
		return true
	}
	return a.scope == store.VaultScopeAgent && scope == store.VaultScopePersonal && a.agentID == b.AgentID
}
