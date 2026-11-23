// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"strings"
	"time"
)

// PairedDevice is one device or channel identity that paired with the gateway.
type PairedDevice struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenant_id"`
	SenderID  string `json:"sender_id"`
	Channel   string `json:"channel,omitempty"`
	ChatID    string `json:"chat_id,omitempty"`
	PairedBy  string `json:"paired_by,omitempty"`
	Metadata  string `json:"metadata,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	PairedAt  string `json:"paired_at"`
}

// PairedDeviceStore is the device registry capability.
type PairedDeviceStore interface {
	InsertPairedDevice(PairedDevice) (*PairedDevice, error)
	ListPairedDevices(tenant string) []*PairedDevice
	DeletePairedDevice(tenant, id string) error
}

// PairedDevices returns the device registry of st, or nil.
func PairedDevices(st StoreIface) PairedDeviceStore {
	if x, ok := st.(PairedDeviceStore); ok {
		return x
	}
	return nil
}

func (s *SQLiteStore) migratePairedDevices() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS paired_devices (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			sender_id TEXT NOT NULL,
			channel TEXT NOT NULL DEFAULT '',
			chat_id TEXT NOT NULL DEFAULT '',
			paired_by TEXT NOT NULL DEFAULT 'operator',
			metadata TEXT NOT NULL DEFAULT '',
			expires_at TEXT NOT NULL DEFAULT '',
			paired_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_paired_device ON paired_devices(tenant_id, sender_id, channel)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// InsertPairedDevice records a pairing. A repeat pairing from the same sender
// and channel refreshes the existing row instead of adding a second one.
func (s *SQLiteStore) InsertPairedDevice(d PairedDevice) (*PairedDevice, error) {
	d.TenantID = NormalizeTenant(d.TenantID)
	d.SenderID = strings.TrimSpace(d.SenderID)
	if d.SenderID == "" {
		return nil, ErrNotFound
	}
	if d.PairedBy == "" {
		d.PairedBy = "operator"
	}
	if d.PairedAt == "" {
		d.PairedAt = formatTime(time.Now().UTC())
	}
	if d.ID == "" {
		d.ID = "pd_" + newID()
	}
	_, err := s.db.Exec(`INSERT INTO paired_devices(id, tenant_id, sender_id, channel, chat_id, paired_by, metadata, expires_at, paired_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(tenant_id, sender_id, channel) DO UPDATE SET chat_id=excluded.chat_id, paired_by=excluded.paired_by, metadata=excluded.metadata, expires_at=excluded.expires_at, paired_at=excluded.paired_at`,
		d.ID, d.TenantID, d.SenderID, d.Channel, d.ChatID, d.PairedBy, d.Metadata, d.ExpiresAt, d.PairedAt)
	if err != nil {
		return nil, err
	}
	var out PairedDevice
	q := `SELECT id, tenant_id, sender_id, channel, chat_id, paired_by, metadata, expires_at, paired_at FROM paired_devices WHERE tenant_id=? AND sender_id=? AND channel=?`
	if serr := s.db.QueryRow(q, d.TenantID, d.SenderID, d.Channel).Scan(&out.ID, &out.TenantID, &out.SenderID, &out.Channel, &out.ChatID, &out.PairedBy, &out.Metadata, &out.ExpiresAt, &out.PairedAt); serr != nil {
		return &d, nil
	}
	return &out, nil
}

func (s *SQLiteStore) ListPairedDevices(tenant string) []*PairedDevice {
	rows, err := s.db.Query(`SELECT id, tenant_id, sender_id, channel, chat_id, paired_by, metadata, expires_at, paired_at FROM paired_devices WHERE tenant_id=? ORDER BY paired_at DESC`, NormalizeTenant(tenant))
	if err != nil {
		return []*PairedDevice{}
	}
	defer rows.Close()
	out := []*PairedDevice{}
	for rows.Next() {
		var d PairedDevice
		if rows.Scan(&d.ID, &d.TenantID, &d.SenderID, &d.Channel, &d.ChatID, &d.PairedBy, &d.Metadata, &d.ExpiresAt, &d.PairedAt) != nil {
			continue
		}
		out = append(out, &d)
	}
	return out
}

func (s *SQLiteStore) DeletePairedDevice(tenant, id string) error {
	res, err := s.db.Exec(`DELETE FROM paired_devices WHERE id=? AND tenant_id=?`, strings.TrimSpace(id), NormalizeTenant(tenant))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
