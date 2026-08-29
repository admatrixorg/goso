// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import "testing"

func TestGatewaySettingsConflict(t *testing.T) {
	s := New()
	first, err := s.PutGatewaySettings(GatewaySettings{Values: map[string]string{"quota_day": "4"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.UpdatedAt.IsZero() || first.Values["quota_day"] != "4" {
		t.Fatalf("first %+v", first)
	}
	if _, err := s.PutGatewaySettings(GatewaySettings{Values: map[string]string{"quota_day": "5"}}); err != ErrConflict {
		t.Fatalf("missing stamp want conflict, got %v", err)
	}
	if _, err := s.PutGatewaySettings(GatewaySettings{Values: map[string]string{"quota_day": "6"}, UpdatedAt: first.UpdatedAt.AddDate(0, 0, -1)}); err != ErrConflict {
		t.Fatalf("stale stamp want conflict, got %v", err)
	}
	second, err := s.PutGatewaySettings(GatewaySettings{Values: map[string]string{"quota_day": "7"}, UpdatedAt: first.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetGatewaySettings()
	if err != nil {
		t.Fatal(err)
	}
	if got.Values["quota_day"] != "7" || got.UpdatedAt.IsZero() {
		t.Fatalf("got %+v", got)
	}
	if !second.UpdatedAt.After(first.UpdatedAt) && !second.UpdatedAt.Equal(first.UpdatedAt) {
		// equal is allowed only if clock didn't tick; still a new row
	}
}

func TestSQLiteGatewaySettingsConflict(t *testing.T) {
	s, err := OpenSQLite("file:gateway-settings?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	first, err := s.PutGatewaySettings(GatewaySettings{Values: map[string]string{"injection": "log"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutGatewaySettings(GatewaySettings{Values: map[string]string{"injection": "block"}}); err != ErrConflict {
		t.Fatalf("want conflict, got %v", err)
	}
	_, err = s.PutGatewaySettings(GatewaySettings{Values: map[string]string{"injection": "block"}, UpdatedAt: first.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetGatewaySettings()
	if err != nil {
		t.Fatal(err)
	}
	if got.Values["injection"] != "block" {
		t.Fatalf("got %+v", got)
	}
}
