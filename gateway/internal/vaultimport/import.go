// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package vaultimport loads one tenant from an offline GoClaw vault export
// into goso's existing vault_docs and vault_links rows.
//
// The export is JSON with the GoClaw column names vault_documents,
// vault_links, and vault_versions. Embedding and tsv are not fields of the
// reader, so the decoder drops them. Only the highest vault_versions row for
// each document is kept, and that text is written into the existing body
// column. This package does not create a vault_versions table.
//
// Link type and context are stored in the existing vault_links.raw text
// ("[[<goso id>|<type>: <context>]]"). Resolution still uses the goso id.
//
// Each imported body ends with a one-line provenance marker
// (<!-- goso-import src=<id> sum=<sha256> -->). A version row keeps sum as
// sha256(title + "\n" + body prefix). A document with no version row takes
// its body from the file under SrcDir, and sum is sha256 of that file's
// bytes. A later run refreshes a version-sourced row when the marker names
// the same document and the stored title and prefix still match that sum.
// A file-sourced row is refreshed only when the marker sum equals sha256 of
// the current file bytes, the title is unchanged, and the stored text still
// begins with that file. A different file, or a hand edit of the file text,
// is path-taken. An empty file body is not imported.
//
// KeepPrefix stores the path with the root still attached. The default
// strips that prefix. A workspace root (".", "/app/workspace") accepts
// every safe relative path in one run. Any tenants/<other> path is
// foreign-tenant. Scope personal is held back unless IncludePersonal is
// set: goso has no per-person access control, so that body would be
// readable by the whole tenant. When a path has both a personal and a
// team document, the team document is stored and the personal one is
// held once as scope-personal-held. It is not also counted as
// scope-conflict. With IncludePersonal, the personal document wins that
// conflict and the team one is scope-conflict.
// There is still no scope column.
//
// A run names one source tenant id and one root folder. A document is imported
// only when both match. Paths that leave the root, walk with "..", are
// absolute, look like a symlink, or enter tenants/<other> are rejected and
// logged as a path. Logs and the report contain paths and reason codes only.
package vaultimport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// errSymlink is a destination that is a symlink or is reached through one.
// The importer does not follow it and does not replace it.
var errSymlink = errors.New("symlink")

// Options selects the single tenant for this run.
type Options struct {
	// SourceTenant is the GoClaw vault_documents.tenant_id. Match is case-insensitive after trim.
	SourceTenant string
	// Root is that tenant's folder. A path must resolve inside it.
	Root string
	// DestTenant is the goso tenant_id written on vault_docs. It is not defaulted.
	DestTenant string
	// VaultRoot, when set, also publishes the body under t/<tenant>/ so the
	// existing vault service can confirm the file. Empty writes the registry only.
	VaultRoot string
	// KeepPrefix stores the cleaned source path without removing Root.
	// The default strips Root. Use KeepPrefix with root "." or /app/workspace
	// to keep original folder prefixes in one run.
	KeepPrefix bool
	// SrcDir, when set, is the source workspace. A document whose file is
	// missing there is path-missing and is not inserted. When the document
	// has a version row, that text is still the body. When it has no version
	// row, the file bytes are the body and the provenance sum is their sha256.
	// A kept document with no version and an empty SrcDir fails the run
	// before any write. Files over maxFileBodyBytes (1 MiB) are too-large.
	// Non-UTF-8 files and files with a NUL byte are binary-skipped.
	// A source or destination path under raw/ is raw-skipped and is not read or written.
	SrcDir string
	// IncludePersonal imports scope=personal documents. Default is off.
	// Turn it on only after the tenant owner accepts that those notes are
	// readable by the whole tenant. This flag is not per-person access control.
	IncludePersonal bool
	// DryRun reports counts and rejections and does not write.
	DryRun bool
	// Log receives rejection lines. Nil discards them. Arguments are paths and
	// reason codes, never document bodies.
	Log Logger
}

// Logger is the rejection log. Printf must not be given a document body.
type Logger interface {
	Printf(format string, args ...any)
}

