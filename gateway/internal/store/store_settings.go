// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"strings"
	"time"
)

func cloneSettingsValues(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = strings.TrimSpace(v)
	}
	return out
}

func (s *Store) GetGatewaySettings() (*GatewaySettings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.gatewaySettings == nil {
		return &GatewaySettings{Values: map[string]string{}}, nil
	}
	return &GatewaySettings{
		Values:    cloneSettingsValues(s.gatewaySettings.Values),
		UpdatedAt: s.gatewaySettings.UpdatedAt,
	}, nil
}

func (s *Store) PutGatewaySettings(in GatewaySettings) (*GatewaySettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gatewaySettings != nil && !s.gatewaySettings.UpdatedAt.IsZero() {
		if in.UpdatedAt.IsZero() || !stampsMatch(s.gatewaySettings.UpdatedAt, in.UpdatedAt) {
			return nil, ErrConflict
		}
	}
	row := &GatewaySettings{
		Values:    cloneSettingsValues(in.Values),
		UpdatedAt: time.Now().UTC(),
	}
	s.gatewaySettings = row
	return &GatewaySettings{
		Values:    cloneSettingsValues(row.Values),
		UpdatedAt: row.UpdatedAt,
	}, nil
}
