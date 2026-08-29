// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

const gatewaySettingsID = "default"

func (s *SQLiteStore) GetGatewaySettings() (*GatewaySettings, error) {
	var raw, stamp string
	err := s.db.QueryRow(`SELECT values_json, updated_at FROM gateway_settings WHERE id=?`, gatewaySettingsID).Scan(&raw, &stamp)
	if err == sql.ErrNoRows {
		return &GatewaySettings{Values: map[string]string{}}, nil
	}
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, err
		}
	}
	t, _ := time.Parse(time.RFC3339Nano, stamp)
	if t.IsZero() {
		t, _ = time.Parse(time.RFC3339, stamp)
	}
	return &GatewaySettings{Values: cloneSettingsValues(values), UpdatedAt: t.UTC()}, nil
}

func (s *SQLiteStore) PutGatewaySettings(in GatewaySettings) (*GatewaySettings, error) {
	cur, err := s.GetGatewaySettings()
	if err != nil {
		return nil, err
	}
	if !cur.UpdatedAt.IsZero() {
		if in.UpdatedAt.IsZero() || !stampsMatch(cur.UpdatedAt, in.UpdatedAt) {
			return nil, ErrConflict
		}
	}
	now := time.Now().UTC()
	values := cloneSettingsValues(in.Values)
	body, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(
		`INSERT INTO gateway_settings(id, values_json, updated_at) VALUES(?,?,?)
		 ON CONFLICT(id) DO UPDATE SET values_json=excluded.values_json, updated_at=excluded.updated_at`,
		gatewaySettingsID, string(body), now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	return &GatewaySettings{Values: values, UpdatedAt: now}, nil
}
