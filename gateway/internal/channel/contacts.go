// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

var (
	ErrContactNotFound        = errors.New("not found")
	ErrContactConfirm         = errors.New("confirm does not match")
	ErrContactConfirmRequired = errors.New("confirm is required")
	ErrContactSelfMerge       = errors.New("cannot merge a contact into itself")
	ErrContactMerged          = errors.New("contact already merged")
	ErrContactNoUndo          = errors.New("nothing to undo")
)

const (
	maxContactChannel = 64
	maxContactDest    = 128
	maxContactList    = 200
)

// Sighting is one inbound channel identity. Text and tokens are never stored.
type Sighting struct {
	Channel  string
	Dest     string
	Kind     string
	SenderID string
	AgentID  string
	TenantID string
	At       time.Time
}

// PublicIdent is one channel identifier on a canonical contact.
type PublicIdent struct {
	Channel    string `json:"channel"`
	Dest       string `json:"dest"`
	Kind       string `json:"kind"`
	Permission string `json:"permission"`
}

// PublicContact is the GET row. No token/code/secret/content fields.
type PublicContact struct {
	ID          string        `json:"id"`
	Display     string        `json:"display"`
	Kind        string        `json:"kind"`
	Channel     string        `json:"channel"`
	Dest        string        `json:"dest"`
	Identifiers []PublicIdent `json:"identifiers"`
	Count       int           `json:"count"`
	FirstSeen   time.Time     `json:"first_seen"`
	LastSeen    time.Time     `json:"last_seen"`
	Permission  string        `json:"permission"`
	AgentID     string        `json:"agent_id,omitempty"`
	CanUndo     bool          `json:"can_undo,omitempty"`
	MergedFrom  []string      `json:"merged_from,omitempty"`
}

type ident struct {
	Channel    string
	Dest       string
	Kind       string
	Permission string
}

type mergeEvent struct {
	SourceID      string
	SourceDisplay string
	SourceKind    string
	SourceChannel string
	SourceDest    string
	SourceCount   int
	SourceAgent   string
	SourceFirst   time.Time
	SourceLast    time.Time
	Moved         []ident
	At            time.Time
	Undone        bool
}

type contact struct {
	ID          string
	TenantID    string
	Display     string
	Kind        string
	Channel     string
	Dest        string
	Identifiers []ident
	Count       int
	FirstSeen   time.Time
	LastSeen    time.Time
	AgentID     string
	Merged      bool
	MergedInto  string
	Merges      []mergeEvent
}

// Contacts is an in-memory canonical directory sourced from inbound identities.
type Contacts struct {
	mu    sync.Mutex
	seq   atomic.Int64
	rows  map[string]*contact // id -> contact
	index map[string]string   // tenant|channel|dest -> id
}

var (
	defaultContactsMu sync.Mutex
	defaultContacts   = NewContacts()
)

// NewContacts returns an empty directory.
func NewContacts() *Contacts {
	return &Contacts{
		rows:  map[string]*contact{},
		index: map[string]string{},
	}
}

// DefaultContacts is the process-wide directory used by inbound + HTTP.
func DefaultContacts() *Contacts {
	defaultContactsMu.Lock()
	defer defaultContactsMu.Unlock()
	return defaultContacts
}

// SetDefaultContacts replaces the process-wide directory (tests).
func SetDefaultContacts(c *Contacts) {
	defaultContactsMu.Lock()
	defer defaultContactsMu.Unlock()
	if c == nil {
		c = NewContacts()
	}
	defaultContacts = c
}

func contactIdentKey(tenant, channel, dest string) string {
	return store.NormalizeTenant(tenant) + "|" + channel + "|" + dest
}

func normalizeKind(dest, hint string) string {
	h := strings.ToLower(strings.TrimSpace(hint))
	switch h {
	case "group":
		return "group"
	case "direct", "user":
		return "user"
	}
	if strings.HasPrefix(strings.TrimSpace(dest), "-") {
		return "group"
	}
	return "user"
}