// Report is the idempotent run summary.
//
// Inserted, Updated, and Skipped count vault_docs rows. Rejected counts
// document and link rejections. Docs and Links are the same rows broken
// down per source tenant. For each section, Input equals
// Inserted + Updated + Skipped + Rejected, and Reasons sums to
// Skipped + Rejected. A personal document that shares a path with a team
// document is one scope-personal-held row, not also scope-conflict.
type Report struct {
	Inserted   int             `json:"inserted"`
	Updated    int             `json:"updated"`
	Skipped    int             `json:"skipped"`
	Rejected   int             `json:"rejected"`
	DryRun     bool            `json:"dry_run"`
	Rejections []Rejection     `json:"rejections,omitempty"`
	Docs       []TenantSection `json:"docs"`
	Links      []TenantSection `json:"links"`
}

// Rejection is one dropped path. Path and Target are export paths, not bodies.
type Rejection struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason"`
}

// Export is an offline dump. Embedding and tsv are not represented.
type Export struct {
	Documents []Document `json:"vault_documents"`
	Links     []Link     `json:"vault_links"`
	Versions  []Version  `json:"vault_versions"`
}

// Document is one vault_documents row. Unknown JSON fields, including
// embedding and tsv, are ignored by the decoder.
type Document struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	Path        string          `json:"path"`
	Title       string          `json:"title"`
	DocType     string          `json:"doc_type"`
	Summary     string          `json:"summary"`
	Metadata    json.RawMessage `json:"metadata"`
	Scope       string          `json:"scope"`
	CustomScope string          `json:"custom_scope"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// Link is one vault_links row. Metadata is not copied; type and context map
// into the existing raw column.
type Link struct {
	ID       string `json:"id"`
	FromDoc  string `json:"from_doc_id"`
	ToDoc    string `json:"to_doc_id"`
	LinkType string `json:"link_type"`
	Context  string `json:"context"`
}

// Version is one vault_versions row. Only the latest version per document is used.
type Version struct {
	ID        string `json:"id"`
	DocID     string `json:"doc_id"`
	Version   int    `json:"version"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// ConfigError is a bad option. The text names the option, not a document.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func errConfig(msg string) error { return &ConfigError{Msg: msg} }

// ReadExport decodes a GoClaw-shaped JSON dump from r.
func ReadExport(r io.Reader) (*Export, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var ex Export
	if err := dec.Decode(&ex); err != nil {
		return nil, fmt.Errorf("read export: %w", err)
	}
	return &ex, nil
}

type keptDoc struct {
	src      Document
	logical  string
	title    string
	body     string
	sum      string
	mtime    time.Time
	gosoID   string
	existing *store.VaultDoc
	docSame  bool
	fileSame bool
	drop     bool
	// fromFile is set when the document has no vault_versions row.
	fromFile bool
	// fileSum is sha256 of the raw file bytes. fileText is the normalized text.
	fileSum  string
	fileText string
}

type wantLink struct {
	to       *keptDoc
	linkType string
	context  string
}

