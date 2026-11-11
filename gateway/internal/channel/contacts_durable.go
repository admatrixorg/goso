// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"errors"
	"log"
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// contactFaultLogf writes durable-store failures. The format is
// "goso contacts: <op> failed class=<class>" with both tokens drawn from a
// fixed set. Names, phones, dests, contact ids, tenants, and message text
// are not passed in.
var contactFaultLogf = log.Printf

// ContactsDurableEnabled reports GOSO_CONTACTS_DURABLE.
// Unset, empty, 0, false, no, and off keep the in-memory directory.
func ContactsDurableEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_CONTACTS_DURABLE"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (c *Contacts) durable() bool {
	return c != nil && ContactsDurableEnabled() && c.db.Load() != nil
}

// BindStore attaches a SQLite or Postgres store. The directory ignores it
// until ContactsDurableEnabled is true.
func (c *Contacts) BindStore(db *store.SQLiteStore) {
	if c == nil {
		return
	}
	c.db.Store(db)
}

// AttachContactsStore binds the SQL store behind st when st is SQLite or Postgres.
// The in-memory store leaves the directory unchanged.
func AttachContactsStore(c *Contacts, st store.StoreIface) {
	db := contactDB(st)
	if c == nil || db == nil {
		return
	}
	c.BindStore(db)
}

func contactDB(st store.StoreIface) *store.SQLiteStore {
	switch s := st.(type) {
	case *store.SQLiteStore:
		return s
	case *store.PostgresStore:
		if s == nil {
			return nil
		}
		return s.SQLiteStore
	default:
		return nil
	}
}

func (c *Contacts) observeDurable(in Sighting) PublicContact {
	db := c.db.Load()
	if db == nil {
		logContactFault("observe", &store.ContactFault{Op: "observe", Class: "store"})
		return PublicContact{}
	}
	out, err := observeContactStore(db, sightingStore(in))
	if err != nil {
		logContactFault("observe", err)
		return PublicContact{}
	}
	sid := clip(in.SenderID, maxContactDest)
	dest := clip(in.Dest, maxContactDest)
	if sid != "" && sid != dest {
		in.Dest = sid
		in.Kind = "user"
		in.SenderID = ""
		if _, err := observeContactStore(db, sightingStore(in)); err != nil {
			logContactFault("observe", err)
		}
	}
	return publicFromStore(out)
}

// observeContactStore is the store call. Tests replace it to inject a failure.
var observeContactStore = func(db *store.SQLiteStore, in store.ContactSighting) (store.ContactRow, error) {
	if db == nil {
		return store.ContactRow{}, &store.ContactFault{Op: "observe", Class: "store"}
	}
	return db.ObserveContact(in)
}

func (c *Contacts) listDurable(tenant, q, channelName, kind string) ([]PublicContact, error) {
	db := c.db.Load()
	if db == nil {
		err := &store.ContactFault{Op: "list", Class: "store"}
		logContactFault("list", err)
		return nil, err
	}
	rows, err := db.ListContacts(tenant, q, channelName, kind)
	if err != nil {
		logContactFault("list", err)
		return nil, err
	}
	out := make([]PublicContact, 0, len(rows))
	for _, row := range rows {
		out = append(out, publicFromStore(row))
	}
	return out, nil
}

func logContactFault(op string, err error) {
	if err == nil {
		return
	}
	class := "driver"
	var fault *store.ContactFault
	if errors.As(err, &fault) {
		class = fault.Class
		if fault.Op != "" {
			op = fault.Op
		}
	}
	contactFaultLogf("goso contacts: %s failed class=%s", safeContactOp(op), safeContactClass(class))
}

func safeContactOp(op string) string {
	switch op {
	case "observe", "list", "upsert", "get", "merge", "undo", "count", "lookup":
		return op
	default:
		return "op"
	}
}

func safeContactClass(class string) string {
	switch class {
	case "closed", "conflict", "schema", "driver", "store":
		return class
	default:
		return "driver"
	}
}

func (c *Contacts) getDurable(id, tenant string) (PublicContact, error) {
	db := c.db.Load()
	if db == nil {
		return PublicContact{}, ErrContactNotFound
	}
	row, err := db.GetContact(id, tenant)
	if err != nil {
		return PublicContact{}, mapContactErr(err)
	}
	return publicFromStore(row), nil
}

func (c *Contacts) mergeDurable(targetID, sourceID, tenant, confirm string) (PublicContact, error) {
	db := c.db.Load()
	if db == nil {
		return PublicContact{}, ErrContactNotFound
	}
	row, err := db.MergeContacts(targetID, sourceID, tenant, confirm)
	if err != nil {
		return PublicContact{}, mapContactErr(err)
	}
	return publicFromStore(row), nil
}

func (c *Contacts) undoDurable(targetID, tenant, confirm string) (PublicContact, error) {
	db := c.db.Load()
	if db == nil {
		return PublicContact{}, ErrContactNotFound
	}
	row, err := db.UndoContact(targetID, tenant, confirm)
	if err != nil {
		return PublicContact{}, mapContactErr(err)
	}
	return publicFromStore(row), nil
}

func sightingStore(in Sighting) store.ContactSighting {
	return store.ContactSighting{
		Channel:  in.Channel,
		Dest:     in.Dest,
		Kind:     in.Kind,
		AgentID:  in.AgentID,
		TenantID: in.TenantID,
		At:       in.At,
	}
}

func publicFromStore(row store.ContactRow) PublicContact {
	if row.ID == "" {
		return PublicContact{}
	}
	ids := make([]PublicIdent, 0, len(row.Identifiers))
	for _, id := range row.Identifiers {
		ids = append(ids, PublicIdent{
			Channel:    id.Channel,
			Dest:       id.Dest,
			Kind:       id.Kind,
			Permission: id.Permission,
		})
	}
	from := append([]string(nil), row.MergedFrom...)
	if from == nil {
		from = []string{}
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
		CanUndo:     row.CanUndo,
		MergedFrom:  from,
	}
}

func mapContactErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return ErrContactNotFound
	case errors.Is(err, store.ErrContactConfirmRequired):
		return ErrContactConfirmRequired
	case errors.Is(err, store.ErrContactConfirm):
		return ErrContactConfirm
	case errors.Is(err, store.ErrContactSelfMerge):
		return ErrContactSelfMerge
	case errors.Is(err, store.ErrContactMerged):
		return ErrContactMerged
	case errors.Is(err, store.ErrContactNoUndo):
		return ErrContactNoUndo
	default:
		return err
	}
}
