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
// bytes. A later run refreshes a row when the marker names the same
// document and the row is unchanged since the importer wrote it: for a
// version row the stored title and prefix still match the sum; for a file
// row the stored prefix, before the importer's trailer, is still the file
// text that hashed to the sum, even if the source file has changed since.
// A hand edit is path-taken. An empty file body is not imported.
//
// A binary file under SrcDir (NUL byte or not UTF-8) whose path is not .md
// or .txt is an attachment, as in GoClaw: its bytes are copied unchanged to
// t/<tenant>/<path>, and the row is a note at <path>.md that names the file,
// its type, size and sha256. No text is extracted, because GoClaw does not.
// Attachments over MaxBinaryBytes are too-large.
//
// KeepPrefix stores the path with the root still attached. The default
// strips that prefix. A workspace root (".", "/app/workspace") accepts
// every safe relative path in one run. Any tenants/<other> path is
// foreign-tenant. Scope personal is imported unless HoldPersonal is set.
//
// GoClaw can hold several rows at one path (shared, team, and personal rows
// of several agents). goso holds one row per path, so the widest audience
// keeps the plain path and each narrower row is stored at a qualified
// sibling path (see placeAll). No row is dropped for sharing a path, and no
// row is widened to reach one.
//
// Access carries over through AgentMap and TeamMap (GoClaw id to goso id):
// a personal document becomes agent scope of its mapped agent (or personal
// scope with PersonalScope "personal"); any other document with an agent_id
// becomes agent scope of its mapped agent; a team document (or one with only
// a team_id) becomes team scope of its mapped team; the rest is tenant
// scope. An agent_id or team_id with no mapping is rejected as
// agent-unmapped or team-unmapped, never widened to tenant scope.
//
// A re-run is idempotent. A row whose GoClaw scope changed is re-scoped. When
// the new audience covers the old one the row is updated in place; otherwise
// the old file and row are removed first and the row is written again, so no
// text is ever readable by an audience it did not have in GoClaw. A row the
// importer wrote for a source document that now lives at another path is
// reused by the document that owns the path now (with AdoptOrphans, also a
// row whose source document is gone from the export).
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
	"github.com/mqglobal/goso/gateway/internal/vault"
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
	// A non-UTF-8 or NUL file at a .md/.txt path is binary-skipped; at any
	// other path it is an attachment (see MaxBinaryBytes).
	// A source or destination path under raw/ is raw-skipped and is not read or written.
	SrcDir string
	// MaxBinaryBytes caps one attachment. Zero uses the vault upload cap
	// (GOSO_VAULT_INGEST_MAX_BYTES, default 20 MiB).
	MaxBinaryBytes int64
	// HoldPersonal holds back scope=personal documents as
	// scope-personal-held. The default imports them.
	HoldPersonal bool
	// AdoptOrphans lets a document reuse an untouched row this importer wrote
	// for a source id that is not in the export (a GoClaw document deleted and
	// created again at the same path). Set it only when the export holds every
	// document of the source tenant: with a partial export the row may belong
	// to another root's document. Off, such a row is path-taken.
	AdoptOrphans bool
	// PersonalScope is the goso scope of a GoClaw personal document:
	// "agent" (default; that agent in every chat, as in GoClaw) or
	// "personal" (that agent, never in a channel thread).
	PersonalScope string
	// AgentMap maps GoClaw agent ids to goso agent ids of DestTenant.
	AgentMap map[string]string
	// TeamMap maps GoClaw team ids to goso team ids of DestTenant.
	TeamMap map[string]string
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
	AgentID     string          `json:"agent_id"`
	TeamID      string          `json:"team_id"`
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
	src Document
	// baseLogical is the cleaned export path; logical is where the row is
	// stored after placeAll (baseLogical or a qualified sibling).
	baseLogical string
	logical     string
	title       string
	body        string
	sum         string
	mtime       time.Time
	access      docAccess
	gosoID      string
	existing    *store.VaultDoc
	docSame     bool
	fileSame    bool
	drop        bool
	// replace is set when the stored row's readers are not all readers of
	// the new access: the old file and row go before the new row is written.
	replace bool
	// takeover names the source id of a row this document reuses.
	takeover string
	// stale is this document's own earlier row at a path it no longer uses.
	stale *store.VaultDoc
	// fromFile is set when the document has no vault_versions row.
	fromFile bool
	// fileSum is sha256 of the raw file bytes. fileText is the normalized text.
	fileSum  string
	fileText string
	// binary is set for an attachment.
	binary *binaryInfo
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
	if err := checkPersonalScope(opt.PersonalScope); err != nil {
		return nil, err
	}
	if opt.MaxBinaryBytes < 0 {
		return nil, errConfig("max binary bytes must not be negative")
	}
	maxBinary := opt.MaxBinaryBytes
	if maxBinary == 0 {
		maxBinary = vault.IngestMaxBytes()
	}
	srcDir := strings.TrimSpace(opt.SrcDir)
	if srcDir != "" {
		if err := checkSrcDir(opt.SrcDir); err != nil {
			return nil, err
		}
	}
	vaultRoot := strings.TrimSpace(opt.VaultRoot)
	logf := opt.Log
	if logf == nil {
		logf = discardLog{}
	}
	ac := &tally{log: logf}
	opt.AgentMap, opt.TeamMap = ownedMaps(st, dest, opt.AgentMap, opt.TeamMap)

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

	// whyNot is the reason a source document is not stored by this run. Link
	// reasons are built from it.
	whyNot := map[string]string{}
	rejectDoc := func(doc Document, reason string) {
		ac.reject("doc", strings.TrimSpace(doc.TenantID), strings.TrimSpace(doc.Path), "", reason)
		if id := strings.TrimSpace(doc.ID); id != "" {
			whyNot[id] = reason
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
		id := strings.TrimSpace(doc.ID)
		if !tenantMatch(doc.TenantID, source) {
			d := classifyPath(doc.Path, root, opt.KeepPrefix)
			if d.Reason != "" {
				ac.reject("doc", tenant, path, "", d.Reason)
			} else {
				ac.add("doc", tenant, "skipped", reasonTenantMismatch, path, "")
			}
			if _, ok := whyNot[id]; id != "" && !ok {
				whyNot[id] = reasonTenantMismatch
			}
			continue
		}
		d := classifyPath(doc.Path, root, opt.KeepPrefix)
		if d.Reason != "" {
			rejectDoc(doc, d.Reason)
			continue
		}
		if !sourceIDOK(id) {
			rejectDoc(doc, "source-id")
			continue
		}
		if _, ok := seenID[id]; ok {
			// The first row with this id is still the one links resolve to.
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
			rejectDoc(doc, "title-required")
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
		if opt.HoldPersonal && isPersonalScope(doc.Scope) {
			rejectDoc(doc, "scope-personal-held")
			continue
		}
		access, reason := mapAccess(doc, opt)
		if reason != "" {
			rejectDoc(doc, reason)
			continue
		}
		candidates = append(candidates, &keptDoc{
			access:      access,
			src:         doc,
			baseLogical: d.Logical,
			logical:     d.Logical,
			title:       title,
			body:        body,
			sum:         hashBody(body),
			mtime:       mtime.UTC(),
			fromFile:    fromFile,
		})
	}
	bySource := map[string]*keptDoc{}
	for _, item := range candidates {
		bySource[strings.TrimSpace(item.src.ID)] = item
	}
	dropDoc := func(item *keptDoc, reason string) {
		rejectDoc(item.src, reason)
		item.drop = true
		delete(bySource, strings.TrimSpace(item.src.ID))
	}
	if srcDir == "" {
		for _, item := range candidates {
			if item.fromFile {
				return ac.abort(opt.DryRun, errConfig("source dir is required when a document has no version"))
			}
		}
	}

	// Source files: path-missing, symlink, text body, or attachment. This runs
	// before placement so a document that is not stored never claims a path.
	var kept []*keptDoc
	for _, item := range candidates {
		if srcDir == "" {
			kept = append(kept, item)
			continue
		}
		info, err := inspectSource(opt.SrcDir, item.src.Path)
		if err != nil {
			return ac.abort(opt.DryRun, err)
		}
		switch info.state {
		case sourceMissing:
			dropDoc(item, "path-missing")
			continue
		case sourceSymlink:
			dropDoc(item, reasonSymlink)
			continue
		case sourcePresent:
		default:
			return ac.abort(opt.DryRun, fmt.Errorf("vaultimport: source file state"))
		}
		if info.abs == "" {
			dropDoc(item, "path-missing")
			continue
		}
		if item.fromFile {
			if !textExt(item.src.Path) {
				bin, reason, err := loadBinary(info.abs, info.size, maxBinary)
				if err != nil {
					return ac.abort(opt.DryRun, err)
				}
				if reason != "" {
					dropDoc(item, reason)
					continue
				}
				if bin != nil {
					item.binary = bin
					kept = append(kept, item)
					continue
				}
			}
			raw, reason, err := loadFileBody(info.abs, info.size)
			if err != nil {
				return ac.abort(opt.DryRun, err)
			}
			if reason != "" {
				dropDoc(item, reason)
				continue
			}
			item.applyFile(raw)
			if item.body == "" {
				return ac.abort(opt.DryRun, errConfig("refusing to import an empty body"))
			}
		}
		kept = append(kept, item)
	}

	// One row per path: the widest audience keeps the plain path.
	owner := ownerLabel(st)
	for _, loser := range placeAll(kept, owner) {
		dropDoc(loser.item, loser.reason)
	}
	dirs := map[string]bool{}
	for _, item := range kept {
		if item.drop {
			continue
		}
		for p := item.logical; strings.Contains(p, "/"); {
			p = p[:strings.LastIndex(p, "/")]
			dirs[p] = true
		}
	}
	for _, item := range kept {
		if item.drop || item.binary == nil {
			continue
		}
		item.binary.logical = strings.TrimSuffix(item.logical, ".md")
		if dirs[item.binary.logical] {
			dropDoc(item, reasonPathCollision)
			continue
		}
		item.applyBinary()
	}
	claimed := map[string]*keptDoc{}
	for _, item := range kept {
		if !item.drop {
			claimed[item.logical] = item
		}
	}

	// The row at each path: new, this document's own, one it may reuse, or
	// somebody else's (path-taken).
	for _, item := range kept {
		if item.drop {
			continue
		}
		id := strings.TrimSpace(item.src.ID)
		if item.body == "" {
			return ac.abort(opt.DryRun, errConfig("refusing to import an empty body"))
		}
		existing, err := findRow(st, dest, item.logical)
		if err != nil {
			return ac.abort(opt.DryRun, err)
		}
		if existing != nil && !existing.OwnerExplicit {
			dropDoc(item, "owner-unclear")
			continue
		}
		if existing != nil {
			from := markerSource(existing.Body)
			switch {
			case from == id:
				if !importOwned(existing.Title, existing.Body, item.title, id, item.fileSum, item.fileText, item.fromFile) &&
					!fileRowIntact(existing.Body, id) {
					dropDoc(item, reasonPathTaken)
					continue
				}
			case from != "" && rowIntact(existing.Title, existing.Body, from):
				// Reuse an untouched imported row whose source moved to another
				// path in this run, or, with AdoptOrphans, whose source is gone
				// from the export.
				if _, inExport := pathByID[from]; inExport {
					other := bySource[from]
					if other == nil || other.drop || other.logical == item.logical {
						dropDoc(item, reasonPathTaken)
						continue
					}
				} else if !opt.AdoptOrphans {
					dropDoc(item, reasonPathTaken)
					continue
				}
				item.takeover = from
			default:
				dropDoc(item, reasonPathTaken)
				continue
			}
		}
		item.existing = existing
		if item.logical == item.baseLogical {
			q, ok := qualifyLogical(item.baseLogical, qualifier(item, owner), item.binary != nil)
			if ok && claimed[q] == nil {
				old, err := findRow(st, dest, q)
				if err != nil {
					return ac.abort(opt.DryRun, err)
				}
				if old != nil && old.OwnerExplicit && markerSource(old.Body) == id && rowIntact(old.Title, old.Body, id) {
					item.stale = old
				}
			}
		}
	}
	// A reused row stays with its source document while that document cannot
	// be stored. Dropping one takeover can undo another, so repeat until none
	// changes.
	for changed := true; changed; {
		changed = false
		for _, item := range kept {
			if item.drop || item.takeover == "" {
				continue
			}
			if other := bySource[item.takeover]; other == nil {
				if _, inExport := pathByID[item.takeover]; !inExport {
					continue
				}
			} else if !other.drop {
				continue
			}
			dropDoc(item, reasonPathTaken)
			changed = true
		}
	}

	for _, item := range kept {
		if item.drop {
			continue
		}
		existing := item.existing
		if existing != nil {
			item.gosoID = existing.ID
			item.docSame = existing.Title == item.title &&
				existing.Path == item.logical &&
				existing.Body == item.body &&
				existing.SHA256 == item.sum &&
				existing.Mtime.Equal(item.mtime) &&
				existing.OwnerExplicit &&
				item.access.same(existing)
			if !covers(item.access, existing) {
				// The row is written again under a new id; links see it as new.
				item.replace = true
				item.gosoID = ""
			}
		}
		item.fileSame = true
		if vaultRoot != "" {
			same, err := fileMatches(opt.VaultRoot, dest, item.logical, []byte(item.body))
			if err == nil && same && item.binary != nil {
				same, err = binaryMatches(opt.VaultRoot, dest, item.binary.logical, item.binary.sum, maxBinary)
			}
			if errors.Is(err, errSymlink) {
				dropDoc(item, reasonSymlink)
				continue
			}
			if err != nil {
				return ac.abort(opt.DryRun, err)
			}
			item.fileSame = same
		}
	}

	plans, wants, err := planLinks(st, ex.Links, bySource, pathByID, tenantByID, whyNot)
	if err != nil {
		return ac.abort(opt.DryRun, err)
	}
	failDoc := func(item *keptDoc, reason string) {
		dropDoc(item, reason)
		retargetLinks(plans, ex.Links, strings.TrimSpace(item.src.ID))
	}

	// New rows, and rows written again because their audience narrowed.
	var fresh []*keptDoc
	for _, item := range kept {
		if item.drop || (item.existing != nil && !item.replace) {
			continue
		}
		fresh = append(fresh, item)
		path := strings.TrimSpace(item.src.Path)
		action := "inserted"
		if item.replace {
			action = "updated"
		}
		if opt.DryRun {
			ac.add("doc", item.src.TenantID, action, "", path, "")
			continue
		}
		if item.replace {
			// File first: a file with no row is never served, and a scoped
			// sync must not find the old text under a new, wider row.
			if err := unpublishFile(opt.VaultRoot, dest, item.existing.Path); err != nil {
				if errors.Is(err, errSymlink) {
					failDoc(item, reasonSymlink)
					continue
				}
				return ac.abort(opt.DryRun, err)
			}
			if err := st.DeleteVaultDoc(item.existing.ID); err != nil {
				return ac.abort(opt.DryRun, err)
			}
		}
		if item.binary != nil && vaultRoot != "" {
			if _, err := publishBinary(opt.VaultRoot, dest, item.binary.logical, item.binary.abs, item.binary.sum, maxBinary); err != nil {
				if errors.Is(err, errSymlink) {
					failDoc(item, reasonSymlink)
					continue
				}
				return ac.abort(opt.DryRun, err)
			}
		}
		// Row before file: if the file write fails the row is removed, so no
		// file is left for a scoped sync to adopt at tenant scope.
		saved, err := st.PutVaultDoc(item.row(dest, ""))
		if err != nil {
			return ac.abort(opt.DryRun, err)
		}
		if vaultRoot != "" {
			if _, err := publishFile(opt.VaultRoot, dest, item.logical, []byte(item.body)); err != nil {
				if derr := st.DeleteVaultDoc(saved.ID); derr != nil {
					return ac.abort(opt.DryRun, derr)
				}
				if errors.Is(err, errSymlink) {
					failDoc(item, reasonSymlink)
					continue
				}
				return ac.abort(opt.DryRun, err)
			}
		}
		item.gosoID = saved.ID
		if err := removeStale(st, opt.VaultRoot, dest, item); err != nil {
			return ac.abort(opt.DryRun, err)
		}
		ac.add("doc", item.src.TenantID, action, "", path, "")
	}

	// Rows updated in place: the new audience covers every old reader, so
	// the file can change before the row.
	replaced := false
	for _, item := range kept {
		if item.drop || item.existing == nil || item.replace {
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
		if item.docSame && item.fileSame && linksSame && item.stale == nil {
			ac.add("doc", item.src.TenantID, "skipped", reasonUnchanged, path, "")
			continue
		}
		if opt.DryRun {
			ac.add("doc", item.src.TenantID, "updated", "", path, "")
			continue
		}
		if !item.fileSame {
			if item.binary != nil {
				if _, err := publishBinary(opt.VaultRoot, dest, item.binary.logical, item.binary.abs, item.binary.sum, maxBinary); err != nil {
					if errors.Is(err, errSymlink) {
						failDoc(item, reasonSymlink)
						continue
					}
					return ac.abort(opt.DryRun, err)
				}
			}
			if _, err := publishFile(opt.VaultRoot, dest, item.logical, []byte(item.body)); err != nil {
				if errors.Is(err, errSymlink) {
					failDoc(item, reasonSymlink)
					continue
				}
				return ac.abort(opt.DryRun, err)
			}
		}
		if !item.docSame {
			if _, err := st.PutVaultDoc(item.row(dest, item.existing.ID)); err != nil {
				return ac.abort(opt.DryRun, err)
			}
		}
		if !linksSame {
			if err := st.ReplaceVaultLinks(item.gosoID, inners); err != nil {
				return ac.abort(opt.DryRun, err)
			}
			replaced = true
		}
		if err := removeStale(st, opt.VaultRoot, dest, item); err != nil {
			return ac.abort(opt.DryRun, err)
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

// row is the vault_docs row for item. An empty id inserts.
func (item *keptDoc) row(dest, id string) store.VaultDoc {
	return store.VaultDoc{
		ID:            id,
		Title:         item.title,
		Path:          item.logical,
		SHA256:        item.sum,
		Mtime:         item.mtime,
		Body:          item.body,
		TenantID:      dest,
		OwnerExplicit: true,
		Scope:         item.access.scope,
		AgentID:       item.access.agentID,
		TeamID:        item.access.teamID,
	}
}

func findRow(st store.StoreIface, tenant, logical string) (*store.VaultDoc, error) {
	d, err := st.FindVaultDocByPath(tenant, logical)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return d, err
}

// removeStale deletes item's own earlier row at its old qualified path: the
// file first, so no scoped sync can adopt it without its scope.
func removeStale(st store.StoreIface, vaultRoot, dest string, item *keptDoc) error {
	old := item.stale
	if old == nil {
		return nil
	}
	item.stale = nil
	if err := unpublishFile(vaultRoot, dest, old.Path); err != nil {
		return err
	}
	if item.binary != nil {
		if err := unpublishFile(vaultRoot, dest, strings.TrimSuffix(old.Path, ".md")); err != nil {
			return err
		}
	}
	return st.DeleteVaultDoc(old.ID)
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

// outOfRun is a document reason that means another run (another tenant or
// root) owns the document, so its links are that run's to report.
func outOfRun(reason string) bool {
	switch reason {
	case reasonTenantMismatch, "foreign-tenant", "outside-root":
		return true
	}
	return false
}

// planLinks decides each export link. A link whose source document belongs
// to another run is skipped with that document's reason. A link whose source
// was rejected here is source-rejected. A target in another tenant or root is
// outside-tenant; a target rejected here is target-rejected; an id that is not
// in the export at all is source-missing or target-missing.
func planLinks(st store.StoreIface, links []Link, bySource map[string]*keptDoc, pathByID, tenantByID, whyNot map[string]string) ([]linkPlan, map[string][]wantLink, error) {
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
			plans[i].action = "rejected"
			plans[i].reason = "source-rejected"
			if _, known := pathByID[fromID]; !known {
				plans[i].reason = "source-missing"
			} else if why := whyNot[fromID]; outOfRun(why) {
				plans[i].action = "skipped"
				plans[i].reason = why
			}
			continue
		}
		if to == nil || to.drop {
			plans[i].action = "rejected"
			plans[i].reason = "target-rejected"
			if _, known := pathByID[toID]; !known {
				plans[i].reason = "target-missing"
				plans[i].target = ""
			} else if outOfRun(whyNot[toID]) {
				plans[i].reason = "outside-tenant"
			}
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
		if plans[i].action == "rejected" || plans[i].action == "skipped" && plans[i].reason != reasonUnchanged {
			continue
		}
		if strings.TrimSpace(l.FromDoc) == id {
			plans[i].action = "rejected"
			plans[i].reason = "source-rejected"
			continue
		}
		if strings.TrimSpace(l.ToDoc) == id {
			plans[i].action = "rejected"
			plans[i].reason = "target-rejected"
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

func isPersonalScope(scope string) bool {
	return strings.EqualFold(strings.TrimSpace(scope), "personal")
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