// Import reads ex into st. A second call with the same ex and options does
// not write when the first call finished. st is the existing goso store.
func Import(st store.StoreIface, ex *Export, opt Options) (*Report, error) {
	if st == nil || ex == nil {
		return nil, errConfig("store and export are required")
	}
	root, err := cleanRoot(opt.Root)
	if err != nil {
		return nil, err
	}
	source := strings.TrimSpace(opt.SourceTenant)
	if source == "" {
		return nil, errConfig("source tenant id is required")
	}
	dest := strings.TrimSpace(opt.DestTenant)
	if !store.TenantOK(dest) {
		return nil, errConfig("dest tenant id is empty or not a goso tenant id")
	}
	if strings.TrimSpace(opt.SrcDir) != "" {
		if err := checkSrcDir(opt.SrcDir); err != nil {
			return nil, err
		}
	}
	logf := opt.Log
	if logf == nil {
		logf = discardLog{}
	}
	ac := &tally{log: logf}

	latest := latestVersions(ex.Versions)
	pathByID := map[string]string{}
	tenantByID := map[string]string{}
	for _, doc := range ex.Documents {
		id := strings.TrimSpace(doc.ID)
		if id == "" {
			continue
		}
		if _, ok := pathByID[id]; !ok {
			pathByID[id] = strings.TrimSpace(doc.Path)
		}
		if _, ok := tenantByID[id]; !ok {
			tenantByID[id] = strings.TrimSpace(doc.TenantID)
		}
	}

	docs := append([]Document(nil), ex.Documents...)
	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Path != docs[j].Path {
			return docs[i].Path < docs[j].Path
		}
		return docs[i].ID < docs[j].ID
	})

	seenID := map[string]struct{}{}
	var candidates []*keptDoc
	for _, doc := range docs {
		path := strings.TrimSpace(doc.Path)
		tenant := strings.TrimSpace(doc.TenantID)
		if !tenantMatch(doc.TenantID, source) {
			d := classifyPath(doc.Path, root, opt.KeepPrefix)
			if d.Reason != "" {
				ac.reject("doc", tenant, path, "", d.Reason)
				continue
			}
			ac.add("doc", tenant, "skipped", reasonTenantMismatch, path, "")
			continue
		}
		d := classifyPath(doc.Path, root, opt.KeepPrefix)
		if d.Reason != "" {
			ac.reject("doc", tenant, path, "", d.Reason)
			continue
		}
		id := strings.TrimSpace(doc.ID)
		if !sourceIDOK(id) {
			ac.reject("doc", tenant, path, "", "source-id")
			continue
		}
		if _, ok := seenID[id]; ok {
			ac.reject("doc", tenant, path, "", "duplicate-id")
			continue
		}
		seenID[id] = struct{}{}
		content := ""
		fromFile := true
		var versionAt time.Time
		if v, ok := latest[id]; ok {
			content = v.content
			versionAt = v.at
			fromFile = false
		}
		meta := scrubMeta(doc.Metadata, map[string]string{
			"doc_type":     doc.DocType,
			"scope":        doc.Scope,
			"custom_scope": doc.CustomScope,
		})
		title := strings.TrimSpace(doc.Title)
		if title == "" {
			title = titleFromLogical(d.Logical)
		}
		if title == "" {
			ac.reject("doc", tenant, path, "", "title-required")
			continue
		}
		body := ""
		if !fromFile {
			body = stampImport(title, composeBody(content, doc.Summary, meta), id)
		}
		mtime := versionAt
		if t, ok := parseTS(doc.UpdatedAt); ok && t.After(mtime) {
			mtime = t
		}
		if mtime.IsZero() {
			if t, ok := parseTS(doc.CreatedAt); ok {
				mtime = t
			}
		}
		if mtime.IsZero() {
			mtime = time.Unix(0, 0).UTC()
		}
		candidates = append(candidates, &keptDoc{
			src:      doc,
			logical:  d.Logical,
			title:    title,
			body:     body,
			sum:      hashBody(body),
			mtime:    mtime.UTC(),
			fromFile: fromFile,
		})
	}
	kept, losers := preferNarrowerScope(candidates, opt.IncludePersonal)
	for _, loser := range losers {
		ac.reject("doc", loser.item.src.TenantID, strings.TrimSpace(loser.item.src.Path), "", loser.reason)
	}
	bySource := map[string]*keptDoc{}
	for _, item := range kept {
		bySource[strings.TrimSpace(item.src.ID)] = item
	}
	if strings.TrimSpace(opt.SrcDir) == "" {
		for _, item := range kept {
			if item.fromFile {
				return ac.abort(opt.DryRun, errConfig("source dir is required when a document has no version"))
			}
		}
	}
	if strings.TrimSpace(opt.SrcDir) != "" {
		var present []*keptDoc
		for _, item := range kept {
			info, err := inspectSource(opt.SrcDir, item.src.Path)
			if err != nil {
				return ac.abort(opt.DryRun, err)
			}
			path := strings.TrimSpace(item.src.Path)
			tenant := item.src.TenantID
			id := strings.TrimSpace(item.src.ID)
			switch info.state {
			case sourceMissing:
				ac.reject("doc", tenant, path, "", "path-missing")
				delete(bySource, id)
				continue
			case sourceSymlink:
				ac.reject("doc", tenant, path, "", reasonSymlink)
				delete(bySource, id)
				continue
			case sourcePresent:
			default:
				return ac.abort(opt.DryRun, fmt.Errorf("vaultimport: source file state"))
			}
			if info.abs == "" {
				ac.reject("doc", tenant, path, "", "path-missing")
				delete(bySource, id)
				continue
			}
			if item.fromFile {
				raw, reason, err := loadFileBody(info.abs, info.size)
				if err != nil {
					return ac.abort(opt.DryRun, err)
				}
				if reason != "" {
					ac.reject("doc", tenant, path, "", reason)
					delete(bySource, id)
					continue
				}
				item.applyFile(raw)
				if item.body == "" {
					return ac.abort(opt.DryRun, errConfig("refusing to import an empty body"))
				}
			}
			present = append(present, item)
		}
		kept = present
	}

	for _, item := range kept {
		path := strings.TrimSpace(item.src.Path)
		tenant := item.src.TenantID
		id := strings.TrimSpace(item.src.ID)
		if item.body == "" {
			return ac.abort(opt.DryRun, errConfig("refusing to import an empty body"))
		}
		existing, err := st.FindVaultDocByPath(dest, item.logical)
		if errors.Is(err, store.ErrNotFound) {
			existing = nil
		} else if err != nil {
			return ac.abort(opt.DryRun, err)
		}
		if existing != nil && !existing.OwnerExplicit {
			ac.reject("doc", tenant, path, "", "owner-unclear")
			item.drop = true
			delete(bySource, id)
			continue
		}
		if existing != nil && !importOwned(existing.Title, existing.Body, item.title, id, item.fileSum, item.fileText, item.fromFile) {
			ac.reject("doc", tenant, path, "", "path-taken")
			item.drop = true
			delete(bySource, id)
			continue
		}
		item.existing = existing
		if existing != nil {
			item.gosoID = existing.ID
			item.docSame = existing.Title == item.title &&
				existing.Path == item.logical &&
				existing.Body == item.body &&
				existing.SHA256 == item.sum &&
				existing.Mtime.Equal(item.mtime) &&
				existing.OwnerExplicit
		}
		item.fileSame = true
		if strings.TrimSpace(opt.VaultRoot) != "" {
			same, err := fileMatches(opt.VaultRoot, dest, item.logical, []byte(item.body))
			if errors.Is(err, errSymlink) {
				ac.reject("doc", tenant, path, "", reasonSymlink)
				item.drop = true
				delete(bySource, id)
				continue
			}
			if err != nil {
				return ac.abort(opt.DryRun, err)
			}
			item.fileSame = same
		}
	}

	plans, wants, err := planLinks(st, ex.Links, bySource, pathByID, tenantByID)
	if err != nil {
		return ac.abort(opt.DryRun, err)
	}

	var fresh []*keptDoc
	for _, item := range kept {
		if item.drop || item.existing != nil {
			continue
		}
		fresh = append(fresh, item)
		path := strings.TrimSpace(item.src.Path)
		if opt.DryRun {
			ac.add("doc", item.src.TenantID, "inserted", "", path, "")
			continue
		}
		if strings.TrimSpace(opt.VaultRoot) != "" {
			if _, err := publishFile(opt.VaultRoot, dest, item.logical, []byte(item.body)); err != nil {
				if errors.Is(err, errSymlink) {
					item.drop = true
					delete(bySource, strings.TrimSpace(item.src.ID))
					ac.reject("doc", item.src.TenantID, path, "", reasonSymlink)
					retargetLinks(plans, ex.Links, strings.TrimSpace(item.src.ID))
					continue
				}
				return ac.abort(opt.DryRun, err)
			}
		}
		saved, err := st.PutVaultDoc(store.VaultDoc{
			Title:         item.title,
			Path:          item.logical,
			SHA256:        item.sum,
			Mtime:         item.mtime,
			Body:          item.body,
			TenantID:      dest,
			OwnerExplicit: true,
		})
		if err != nil {
			return ac.abort(opt.DryRun, err)
		}
		item.gosoID = saved.ID
		ac.add("doc", item.src.TenantID, "inserted", "", path, "")
	}

	replaced := false
	for _, item := range kept {
		if item.drop || item.existing == nil {
			continue
		}
		path := strings.TrimSpace(item.src.Path)
		inners, pending := innersFor(item, wants)
		linksSame := false
		if !pending {
			linksSame, err = sameLinks(st, item.gosoID, inners)
			if err != nil {
				return ac.abort(opt.DryRun, err)
			}
		}
		if item.docSame && item.fileSame && linksSame {
			ac.add("doc", item.src.TenantID, "skipped", reasonUnchanged, path, "")
			continue
		}
		if opt.DryRun {
			ac.add("doc", item.src.TenantID, "updated", "", path, "")
			continue
		}
		if !item.fileSame {
			if _, err := publishFile(opt.VaultRoot, dest, item.logical, []byte(item.body)); err != nil {
				if errors.Is(err, errSymlink) {
					item.drop = true
					delete(bySource, strings.TrimSpace(item.src.ID))
					ac.reject("doc", item.src.TenantID, path, "", reasonSymlink)
					retargetLinks(plans, ex.Links, strings.TrimSpace(item.src.ID))
					continue
				}
				return ac.abort(opt.DryRun, err)
			}
		}
		if !item.docSame {
			if _, err := st.PutVaultDoc(store.VaultDoc{
				ID:            item.existing.ID,
				Title:         item.title,
				Path:          item.logical,
				SHA256:        item.sum,
				Mtime:         item.mtime,
				Body:          item.body,
				TenantID:      dest,
				OwnerExplicit: true,
			}); err != nil {
				return ac.abort(opt.DryRun, err)
			}
		}
		if !linksSame {
			if err := st.ReplaceVaultLinks(item.gosoID, inners); err != nil {
				return ac.abort(opt.DryRun, err)
			}
			replaced = true
		}
		ac.add("doc", item.src.TenantID, "updated", "", path, "")
	}

	if !opt.DryRun {
		for _, item := range fresh {
			if item.drop || item.gosoID == "" {
				continue
			}
			inners, _ := innersFor(item, wants)
			if len(inners) == 0 {
				continue
			}
			if err := st.ReplaceVaultLinks(item.gosoID, inners); err != nil {
				return ac.abort(opt.DryRun, err)
			}
			replaced = true
		}
		if replaced {
			if err := st.ReResolveVaultLinks(); err != nil {
				return ac.abort(opt.DryRun, err)
			}
		}
	}
	for _, p := range plans {
		ac.add("link", p.tenant, p.action, p.reason, p.path, p.target)
	}
	return ac.finish(opt.DryRun, len(ex.Documents), len(ex.Links))
}

