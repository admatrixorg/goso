// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// Contact directory persistence.
//
// Natural identity is (tenant_id, channel, dest). The in-memory directory
// already indexes a sighting as tenant|channel|dest: dest is the channel's
// external id (peer id, group chat id, or a sender id split into its own
// row). One canonical contact can own many of those pairs after a merge, so
// the unique key lives on contact_identifiers, not on the contact row.
// Two tenants may use the same channel and dest; the tenant_id column keeps
// those rows apart. This shape is goso's contact model. It is not a copy of
// any other product's contacts table.
//
// GOSO_CONTACTS_DURABLE defaults off. These tables are created either way
// (additive, empty). The directory reads and writes them only when the flag
// is on. Memory, vault, and knowledge-graph tables are not modified here.

const (
	maxContactChannelLen = 64
	maxContactDestLen    = 128
	maxContactDisplayLen = 256
	maxContactAgentLen   = 256
)

var (
	ErrContactConfirmRequired = errors.New("confirm is required")
	ErrContactConfirm         = errors.New("confirm does not match")
	ErrContactSelfMerge       = errors.New("cannot merge a contact into itself")
	ErrContactMerged          = errors.New("contact already merged")
	ErrContactNoUndo          = errors.New("nothing to undo")
)

// contactLogf writes operator logs. Callers pass counts only.
var contactLogf = log.Printf

// contactsMigrateFault, when set, aborts migrateContacts after the named step.
// Tests use it to prove the migration rolls back. Production leaves it nil.
var contactsMigrateFault func(step string) error

// ContactIdent is one channel-external id on a canonical contact.
type ContactIdent struct {
	Channel    string `json:"channel"`
	Dest       string `json:"dest"`
	Kind       string `json:"kind"`
	Permission string `json:"permission"`
}

// ContactRow is one live canonical contact. No token, code, or message fields.
type ContactRow struct {
	ID          string
	TenantID    string
	Display     string
	Kind        string
	Channel     string
	Dest        string
	Identifiers []ContactIdent
	Count       int
	FirstSeen   time.Time
	LastSeen    time.Time
	AgentID     string
	CanUndo     bool
	MergedFrom  []string
}

// ContactSighting is one inbound identity. Text and tokens are not accepted.
type ContactSighting struct {
	Channel  string
	Dest     string
	Kind     string
	AgentID  string
	TenantID string
	At       time.Time
}

// ContactSeed is one identity for BulkUpsertContacts.
// Dest is the channel-external id. The natural key is (TenantID, Channel, Dest).
type ContactSeed struct {
	TenantID  string
	Display   string
	Kind      string
	Channel   string
	Dest      string
	AgentID   string
	Count     int
	FirstSeen time.Time
	LastSeen  time.Time
}

// ContactBulkResult counts seeds applied. It does not carry identities.
type ContactBulkResult struct {
	Applied int
}

// ErrContactRollbackConfirm is returned when mapped contacts were used or
// merged after import and confirm is false. The text has no contact fields.
var ErrContactRollbackConfirm = errors.New("rollback needs confirm")

// ContactRollback is a count-only result. It does not name contacts.
type ContactRollback struct {
	Candidates int
	Active     int
	Deleted    int
}

type contactHead struct {
	ContactRow
	Merged     bool
	MergedInto string
}

type contactMergeRec struct {
	ID            string
	SourceID      string
	SourceDisplay string
	SourceKind    string
	SourceChannel string
	SourceDest    string
	SourceCount   int
	SourceAgent   string
	SourceFirst   time.Time
	SourceLast    time.Time
	Moved         []ContactIdent
	At            time.Time
	Undone        bool
}

