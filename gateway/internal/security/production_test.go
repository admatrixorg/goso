// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import "testing"

func TestCheckProduction_RefuseVsDemoAllow(t *testing.T) {
	t.Setenv("GOSO_ENV", "production")
	t.Setenv("GOSO_WS_ORIGINS", "")
	if err := CheckProduction(); err == nil {
		t.Fatal("production empty origins should refuse")
	}
	t.Setenv("GOSO_WS_ORIGINS", "https://app.example")
	if err := CheckProduction(); err != nil {
		t.Fatalf("production with origins: %v", err)
	}
	t.Setenv("GOSO_ENV", "PRODUCTION")
	t.Setenv("GOSO_WS_ORIGINS", "")
	if err := CheckProduction(); err == nil {
		t.Fatal("case-insensitive production should refuse")
	}
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_WS_ORIGINS", "")
	if err := CheckProduction(); err != nil {
		t.Fatalf("demo allow: %v", err)
	}
	t.Setenv("GOSO_ENV", "")
	if err := CheckProduction(); err != nil {
		t.Fatalf("unset env allow: %v", err)
	}
}

func TestProduction(t *testing.T) {
	t.Setenv("GOSO_ENV", "production")
	if !Production() {
		t.Fatal("production")
	}
	t.Setenv("GOSO_ENV", "demo")
	if Production() {
		t.Fatal("demo is not production")
	}
}