func (item *keptDoc) applyFile(raw []byte) {
	item.fromFile = true
	item.fileSum = hashBody(string(raw))
	item.fileText = normalizeNewlines(string(raw))
	meta := scrubMeta(item.src.Metadata, map[string]string{
		"doc_type":     item.src.DocType,
		"scope":        item.src.Scope,
		"custom_scope": item.src.CustomScope,
	})
	plain := composeBody(item.fileText, item.src.Summary, meta)
	item.body = stampImportSum(item.title, plain, strings.TrimSpace(item.src.ID), item.fileSum)
	item.sum = hashBody(item.body)
}

type linkPlan struct {
	tenant string
	action string
	reason string
	path   string
	target string
}

func planLinks(st store.StoreIface, links []Link, bySource map[string]*keptDoc, pathByID, tenantByID map[string]string) ([]linkPlan, map[string][]wantLink, error) {
	plans := make([]linkPlan, len(links))
	wants := map[string][]wantLink{}
	cache := map[string][]store.VaultLink{}
	for i, l := range links {
		fromID := strings.TrimSpace(l.FromDoc)
		toID := strings.TrimSpace(l.ToDoc)
		from := bySource[fromID]
		to := bySource[toID]
		plans[i] = linkPlan{tenant: tenantByID[fromID], path: pathByID[fromID], target: pathByID[toID]}
		if from == nil || from.drop {
			reason := "source-rejected"
			if _, known := pathByID[fromID]; !known {
				reason = "source-missing"
			}
			plans[i].action = "rejected"
			plans[i].reason = reason
			continue
		}
		if to == nil || to.drop {
			reason := "outside-tenant"
			if _, known := pathByID[toID]; !known {
				reason = "target-missing"
				plans[i].target = ""
			}
			plans[i].action = "rejected"
			plans[i].reason = reason
			continue
		}
		action, err := linkAction(st, from, to, l, cache)
		if err != nil {
			return nil, nil, err
		}
		plans[i].action = action
		if action == "skipped" {
			plans[i].reason = reasonUnchanged
		}
		wants[fromID] = append(wants[fromID], wantLink{to: to, linkType: l.LinkType, context: l.Context})
	}
	return plans, wants, nil
}