func permissionOf(kind string) string {
	if kind == "group" {
		return "group"
	}
	return "direct"
}

func (c *Contacts) nextID() string {
	n := c.seq.Add(1)
	return "ct_" + itoaPending(n)
}

func displayOf(channel, dest string) string {
	ch := strings.TrimSpace(channel)
	d := strings.TrimSpace(dest)
	if ch != "" && d != "" {
		return ch + ":" + d
	}
	return d + ch
}

// Observe records one inbound identity. Payloads are discarded.
func (c *Contacts) Observe(in Sighting) PublicContact {
	if c == nil {
		return PublicContact{}
	}
	out := c.observeOne(in)
	sid := clip(in.SenderID, maxContactDest)
	dest := clip(in.Dest, maxContactDest)
	if sid != "" && sid != dest {
		in.Dest = sid
		in.Kind = "user"
		in.SenderID = ""
		c.observeOne(in)
	}
	return out
}

func (c *Contacts) observeOne(in Sighting) PublicContact {
	ch := clip(in.Channel, maxContactChannel)
	dest := clip(in.Dest, maxContactDest)
	if ch == "" || dest == "" {
		return PublicContact{}
	}
	kind := normalizeKind(dest, in.Kind)
	perm := permissionOf(kind)
	agentID := strings.TrimSpace(in.AgentID)
	tenant := store.NormalizeTenant(in.TenantID)
	at := in.At
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	key := contactIdentKey(tenant, ch, dest)

	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.index[key]
	row := c.rows[id]
	if row == nil || row.Merged {
		id = c.nextID()
		row = &contact{
			ID:          id,
			TenantID:    tenant,
			Display:     displayOf(ch, dest),
			Kind:        kind,
			Channel:     ch,
			Dest:        dest,
			Identifiers: []ident{{Channel: ch, Dest: dest, Kind: kind, Permission: perm}},
			Count:       1,
			FirstSeen:   at,
			LastSeen:    at,
			AgentID:     agentID,
		}
		c.rows[id] = row
		c.index[key] = id
		return publicContact(row)
	}
	row.Count++
	if at.Before(row.FirstSeen) {
		row.FirstSeen = at
	}
	if at.After(row.LastSeen) {
		row.LastSeen = at
	}
	if row.AgentID == "" {
		row.AgentID = agentID
	}
	return publicContact(row)
}

// ObserveDefault records onto the process-wide directory.
func ObserveDefault(in Sighting) PublicContact {
	return DefaultContacts().Observe(in)
}

// List returns live (non-merged) contacts for tenant, newest last-seen first.
func (c *Contacts) List(tenant, q, channel, kind string) []PublicContact {
	if c == nil {
		return []PublicContact{}
	}
	want := store.NormalizeTenant(tenant)
	q = strings.ToLower(strings.TrimSpace(q))
	channel = strings.TrimSpace(channel)
	kind = strings.ToLower(strings.TrimSpace(kind))
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]PublicContact, 0, len(c.rows))
	for _, row := range c.rows {
		if row == nil || row.Merged || !store.SameTenant(row.TenantID, want) {
			continue
		}
		p := publicContact(row)
		if channel != "" && p.Channel != channel {
			if !identHasChannel(p.Identifiers, channel) {
				continue
			}
		}
		if kind != "" && p.Kind != kind {
			continue
		}
		if q != "" && !contactMatches(p, q) {
			continue
		}
		out = append(out, p)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].LastSeen.After(out[i].LastSeen) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > maxContactList {
		out = out[:maxContactList]
	}
	return out
}

func identHasChannel(ids []PublicIdent, channel string) bool {
	for _, id := range ids {
		if id.Channel == channel {
			return true
		}
	}
	return false
}