type contactQuery interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func (s *SQLiteStore) migrateContacts() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	steps := []struct {
		name string
		sql  string
	}{
		{"contacts", `CREATE TABLE IF NOT EXISTS contacts (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			display TEXT NOT NULL,
			kind TEXT NOT NULL,
			channel TEXT NOT NULL,
			dest TEXT NOT NULL,
			seen_count INTEGER NOT NULL DEFAULT 0,
			first_seen TEXT NOT NULL,
			last_seen TEXT NOT NULL,
			agent_id TEXT NOT NULL DEFAULT '',
			merged INTEGER NOT NULL DEFAULT 0,
			merged_into TEXT NOT NULL DEFAULT ''
		)`},
		{"idx_contacts_tenant_seen", `CREATE INDEX IF NOT EXISTS idx_contacts_tenant_seen ON contacts(tenant_id, last_seen)`},
		{"contact_identifiers", `CREATE TABLE IF NOT EXISTS contact_identifiers (
			tenant_id TEXT NOT NULL,
			channel TEXT NOT NULL,
			dest TEXT NOT NULL,
			contact_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			permission TEXT NOT NULL,
			pos INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (tenant_id, channel, dest),
			FOREIGN KEY (contact_id) REFERENCES contacts(id)
		)`},
		{"idx_contact_identifiers_owner", `CREATE INDEX IF NOT EXISTS idx_contact_identifiers_owner ON contact_identifiers(tenant_id, contact_id)`},
		{"contact_merges", `CREATE TABLE IF NOT EXISTS contact_merges (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			target_id TEXT NOT NULL,
			source_id TEXT NOT NULL,
			source_display TEXT NOT NULL,
			source_kind TEXT NOT NULL,
			source_channel TEXT NOT NULL,
			source_dest TEXT NOT NULL,
			source_count INTEGER NOT NULL,
			source_agent TEXT NOT NULL DEFAULT '',
			source_first TEXT NOT NULL,
			source_last TEXT NOT NULL,
			moved_json TEXT NOT NULL,
			at TEXT NOT NULL,
			undone INTEGER NOT NULL DEFAULT 0
		)`},
		{"idx_contact_merges_target", `CREATE INDEX IF NOT EXISTS idx_contact_merges_target ON contact_merges(tenant_id, target_id)`},
		{"contact_import_map", `CREATE TABLE IF NOT EXISTS contact_import_map (
			tenant_id TEXT NOT NULL,
			contact_id TEXT NOT NULL,
			channel TEXT NOT NULL,
			dest TEXT NOT NULL,
			PRIMARY KEY (tenant_id, channel, dest)
		)`},
	}
	for _, step := range steps {
		if _, err := tx.Exec(step.sql); err != nil {
			return fmt.Errorf("migrate contacts %s: %w", step.name, err)
		}
		if contactsMigrateFault != nil {
			if err := contactsMigrateFault(step.name); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// BulkUpsertContacts inserts or updates by (tenant_id, channel, dest).
// A second call with the same keys updates the existing contact and does not
// insert another row. Seen counts are set from the seed, not incremented.
// The call is one transaction. Logs carry the applied count only.
func (s *SQLiteStore) BulkUpsertContacts(seeds []ContactSeed) (ContactBulkResult, error) {
	if s == nil || s.db == nil {
		return ContactBulkResult{}, errors.New("contact store unavailable")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ContactBulkResult{}, opaqueContactErr("upsert")
	}
	defer func() { _ = tx.Rollback() }()
	applied := 0
	for _, seed := range seeds {
		ok, err := upsertContactTx(tx, seed)
		if err != nil {
			return ContactBulkResult{}, err
		}
		if ok {
			applied++
		}
	}
	if err := tx.Commit(); err != nil {
		return ContactBulkResult{}, opaqueContactErr("upsert")
	}
	contactLogf("goso contacts: bulk upsert count=%d", applied)
	return ContactBulkResult{Applied: applied}, nil
}

// PutContactImport records one contact this importer inserted.
// RollbackContactImport deletes only those rows for the tenant.
func (s *SQLiteStore) PutContactImport(tenant, contactID, channel, dest string) error {
	if s == nil || s.db == nil {
		return errors.New("contact store unavailable")
	}
	tenant = NormalizeTenant(tenant)
	if _, err := s.db.Exec(`INSERT INTO contact_import_map(tenant_id, contact_id, channel, dest) VALUES(?,?,?,?)
		ON CONFLICT(tenant_id, channel, dest) DO NOTHING`, tenant, contactID, channel, dest); err != nil {
		return opaqueContactErr("upsert")
	}
	return nil
}

// RollbackContactImport deletes contacts this importer inserted for one tenant.
// Contacts that are not in the map stay. Active counts mapped contacts that
// were observed again (seen_count > 1) or merged after import. When active
// is greater than zero and confirm is false, nothing is deleted.
// apply false reports the counts and does not delete.
func (s *SQLiteStore) RollbackContactImport(tenant string, apply, confirm bool) (ContactRollback, error) {
	var out ContactRollback
	if s == nil || s.db == nil {
		return out, errors.New("contact store unavailable")
	}
	tenant = NormalizeTenant(tenant)
	tx, err := s.db.Begin()
	if err != nil {
		return out, opaqueContactErr("upsert")
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Query(`SELECT contact_id FROM contact_import_map WHERE tenant_id=?`, tenant)
	if err != nil {
		return out, opaqueContactErr("lookup")
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return out, opaqueContactErr("lookup")
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return out, opaqueContactErr("lookup")
	}
	rows.Close()
	seen := map[string]struct{}{}
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	out.Candidates = len(unique)
	for _, id := range unique {
		var seenCount, merged int
		err := tx.QueryRow(`SELECT seen_count, merged FROM contacts WHERE tenant_id=? AND id=?`, tenant, id).Scan(&seenCount, &merged)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return out, opaqueContactErr("lookup")
		}
		var merges int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM contact_merges WHERE tenant_id=? AND (source_id=? OR target_id=?)`, tenant, id, id).Scan(&merges); err != nil {
			return out, opaqueContactErr("lookup")
		}
		if seenCount > 1 || merged != 0 || merges > 0 {
			out.Active++
		}
	}
	if !apply {
		return out, nil
	}
	if out.Active > 0 && !confirm {
		return out, ErrContactRollbackConfirm
	}
	for _, id := range unique {
		if _, err := tx.Exec(`DELETE FROM contact_identifiers WHERE tenant_id=? AND contact_id=?`, tenant, id); err != nil {
			return out, opaqueContactErr("upsert")
		}
		res, err := tx.Exec(`DELETE FROM contacts WHERE tenant_id=? AND id=?`, tenant, id)
		if err != nil {
			return out, opaqueContactErr("upsert")
		}
		aff, _ := res.RowsAffected()
		out.Deleted += int(aff)
	}
	if _, err := tx.Exec(`DELETE FROM contact_import_map WHERE tenant_id=?`, tenant); err != nil {
		return out, opaqueContactErr("upsert")
	}
	if err := tx.Commit(); err != nil {
		return out, opaqueContactErr("upsert")
	}
	return out, nil
}

func upsertContactTx(tx contactQuery, seed ContactSeed) (bool, error) {
	ch := clipContact(seed.Channel, maxContactChannelLen)
	dest := clipContact(seed.Dest, maxContactDestLen)
	if ch == "" || dest == "" {
		return false, nil
	}
	tenant := NormalizeTenant(seed.TenantID)
	kind := contactKind(dest, seed.Kind)
	perm := contactPermission(kind)
	display := clipContact(seed.Display, maxContactDisplayLen)
	if display == "" {
		display = contactDisplay(ch, dest)
	}
	agent := clipContact(seed.AgentID, maxContactAgentLen)
	count := seed.Count
	if count < 1 {
		count = 1
	}
	first := seed.FirstSeen.UTC()
	last := seed.LastSeen.UTC()
	now := time.Now().UTC()
	if first.IsZero() {
		first = now
	}
	if last.IsZero() {
		last = first
	}
	id, found, err := lookupContactID(tx, tenant, ch, dest)
	if err != nil {
		return false, err
	}
	if !found {
		id = newContactID()
		if err := insertContactTx(tx, ContactRow{
			ID: id, TenantID: tenant, Display: display, Kind: kind,
			Channel: ch, Dest: dest, Count: count,
			FirstSeen: first, LastSeen: last, AgentID: agent,
		}); err != nil {
			return false, opaqueContactErr("upsert")
		}
		if err := insertIdentTx(tx, tenant, id, ContactIdent{Channel: ch, Dest: dest, Kind: kind, Permission: perm}, 0); err != nil {
			return false, opaqueContactErr("upsert")
		}
		return true, nil
	}
	if _, err := tx.Exec(`UPDATE contacts SET display=?, kind=?, seen_count=?, first_seen=?, last_seen=?, agent_id=? WHERE id=? AND tenant_id=?`,
		display, kind, count, formatTime(first), formatTime(last), agent, id, tenant); err != nil {
		return false, opaqueContactErr("upsert")
	}
	if _, err := tx.Exec(`UPDATE contact_identifiers SET kind=?, permission=? WHERE tenant_id=? AND channel=? AND dest=?`,
		kind, perm, tenant, ch, dest); err != nil {
		return false, opaqueContactErr("upsert")
	}
	return true, nil
}

// ObserveContact records one sighting. An existing (tenant, channel, dest)
// increments the same contact. Empty channel or dest is ignored.
func (s *SQLiteStore) ObserveContact(in ContactSighting) (ContactRow, error) {
	if s == nil || s.db == nil {
		return ContactRow{}, errors.New("contact store unavailable")
	}
	ch := clipContact(in.Channel, maxContactChannelLen)
	dest := clipContact(in.Dest, maxContactDestLen)
	if ch == "" || dest == "" {
		return ContactRow{}, nil
	}
	for attempt := 0; attempt < 2; attempt++ {
		row, err := s.observeContactOnce(in, ch, dest)
		if err == nil {
			return row, nil
		}
		if contactUniqueErr(err) && attempt == 0 {
			continue
		}
		return ContactRow{}, opaqueContactCause("observe", err)
	}
	return ContactRow{}, opaqueContactErr("observe")
}

func (s *SQLiteStore) observeContactOnce(in ContactSighting, ch, dest string) (ContactRow, error) {
	tenant := NormalizeTenant(in.TenantID)
	kind := contactKind(dest, in.Kind)
	perm := contactPermission(kind)
	agent := clipContact(in.AgentID, maxContactAgentLen)
	at := in.At.UTC()
	if at.IsZero() {
		at = time.Now().UTC()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ContactRow{}, opaqueContactCause("observe", err)
	}
	defer func() { _ = tx.Rollback() }()
	id, found, err := lookupContactID(tx, tenant, ch, dest)
	if err != nil {
		return ContactRow{}, err
	}
	if !found {
		id = newContactID()
		row := ContactRow{
			ID: id, TenantID: tenant, Display: contactDisplay(ch, dest), Kind: kind,
			Channel: ch, Dest: dest, Count: 1, FirstSeen: at, LastSeen: at, AgentID: agent,
			Identifiers: []ContactIdent{{Channel: ch, Dest: dest, Kind: kind, Permission: perm}},
		}
		if err := insertContactTx(tx, row); err != nil {
			return ContactRow{}, err
		}
		if err := insertIdentTx(tx, tenant, id, row.Identifiers[0], 0); err != nil {
			return ContactRow{}, err
		}
		if err := tx.Commit(); err != nil {
			return ContactRow{}, opaqueContactErr("observe")
		}
		return row, nil
	}
	head, err := loadContactTx(tx, id, tenant, false)
	if err != nil {
		return ContactRow{}, err
	}
	if head.Merged {
		// The identifier still points at a merged row. Open a live contact
		// and repoint that one key. The key stays unique.
		id = newContactID()
		row := ContactRow{
			ID: id, TenantID: tenant, Display: contactDisplay(ch, dest), Kind: kind,
			Channel: ch, Dest: dest, Count: 1, FirstSeen: at, LastSeen: at, AgentID: agent,
			Identifiers: []ContactIdent{{Channel: ch, Dest: dest, Kind: kind, Permission: perm}},
		}
		if err := insertContactTx(tx, row); err != nil {
			return ContactRow{}, err
		}
		if _, err := tx.Exec(`UPDATE contact_identifiers SET contact_id=?, kind=?, permission=?, pos=0 WHERE tenant_id=? AND channel=? AND dest=?`,
			id, kind, perm, tenant, ch, dest); err != nil {
			return ContactRow{}, opaqueContactErr("observe")
		}
		if err := tx.Commit(); err != nil {
			return ContactRow{}, opaqueContactErr("observe")
		}
		return row, nil
	}
	head.Count++
	if at.Before(head.FirstSeen) {
		head.FirstSeen = at
	}
	if at.After(head.LastSeen) {
		head.LastSeen = at
	}
	if head.AgentID == "" {
		head.AgentID = agent
	}
	if _, err := tx.Exec(`UPDATE contacts SET seen_count=?, first_seen=?, last_seen=?, agent_id=? WHERE id=? AND tenant_id=?`,
		head.Count, formatTime(head.FirstSeen), formatTime(head.LastSeen), head.AgentID, id, tenant); err != nil {
		return ContactRow{}, opaqueContactErr("observe")
	}
	if err := tx.Commit(); err != nil {
		return ContactRow{}, opaqueContactErr("observe")
	}
	return s.GetContact(id, tenant)
}

// ListContacts returns live contacts for one tenant, newest last_seen first.
// There is no 200 cap. q, channel, and kind filter that tenant's rows only.
func (s *SQLiteStore) ListContacts(tenant, q, channel, kind string) ([]ContactRow, error) {
	if s == nil || s.db == nil {
		return []ContactRow{}, errors.New("contact store unavailable")
	}
	tenant = NormalizeTenant(tenant)
	q = strings.ToLower(strings.TrimSpace(q))
	channel = strings.TrimSpace(channel)
	kind = strings.ToLower(strings.TrimSpace(kind))
	heads, err := s.scanContacts(`SELECT id, tenant_id, display, kind, channel, dest, seen_count, first_seen, last_seen, agent_id, merged, merged_into FROM contacts WHERE tenant_id=? AND merged=0`, tenant)
	if err != nil {
		return nil, err
	}
	idents, err := loadIdentMap(s.db, tenant)
	if err != nil {
		return nil, err
	}
	merges, err := loadMergeHeads(s.db, tenant)
	if err != nil {
		return nil, err
	}
	out := make([]ContactRow, 0, len(heads))
	for _, head := range heads {
		if !SameTenant(head.TenantID, tenant) || head.Merged {
			continue
		}
		row := finishContact(head, idents[head.ID], merges[head.ID])
		if channel != "" && !contactHasChannel(row, channel) {
			continue
		}
		if kind != "" && row.Kind != kind {
			continue
		}
		if q != "" && !contactRowMatches(row, q) {
			continue
		}
		out = append(out, row)
	}
	sortContacts(out)
	return out, nil
}

// CountContacts counts live contacts for one tenant. It never counts another tenant.
func (s *SQLiteStore) CountContacts(tenant string) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("contact store unavailable")
	}
	tenant = NormalizeTenant(tenant)
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM contacts WHERE tenant_id=? AND merged=0`, tenant).Scan(&n); err != nil {
		return 0, opaqueContactErr("count")
	}
	return n, nil
}

// GetContact returns one live contact in the tenant, or ErrNotFound.
// A missing id and another tenant's id return the same error.
func (s *SQLiteStore) GetContact(id, tenant string) (ContactRow, error) {
	if s == nil || s.db == nil {
		return ContactRow{}, errors.New("contact store unavailable")
	}
	id = strings.TrimSpace(id)
	tenant = NormalizeTenant(tenant)
	if id == "" {
		return ContactRow{}, ErrNotFound
	}
	head, err := loadContactTx(s.db, id, tenant, true)
	if err != nil {
		return ContactRow{}, err
	}
	idents, err := loadIdentsFor(s.db, tenant, id)
	if err != nil {
		return ContactRow{}, err
	}
	merges, err := loadMergesFor(s.db, tenant, id)
	if err != nil {
		return ContactRow{}, err
	}
	return finishContact(head, idents, merges), nil
}

// MergeContacts moves source identifiers onto target inside one tenant.
func (s *SQLiteStore) MergeContacts(targetID, sourceID, tenant, confirm string) (ContactRow, error) {
	if s == nil || s.db == nil {
		return ContactRow{}, errors.New("contact store unavailable")
	}
	if strings.TrimSpace(confirm) == "" {
		return ContactRow{}, ErrContactConfirmRequired
	}
	targetID = strings.TrimSpace(targetID)
	sourceID = strings.TrimSpace(sourceID)
	tenant = NormalizeTenant(tenant)
	if targetID == "" || sourceID == "" {
		return ContactRow{}, ErrNotFound
	}
	if targetID == sourceID {
		return ContactRow{}, ErrContactSelfMerge
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ContactRow{}, opaqueContactErr("merge")
	}
	defer func() { _ = tx.Rollback() }()
	target, err := loadFullTx(tx, targetID, tenant, true)
	if err != nil {
		return ContactRow{}, err
	}
	source, err := loadFullTx(tx, sourceID, tenant, false)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ContactRow{}, ErrNotFound
		}
		return ContactRow{}, err
	}
	if source.Merged {
		return ContactRow{}, ErrContactMerged
	}
	if !contactMergeConfirmOK(confirm, target.ContactRow, source.ContactRow) {
		return ContactRow{}, ErrContactConfirm
	}
	moved := append([]ContactIdent(nil), source.Identifiers...)
	next, err := nextIdentPos(tx, tenant, target.ID)
	if err != nil {
		return ContactRow{}, err
	}
	have := map[string]struct{}{}
	for _, id := range target.Identifiers {
		have[id.Channel+"|"+id.Dest] = struct{}{}
	}
	for _, id := range moved {
		key := id.Channel + "|" + id.Dest
		if _, ok := have[key]; ok {
			if _, err := tx.Exec(`UPDATE contact_identifiers SET contact_id=? WHERE tenant_id=? AND channel=? AND dest=?`,
				target.ID, tenant, id.Channel, id.Dest); err != nil {
				return ContactRow{}, opaqueContactErr("merge")
			}
			continue
		}
		have[key] = struct{}{}
		if _, err := tx.Exec(`UPDATE contact_identifiers SET contact_id=?, pos=? WHERE tenant_id=? AND channel=? AND dest=?`,
			target.ID, next, tenant, id.Channel, id.Dest); err != nil {
			return ContactRow{}, opaqueContactErr("merge")
		}
		next++
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
	if _, err := tx.Exec(`UPDATE contacts SET seen_count=?, first_seen=?, last_seen=?, agent_id=? WHERE id=? AND tenant_id=?`,
		target.Count, formatTime(target.FirstSeen), formatTime(target.LastSeen), target.AgentID, target.ID, tenant); err != nil {
		return ContactRow{}, opaqueContactErr("merge")
	}
	if _, err := tx.Exec(`UPDATE contacts SET merged=1, merged_into=? WHERE id=? AND tenant_id=?`,
		target.ID, source.ID, tenant); err != nil {
		return ContactRow{}, opaqueContactErr("merge")
	}
	raw, err := json.Marshal(moved)
	if err != nil {
		return ContactRow{}, opaqueContactErr("merge")
	}
	if _, err := tx.Exec(`INSERT INTO contact_merges(
		id, tenant_id, target_id, source_id, source_display, source_kind, source_channel, source_dest,
		source_count, source_agent, source_first, source_last, moved_json, at, undone)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,0)`,
		newContactMergeID(), tenant, target.ID, source.ID, source.Display, source.Kind, source.Channel, source.Dest,
		source.Count, source.AgentID, formatTime(source.FirstSeen), formatTime(source.LastSeen), string(raw), formatTime(time.Now().UTC())); err != nil {
		return ContactRow{}, opaqueContactErr("merge")
	}
	out, err := loadFullTx(tx, target.ID, tenant, true)
	if err != nil {
		return ContactRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return ContactRow{}, opaqueContactErr("merge")
	}
	return out.ContactRow, nil
}

// UndoContact reverses the last merge on target inside one tenant.
func (s *SQLiteStore) UndoContact(targetID, tenant, confirm string) (ContactRow, error) {
	if s == nil || s.db == nil {
		return ContactRow{}, errors.New("contact store unavailable")
	}
	if strings.TrimSpace(confirm) == "" {
		return ContactRow{}, ErrContactConfirmRequired
	}
	targetID = strings.TrimSpace(targetID)
	tenant = NormalizeTenant(tenant)
	if targetID == "" {
		return ContactRow{}, ErrNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ContactRow{}, opaqueContactErr("undo")
	}
	defer func() { _ = tx.Rollback() }()
	target, err := loadFullTx(tx, targetID, tenant, true)
	if err != nil {
		return ContactRow{}, err
	}
	ev, err := lastOpenMergeTx(tx, tenant, target.ID)
	if err != nil {
		return ContactRow{}, err
	}
	if !contactUndoConfirmOK(confirm, target.ContactRow, ev.SourceID) {
		return ContactRow{}, ErrContactConfirm
	}
	source, err := loadContactTx(tx, ev.SourceID, tenant, false)
	if errors.Is(err, ErrNotFound) {
		source = contactHead{ContactRow: ContactRow{ID: ev.SourceID, TenantID: tenant}}
		if err := insertContactTx(tx, source.ContactRow); err != nil {
			return ContactRow{}, err
		}
	} else if err != nil {
		return ContactRow{}, err
	}
	if _, err := tx.Exec(`UPDATE contacts SET display=?, kind=?, channel=?, dest=?, seen_count=?, agent_id=?, first_seen=?, last_seen=?, merged=0, merged_into='' WHERE id=? AND tenant_id=?`,
		ev.SourceDisplay, ev.SourceKind, ev.SourceChannel, ev.SourceDest, ev.SourceCount, ev.SourceAgent,
		formatTime(ev.SourceFirst), formatTime(ev.SourceLast), ev.SourceID, tenant); err != nil {
		return ContactRow{}, opaqueContactErr("undo")
	}
	for i, id := range ev.Moved {
		if _, err := tx.Exec(`UPDATE contact_identifiers SET contact_id=?, pos=? WHERE tenant_id=? AND channel=? AND dest=?`,
			ev.SourceID, i, tenant, id.Channel, id.Dest); err != nil {
			return ContactRow{}, opaqueContactErr("undo")
		}
	}
	var left int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM contact_identifiers WHERE tenant_id=? AND contact_id=?`, tenant, target.ID).Scan(&left); err != nil {
		return ContactRow{}, opaqueContactErr("undo")
	}
	if left == 0 {
		kind := contactKind(target.Dest, target.Kind)
		ident := ContactIdent{Channel: target.Channel, Dest: target.Dest, Kind: kind, Permission: contactPermission(kind)}
		if err := insertIdentTx(tx, tenant, target.ID, ident, 0); err != nil {
			return ContactRow{}, err
		}
	}
	if target.Count > ev.SourceCount {
		target.Count -= ev.SourceCount
		if _, err := tx.Exec(`UPDATE contacts SET seen_count=? WHERE id=? AND tenant_id=?`, target.Count, target.ID, tenant); err != nil {
			return ContactRow{}, opaqueContactErr("undo")
		}
	}
	if _, err := tx.Exec(`UPDATE contact_merges SET undone=1 WHERE id=? AND tenant_id=? AND target_id=?`, ev.ID, tenant, target.ID); err != nil {
		return ContactRow{}, opaqueContactErr("undo")
	}
	out, err := loadFullTx(tx, target.ID, tenant, true)
	if err != nil {
		return ContactRow{}, err
	}
	if err := tx.Commit(); err != nil {
		return ContactRow{}, opaqueContactErr("undo")
	}
	return out.ContactRow, nil
}

func lookupContactID(q contactQuery, tenant, channel, dest string) (string, bool, error) {
	var id string
	var merged int
	err := q.QueryRow(`SELECT c.id, c.merged FROM contact_identifiers i
		JOIN contacts c ON c.id=i.contact_id AND c.tenant_id=i.tenant_id
		WHERE i.tenant_id=? AND i.channel=? AND i.dest=?`, tenant, channel, dest).Scan(&id, &merged)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, opaqueContactErr("lookup")
	}
	return id, true, nil
}

func insertContactTx(q contactQuery, row ContactRow) error {
	_, err := q.Exec(`INSERT INTO contacts(id, tenant_id, display, kind, channel, dest, seen_count, first_seen, last_seen, agent_id, merged, merged_into)
		VALUES(?,?,?,?,?,?,?,?,?,?,0,'')`,
		row.ID, NormalizeTenant(row.TenantID), row.Display, row.Kind, row.Channel, row.Dest, row.Count,
		formatTime(row.FirstSeen), formatTime(row.LastSeen), row.AgentID)
	if err != nil {
		return err
	}
	return nil
}

func insertIdentTx(q contactQuery, tenant, contactID string, id ContactIdent, pos int) error {
	_, err := q.Exec(`INSERT INTO contact_identifiers(tenant_id, channel, dest, contact_id, kind, permission, pos) VALUES(?,?,?,?,?,?,?)`,
		tenant, id.Channel, id.Dest, contactID, id.Kind, id.Permission, pos)
	if err != nil {
		return err
	}
	return nil
}

func nextIdentPos(q contactQuery, tenant, contactID string) (int, error) {
	var n int
	if err := q.QueryRow(`SELECT COALESCE(MAX(pos), -1) FROM contact_identifiers WHERE tenant_id=? AND contact_id=?`, tenant, contactID).Scan(&n); err != nil {
		return 0, opaqueContactErr("lookup")
	}
	return n + 1, nil
}

func loadContactTx(q contactQuery, id, tenant string, liveOnly bool) (contactHead, error) {
	var head contactHead
	var count, merged int
	var first, last string
	err := q.QueryRow(`SELECT id, tenant_id, display, kind, channel, dest, seen_count, first_seen, last_seen, agent_id, merged, merged_into FROM contacts WHERE id=? AND tenant_id=?`,
		id, tenant).Scan(&head.ID, &head.TenantID, &head.Display, &head.Kind, &head.Channel, &head.Dest, &count, &first, &last, &head.AgentID, &merged, &head.MergedInto)
	if errors.Is(err, sql.ErrNoRows) {
		return contactHead{}, ErrNotFound
	}
	if err != nil {
		return contactHead{}, opaqueContactErr("get")
	}
	if !SameTenant(head.TenantID, tenant) {
		return contactHead{}, ErrNotFound
	}
	head.Count = count
	head.Merged = merged != 0
	head.FirstSeen = parseTime(first)
	head.LastSeen = parseTime(last)
	if liveOnly && head.Merged {
		return contactHead{}, ErrNotFound
	}
	return head, nil
}

func loadFullTx(q contactQuery, id, tenant string, liveOnly bool) (contactHead, error) {
	head, err := loadContactTx(q, id, tenant, liveOnly)
	if err != nil {
		return contactHead{}, err
	}
	idents, err := loadIdentsFor(q, tenant, id)
	if err != nil {
		return contactHead{}, err
	}
	merges, err := loadMergesFor(q, tenant, id)
	if err != nil {
		return contactHead{}, err
	}
	head.ContactRow = finishContact(head, idents, merges)
	return head, nil
}

func (s *SQLiteStore) scanContacts(query string, args ...any) ([]contactHead, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, opaqueContactCause("list", err)
	}
	defer rows.Close()
	out := []contactHead{}
	for rows.Next() {
		head, err := scanContactHead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, head)
	}
	if err := rows.Err(); err != nil {
		return nil, opaqueContactCause("list", err)
	}
	return out, nil
}

func scanContactHead(sc scanner) (contactHead, error) {
	var head contactHead
	var count, merged int
	var first, last string
	if err := sc.Scan(&head.ID, &head.TenantID, &head.Display, &head.Kind, &head.Channel, &head.Dest, &count, &first, &last, &head.AgentID, &merged, &head.MergedInto); err != nil {
		return contactHead{}, opaqueContactErr("list")
	}
	head.Count = count
	head.Merged = merged != 0
	head.FirstSeen = parseTime(first)
	head.LastSeen = parseTime(last)
	head.TenantID = NormalizeTenant(head.TenantID)
	return head, nil
}

func loadIdentMap(q contactQuery, tenant string) (map[string][]ContactIdent, error) {
	rows, err := q.Query(`SELECT contact_id, channel, dest, kind, permission FROM contact_identifiers WHERE tenant_id=? ORDER BY pos ASC, channel ASC, dest ASC`, tenant)
	if err != nil {
		return nil, opaqueContactErr("list")
	}
	defer rows.Close()
	out := map[string][]ContactIdent{}
	for rows.Next() {
		var contactID string
		var id ContactIdent
		if err := rows.Scan(&contactID, &id.Channel, &id.Dest, &id.Kind, &id.Permission); err != nil {
			return nil, opaqueContactErr("list")
		}
		out[contactID] = append(out[contactID], id)
	}
	if err := rows.Err(); err != nil {
		return nil, opaqueContactErr("list")
	}
	return out, nil
}

func loadIdentsFor(q contactQuery, tenant, contactID string) ([]ContactIdent, error) {
	rows, err := q.Query(`SELECT channel, dest, kind, permission FROM contact_identifiers WHERE tenant_id=? AND contact_id=? ORDER BY pos ASC, channel ASC, dest ASC`, tenant, contactID)
	if err != nil {
		return nil, opaqueContactErr("get")
	}
	defer rows.Close()
	out := []ContactIdent{}
	for rows.Next() {
		var id ContactIdent
		if err := rows.Scan(&id.Channel, &id.Dest, &id.Kind, &id.Permission); err != nil {
			return nil, opaqueContactErr("get")
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, opaqueContactErr("get")
	}
	return out, nil
}

func loadMergeHeads(q contactQuery, tenant string) (map[string][]contactMergeRec, error) {
	rows, err := q.Query(`SELECT target_id, source_id, undone FROM contact_merges WHERE tenant_id=? ORDER BY at ASC, id ASC`, tenant)
	if err != nil {
		return nil, opaqueContactErr("list")
	}
	defer rows.Close()
	out := map[string][]contactMergeRec{}
	for rows.Next() {
		var target string
		var rec contactMergeRec
		var undone int
		if err := rows.Scan(&target, &rec.SourceID, &undone); err != nil {
			return nil, opaqueContactErr("list")
		}
		rec.Undone = undone != 0
		out[target] = append(out[target], rec)
	}
	if err := rows.Err(); err != nil {
		return nil, opaqueContactErr("list")
	}
	return out, nil
}

func loadMergesFor(q contactQuery, tenant, targetID string) ([]contactMergeRec, error) {
	rows, err := q.Query(`SELECT id, source_id, undone FROM contact_merges WHERE tenant_id=? AND target_id=? ORDER BY at ASC, id ASC`, tenant, targetID)
	if err != nil {
		return nil, opaqueContactErr("get")
	}
	defer rows.Close()
	out := []contactMergeRec{}
	for rows.Next() {
		var rec contactMergeRec
		var undone int
		if err := rows.Scan(&rec.ID, &rec.SourceID, &undone); err != nil {
			return nil, opaqueContactErr("get")
		}
		rec.Undone = undone != 0
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, opaqueContactErr("get")
	}
	return out, nil
}

func lastOpenMergeTx(q contactQuery, tenant, targetID string) (contactMergeRec, error) {
	rows, err := q.Query(`SELECT id, source_id, source_display, source_kind, source_channel, source_dest, source_count, source_agent, source_first, source_last, moved_json, at, undone
		FROM contact_merges WHERE tenant_id=? AND target_id=? ORDER BY at ASC, id ASC`, tenant, targetID)
	if err != nil {
		return contactMergeRec{}, opaqueContactErr("undo")
	}
	defer rows.Close()
	var last contactMergeRec
	found := false
	for rows.Next() {
		var rec contactMergeRec
		var undone int
		var first, sourceLast, at, moved string
		if err := rows.Scan(&rec.ID, &rec.SourceID, &rec.SourceDisplay, &rec.SourceKind, &rec.SourceChannel, &rec.SourceDest, &rec.SourceCount, &rec.SourceAgent, &first, &sourceLast, &moved, &at, &undone); err != nil {
			return contactMergeRec{}, opaqueContactErr("undo")
		}
		rec.Undone = undone != 0
		rec.SourceFirst = parseTime(first)
		rec.SourceLast = parseTime(sourceLast)
		rec.At = parseTime(at)
		if strings.TrimSpace(moved) != "" {
			if err := json.Unmarshal([]byte(moved), &rec.Moved); err != nil {
				return contactMergeRec{}, opaqueContactErr("undo")
			}
		}
		if !rec.Undone {
			last = rec
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		return contactMergeRec{}, opaqueContactErr("undo")
	}
	if !found {
		return contactMergeRec{}, ErrContactNoUndo
	}
	return last, nil
}

func finishContact(head contactHead, idents []ContactIdent, merges []contactMergeRec) ContactRow {
	row := head.ContactRow
	row.Identifiers = append([]ContactIdent(nil), idents...)
	if row.Identifiers == nil {
		row.Identifiers = []ContactIdent{}
	}
	from := make([]string, 0)
	for _, ev := range merges {
		if ev.Undone {
			continue
		}
		from = append(from, ev.SourceID)
		row.CanUndo = true
	}
	row.MergedFrom = from
	return row
}

func sortContacts(out []ContactRow) {
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].LastSeen.After(out[i].LastSeen) || (out[j].LastSeen.Equal(out[i].LastSeen) && out[j].ID < out[i].ID) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
}

func contactHasChannel(row ContactRow, channel string) bool {
	if row.Channel == channel {
		return true
	}
	for _, id := range row.Identifiers {
		if id.Channel == channel {
			return true
		}
	}
	return false
}

func contactRowMatches(row ContactRow, q string) bool {
	if strings.Contains(strings.ToLower(row.ID), q) {
		return true
	}
	if strings.Contains(strings.ToLower(row.Display), q) {
		return true
	}
	if strings.Contains(strings.ToLower(row.Channel), q) {
		return true
	}
	if strings.Contains(strings.ToLower(row.Dest), q) {
		return true
	}
	for _, id := range row.Identifiers {
		if strings.Contains(strings.ToLower(id.Channel), q) || strings.Contains(strings.ToLower(id.Dest), q) {
			return true
		}
	}
	return false
}

func contactMergeConfirmOK(typed string, target, source ContactRow) bool {
	v := strings.TrimSpace(typed)
	if v == "" {
		return false
	}
	if v == source.ID || v == target.ID || v == source.Dest {
		return true
	}
	return v == source.ID+">"+target.ID
}

func contactUndoConfirmOK(typed string, target ContactRow, lastSource string) bool {
	v := strings.TrimSpace(typed)
	if v == "" {
		return false
	}
	return v == target.ID || (lastSource != "" && v == lastSource)
}

func contactKind(dest, hint string) string {
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

func contactPermission(kind string) string {
	if kind == "group" {
		return "group"
	}
	return "direct"
}

func contactDisplay(channel, dest string) string {
	ch := strings.TrimSpace(channel)
	d := strings.TrimSpace(dest)
	if ch != "" && d != "" {
		return ch + ":" + d
	}
	return d + ch
}

func clipContact(s string, n int) string {
	s = strings.TrimSpace(s)
	if n > 0 && len(s) > n {
		return s[:n]
	}
	return s
}

func newContactID() string      { return "ct_" + newID() }
func newContactMergeID() string { return "cm_" + newID() }

// ContactFault is a store failure whose text is safe to return to callers.
// Class is a fixed token (closed, conflict, schema, driver, store). The
// driver message is not retained, because it can echo bound values.
type ContactFault struct {
	Op    string
	Class string
}

func (e *ContactFault) Error() string {
	return "contact store " + safeContactStoreOp(e.Op) + " failed"
}

func opaqueContactErr(op string) error {
	return &ContactFault{Op: op, Class: "driver"}
}

func opaqueContactCause(op string, cause error) error {
	if cause == nil {
		return opaqueContactErr(op)
	}
	var inner *ContactFault
	if errors.As(cause, &inner) && safeContactStoreClass(inner.Class) == inner.Class && inner.Class != "" {
		return &ContactFault{Op: op, Class: inner.Class}
	}
	return &ContactFault{Op: op, Class: contactFaultClass(cause)}
}

func contactFaultClass(err error) string {
	if err == nil {
		return "driver"
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "closed"), strings.Contains(s, "conn done"):
		return "closed"
	case strings.Contains(s, "unique"), strings.Contains(s, "duplicate"):
		return "conflict"
	case strings.Contains(s, "no such"), strings.Contains(s, "syntax"):
		return "schema"
	default:
		return "driver"
	}
}

func safeContactStoreOp(op string) string {
	switch op {
	case "observe", "list", "upsert", "get", "merge", "undo", "count", "lookup":
		return op
	default:
		return "op"
	}
}

func safeContactStoreClass(class string) string {
	switch class {
	case "closed", "conflict", "schema", "driver", "store":
		return class
	default:
		return "driver"
	}
}

func contactUniqueErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique") || strings.Contains(s, "duplicate key")
}