func linkAction(st store.StoreIface, from, to *keptDoc, l Link, cache map[string][]store.VaultLink) (string, error) {
	if from.existing == nil || from.gosoID == "" {
		return "inserted", nil
	}
	toID := to.gosoID
	if toID == "" && to.existing != nil {
		toID = to.existing.ID
	}
	if toID == "" {
		return "inserted", nil
	}
	ob, ok := cache[from.gosoID]
	if !ok {
		var err error
		ob, _, err = st.ListVaultDocLinks(from.gosoID)
		if err != nil {
			return "", err
		}
		cache[from.gosoID] = ob
	}
	raw := storedRaw(linkInner(toID, l.LinkType, l.Context))
	for _, got := range ob {
		if got.Raw == raw {
			return "skipped", nil
		}
	}
	for _, got := range ob {
		if linkTargetID(got.Raw) == toID {
			return "updated", nil
		}
	}
	return "inserted", nil
}

func retargetLinks(plans []linkPlan, links []Link, id string) {
	id = strings.TrimSpace(id)
	for i, l := range links {
		if plans[i].action == "rejected" {
			continue
		}
		if strings.TrimSpace(l.FromDoc) == id {
			plans[i].action = "rejected"
			plans[i].reason = "source-rejected"
			continue
		}
		if strings.TrimSpace(l.ToDoc) == id {
			plans[i].action = "rejected"
			plans[i].reason = "outside-tenant"
		}
	}
}