func contactMatches(p PublicContact, q string) bool {
	if strings.Contains(strings.ToLower(p.ID), q) {
		return true
	}
	if strings.Contains(strings.ToLower(p.Display), q) {
		return true
	}
	if strings.Contains(strings.ToLower(p.Channel), q) {
		return true
	}
	if strings.Contains(strings.ToLower(p.Dest), q) {
		return true
	}
	for _, id := range p.Identifiers {
		if strings.Contains(strings.ToLower(id.Channel), q) || strings.Contains(strings.ToLower(id.Dest), q) {
			return true
		}
	}
	return false
}

// Get returns one live contact or ErrContactNotFound.
func (c *Contacts) Get(id, tenant string) (PublicContact, error) {
	if c == nil {
		return PublicContact{}, ErrContactNotFound
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	row := c.rows[strings.TrimSpace(id)]
	if row == nil || row.Merged || !store.SameTenant(row.TenantID, store.NormalizeTenant(tenant)) {
		return PublicContact{}, ErrContactNotFound
	}
	return publicContact(row), nil
}

// MergeConfirmOK is true when typed matches source id, target id, source dest, or source>target.
func MergeConfirmOK(typed string, target, source PublicContact) bool {
	v := strings.TrimSpace(typed)
	if v == "" {
		return false
	}
	if v == source.ID || v == target.ID || v == source.Dest {
		return true
	}
	return v == source.ID+">"+target.ID
}

// UndoConfirmOK is true when typed matches target id or the last merged source id.
func UndoConfirmOK(typed string, target PublicContact, lastSource string) bool {
	v := strings.TrimSpace(typed)
	if v == "" {
		return false
	}
	return v == target.ID || (lastSource != "" && v == lastSource)
}

// Merge moves source identifiers onto target. Requires a matching confirm name.
func (c *Contacts) Merge(targetID, sourceID, tenant, confirm string) (PublicContact, error) {
	if c == nil {
		return PublicContact{}, ErrContactNotFound
	}
	if strings.TrimSpace(confirm) == "" {
		return PublicContact{}, ErrContactConfirmRequired
	}
	tid := strings.TrimSpace(targetID)
	sid := strings.TrimSpace(sourceID)
	if tid == "" || sid == "" {
		return PublicContact{}, ErrContactNotFound
	}
	if tid == sid {
		return PublicContact{}, ErrContactSelfMerge
	}
	want := store.NormalizeTenant(tenant)

	c.mu.Lock()
	defer c.mu.Unlock()
	target := c.rows[tid]
	source := c.rows[sid]
	if target == nil || target.Merged || !store.SameTenant(target.TenantID, want) {
		return PublicContact{}, ErrContactNotFound
	}
	if source == nil || !store.SameTenant(source.TenantID, want) {
		return PublicContact{}, ErrContactNotFound
	}
	if source.Merged {
		return PublicContact{}, ErrContactMerged
	}
	if !MergeConfirmOK(confirm, publicContact(target), publicContact(source)) {
		return PublicContact{}, ErrContactConfirm
	}

	have := map[string]struct{}{}
	for _, id := range target.Identifiers {
		have[id.Channel+"|"+id.Dest] = struct{}{}
	}
	for _, id := range source.Identifiers {
		key := id.Channel + "|" + id.Dest
		idx := contactIdentKey(source.TenantID, id.Channel, id.Dest)
		c.index[idx] = target.ID
		if _, ok := have[key]; ok {
			continue
		}
		have[key] = struct{}{}
		target.Identifiers = append(target.Identifiers, id)
	}
	target.Count += source.Count
	if source.FirstSeen.Before(target.FirstSeen) {
		target.FirstSeen = source.FirstSeen
	}
	if source.LastSeen.After(target.LastSeen) {
		target.LastSeen = source.LastSeen
	}
	if target.AgentID == "" {
		target.AgentID = source.AgentID
	}
	target.Merges = append(target.Merges, mergeEvent{
		SourceID:      source.ID,
		SourceDisplay: source.Display,
		SourceKind:    source.Kind,
		SourceChannel: source.Channel,
		SourceDest:    source.Dest,
		SourceCount:   source.Count,
		SourceAgent:   source.AgentID,
		SourceFirst:   source.FirstSeen,
		SourceLast:    source.LastSeen,
		Moved:         append([]ident(nil), source.Identifiers...),
		At:            time.Now().UTC(),
	})
	source.Merged = true
	source.MergedInto = target.ID
	return publicContact(target), nil
}

// Undo reverses the last non-undone merge on target. Requires confirm.
func (c *Contacts) Undo(targetID, tenant, confirm string) (PublicContact, error) {
	if c == nil {
		return PublicContact{}, ErrContactNotFound
	}
	if strings.TrimSpace(confirm) == "" {
		return PublicContact{}, ErrContactConfirmRequired
	}
	tid := strings.TrimSpace(targetID)
	want := store.NormalizeTenant(tenant)

	c.mu.Lock()
	defer c.mu.Unlock()
	target := c.rows[tid]
	if target == nil || target.Merged || !store.SameTenant(target.TenantID, want) {
		return PublicContact{}, ErrContactNotFound
	}
	idx := -1
	for i := len(target.Merges) - 1; i >= 0; i-- {
		if !target.Merges[i].Undone {
			idx = i
			break
		}
	}
	if idx < 0 {
		return PublicContact{}, ErrContactNoUndo
	}
	ev := target.Merges[idx]
	if !UndoConfirmOK(confirm, publicContact(target), ev.SourceID) {
		return PublicContact{}, ErrContactConfirm
	}

	source := c.rows[ev.SourceID]
	if source == nil {
		source = &contact{ID: ev.SourceID, TenantID: target.TenantID}
		c.rows[ev.SourceID] = source
	}
	source.Display = ev.SourceDisplay
	source.Kind = ev.SourceKind
	source.Channel = ev.SourceChannel
	source.Dest = ev.SourceDest
	source.Count = ev.SourceCount
	source.AgentID = ev.SourceAgent
	source.FirstSeen = ev.SourceFirst
	source.LastSeen = ev.SourceLast
	source.Identifiers = append([]ident(nil), ev.Moved...)
	source.Merged = false
	source.MergedInto = ""

	keep := make([]ident, 0, len(target.Identifiers))
	drop := map[string]struct{}{}
	for _, id := range ev.Moved {
		drop[id.Channel+"|"+id.Dest] = struct{}{}
		c.index[contactIdentKey(target.TenantID, id.Channel, id.Dest)] = source.ID
	}
	for _, id := range target.Identifiers {
		if _, ok := drop[id.Channel+"|"+id.Dest]; ok {
			continue
		}
		keep = append(keep, id)
	}
	if len(keep) == 0 {
		keep = []ident{{Channel: target.Channel, Dest: target.Dest, Kind: target.Kind, Permission: permissionOf(target.Kind)}}
		c.index[contactIdentKey(target.TenantID, target.Channel, target.Dest)] = target.ID
	}
	target.Identifiers = keep
	if target.Count > ev.SourceCount {
		target.Count -= ev.SourceCount
	}
	target.Merges[idx].Undone = true
	return publicContact(target), nil
}

func publicContact(row *contact) PublicContact {
	ids := make([]PublicIdent, 0, len(row.Identifiers))
	for _, id := range row.Identifiers {
		ids = append(ids, PublicIdent{
			Channel:    id.Channel,
			Dest:       id.Dest,
			Kind:       id.Kind,
			Permission: id.Permission,
		})
	}
	from := make([]string, 0)
	canUndo := false
	for i := range row.Merges {
		if row.Merges[i].Undone {
			continue
		}
		from = append(from, row.Merges[i].SourceID)
		canUndo = true
	}
	return PublicContact{
		ID:          row.ID,
		Display:     row.Display,
		Kind:        row.Kind,
		Channel:     row.Channel,
		Dest:        row.Dest,
		Identifiers: ids,
		Count:       row.Count,
		FirstSeen:   row.FirstSeen,
		LastSeen:    row.LastSeen,
		Permission:  permissionOf(row.Kind),
		AgentID:     row.AgentID,
		CanUndo:     canUndo,
		MergedFrom:  from,
	}
}
