// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func exerciseToolSettings(t *testing.T, ts ToolSettingsStore) {
	t.Helper()
	if _, err := ts.GetToolSettings("shop-a", "read_image"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty get %v", err)
	}
	a := `{"providers":[{"provider":"gem-a","model":"gemini-2.5-flash"}]}`
	row, err := ts.PutToolSettings(ToolSettings{TenantID: "shop-a", Tool: "read_image", Settings: json.RawMessage(a)})
	if err != nil || row.TenantID != "shop-a" || row.UpdatedAt.IsZero() {
		t.Fatalf("put %v %+v", err, row)
	}
	if _, err := ts.PutToolSettings(ToolSettings{TenantID: "shop-b", Tool: "read_image", Settings: json.RawMessage(`{"providers":[]}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.PutToolSettings(ToolSettings{TenantID: "shop-a", Tool: "create_image", Settings: json.RawMessage(`{"providers":[]}`)}); err != nil {
		t.Fatal(err)
	}
	got, err := ts.GetToolSettings("shop-a", "read_image")
	if err != nil || !strings.Contains(string(got.Settings), "gem-a") {
		t.Fatalf("get %v %+v", err, got)
	}
	// Upsert replaces.
	if _, err := ts.PutToolSettings(ToolSettings{TenantID: "shop-a", Tool: "read_image", Settings: json.RawMessage(`{"providers":[{"provider":"gem-c"}]}`)}); err != nil {
		t.Fatal(err)
	}
	got, _ = ts.GetToolSettings("shop-a", "read_image")
	if !strings.Contains(string(got.Settings), "gem-c") {
		t.Fatalf("upsert %s", got.Settings)
	}
	// Tenant isolation.
	other, _ := ts.GetToolSettings("shop-b", "read_image")
	if other == nil || strings.Contains(string(other.Settings), "gem") {
		t.Fatalf("shop-b sees %+v", other)
	}
	list, err := ts.ListToolSettings("shop-a")
	if err != nil || len(list) != 2 || list[0].Tool != "create_image" || list[1].Tool != "read_image" {
		t.Fatalf("list %v %+v", err, list)
	}
	if list, _ := ts.ListToolSettings("shop-c"); len(list) != 0 {
		t.Fatalf("shop-c list %+v", list)
	}
	for _, bad := range []string{``, `[]`, `"x"`, `{bad`, `{"x":"` + strings.Repeat("a", MaxToolSettingsBytes) + `"}`} {
		if _, err := ts.PutToolSettings(ToolSettings{TenantID: "shop-a", Tool: "read_image", Settings: json.RawMessage(bad)}); err == nil {
			t.Fatalf("accepted %.20q", bad)
		}
	}
	if _, err := ts.PutToolSettings(ToolSettings{TenantID: "shop-a", Tool: " ", Settings: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("empty tool accepted")
	}
	if err := ts.DeleteToolSettings("shop-b", "create_image"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant delete %v", err)
	}
	if err := ts.DeleteToolSettings("shop-a", "read_image"); err != nil {
		t.Fatal(err)
	}
	if _, err := ts.GetToolSettings("shop-a", "read_image"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete %v", err)
	}
	if _, err := ts.GetToolSettings("shop-b", "read_image"); err != nil {
		t.Fatalf("shop-b row removed: %v", err)
	}
}

func TestToolSettings_Memory(t *testing.T) {
	exerciseToolSettings(t, New())
}

func TestToolSettings_SQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ts.db")
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	exerciseToolSettings(t, st)
	if _, err := st.PutToolSettings(ToolSettings{TenantID: "shop-a", Tool: "read_video", Settings: json.RawMessage(`{"providers":[{"provider":"gem-a"}]}`)}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	again, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	row, err := again.GetToolSettings("shop-a", "read_video")
	if err != nil || !strings.Contains(string(row.Settings), "gem-a") {
		t.Fatalf("reopen %v %+v", err, row)
	}
}

func TestToolSettings_Postgres(t *testing.T) {
	dsn := postgresTestDSN()
	if dsn == "" {
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	t.Setenv("GOSO_DATABASE_URL", "")
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, tn := range []string{"shop-a", "shop-b"} {
		for _, tool := range []string{"read_image", "create_image"} {
			_ = st.DeleteToolSettings(tn, tool)
		}
	}
	exerciseToolSettings(t, st)
}