func linkTargetID(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "[[")
	s = strings.TrimSuffix(s, "]]")
	if i := strings.IndexByte(s, '|'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

type scopeLoser struct {
	item   *keptDoc
	reason string
}

// preferNarrowerScope keeps one document per logical path.
//
// Scope personal is omitted unless includePersonal is set. goso cannot
// restrict a vault row to one person, so the default is to hold those
// documents once, as scope-personal-held. A team document at the same
// path is stored. The held personal document is not also scope-conflict.
//
// With includePersonal, personal is the narrowest scope and wins the
// path. The other scopes are scope-conflict. Same-scope extras are
// path-collision. Neither branch overwrites silently.
func preferNarrowerScope(items []*keptDoc, includePersonal bool) ([]*keptDoc, []scopeLoser) {
	groups := map[string][]*keptDoc{}
	var order []string
	for _, item := range items {
		if _, ok := groups[item.logical]; !ok {
			order = append(order, item.logical)
		}
		groups[item.logical] = append(groups[item.logical], item)
	}
	var winners []*keptDoc
	var losers []scopeLoser
	for _, logical := range order {
		group := append([]*keptDoc(nil), groups[logical]...)
		sort.Slice(group, func(i, j int) bool {
			return strings.TrimSpace(group[i].src.ID) < strings.TrimSpace(group[j].src.ID)
		})
		if !includePersonal {
			var personal, rest []*keptDoc
			for _, item := range group {
				if isPersonalScope(item.src.Scope) {
					personal = append(personal, item)
				} else {
					rest = append(rest, item)
				}
			}
			for _, item := range personal {
				losers = append(losers, scopeLoser{item: item, reason: "scope-personal-held"})
			}
			group = rest
			if len(group) == 0 {
				continue
			}
		}
		best := group[0]
		for _, item := range group[1:] {
			if scopeRank(item.src.Scope) < scopeRank(best.src.Scope) {
				best = item
			}
		}
		winners = append(winners, best)
		bestRank := scopeRank(best.src.Scope)
		for _, item := range group {
			if item == best {
				continue
			}
			reason := "path-collision"
			if scopeRank(item.src.Scope) != bestRank {
				reason = "scope-conflict"
			}
			losers = append(losers, scopeLoser{item: item, reason: reason})
		}
	}
	return winners, losers
}

func isPersonalScope(scope string) bool {
	return strings.EqualFold(strings.TrimSpace(scope), "personal")
}

func scopeRank(scope string) int {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "personal":
		return 0
	case "team":
		return 1
	case "shared":
		return 2
	case "custom":
		return 3
	default:
		return 4
	}
}

type versionPick struct {
	n       int
	at      time.Time
	content string
}

func latestVersions(versions []Version) map[string]versionPick {
	out := map[string]versionPick{}
	for _, v := range versions {
		id := strings.TrimSpace(v.DocID)
		if id == "" {
			continue
		}
		at, _ := parseTS(v.CreatedAt)
		cur, ok := out[id]
		replace := !ok || v.Version > cur.n || (v.Version == cur.n && at.After(cur.at))
		if replace {
			out[id] = versionPick{n: v.Version, at: at, content: v.Content}
		}
	}
	return out
}

func tenantMatch(docTenant, source string) bool {
	return strings.EqualFold(strings.TrimSpace(docTenant), strings.TrimSpace(source))
}

func titleFromLogical(logical string) string {
	base := logical
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".md")
	base = strings.TrimSuffix(base, ".txt")
	return strings.TrimSpace(base)
}

// innersFor builds link inners. pending is true when a target does not yet
// have a goso id (a new document during dry-run), so the link set cannot match.
func innersFor(from *keptDoc, wants map[string][]wantLink) ([]string, bool) {
	var inners []string
	seen := map[string]struct{}{}
	pending := false
	for _, w := range wants[strings.TrimSpace(from.src.ID)] {
		if w.to == nil || w.to.drop {
			continue
		}
		if w.to.gosoID == "" {
			pending = true
			continue
		}
		inner := linkInner(w.to.gosoID, w.linkType, w.context)
		if _, ok := seen[inner]; ok {
			continue
		}
		seen[inner] = struct{}{}
		inners = append(inners, inner)
	}
	sort.Strings(inners)
	return inners, pending
}

func sameLinks(st store.StoreIface, fromID string, inners []string) (bool, error) {
	if fromID == "" {
		return len(inners) == 0, nil
	}
	ob, _, err := st.ListVaultDocLinks(fromID)
	if err != nil {
		return false, err
	}
	got := map[string]struct{}{}
	for _, l := range ob {
		if strings.TrimSpace(l.Raw) == "" {
			continue
		}
		got[l.Raw] = struct{}{}
	}
	want := map[string]struct{}{}
	for _, inner := range inners {
		want[storedRaw(inner)] = struct{}{}
	}
	if len(got) != len(want) {
		return false, nil
	}
	for raw := range want {
		if _, ok := got[raw]; !ok {
			return false, nil
		}
	}
	return true, nil
}

type discardLog struct{}

func (discardLog) Printf(string, ...any) {}
