// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package workstation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCreateListGetUpdate(t *testing.T) {
	w := New()
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	w.now = func() time.Time { return base }

	_, err := w.Create(Input{})
	if !errors.Is(err, ErrDisplayRequired) {
		t.Fatalf("empty display %v", err)
	}
	_, err = w.Create(Input{Display: "build-1", Backend: "ssh", Host: "10.0.0.8"})
	if !errors.Is(err, ErrUserRequired) {
		t.Fatalf("ssh user %v", err)
	}
	_, err = w.Create(Input{Display: "build-1", Backend: "ftp", Host: "10.0.0.8", User: "ops"})
	if !errors.Is(err, ErrBackend) {
		t.Fatalf("backend %v", err)
	}
	_, err = w.Create(Input{Display: "build-1", Backend: "ssh", Host: "ops@10.0.0.8", User: "ops"})
	if !errors.Is(err, ErrHost) {
		t.Fatalf("host user@ %v", err)
	}

	row, err := w.Create(Input{
		Display:     "build-1",
		Backend:     "ssh",
		Host:        "10.0.0.8",
		User:        "ops",
		IdentityRef: "~/.ssh/id_ed25519",
		AgentID:     "ag_1",
		At:          base,
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.ID == "" || row.Backend != backendSSH || row.Port != defaultSSHPort || row.Health != healthUnknown {
		t.Fatalf("created %#v", row)
	}
	if !row.IdentitySet || row.IdentityRef != "~/.ssh/id_ed25519" {
		t.Fatalf("identity %#v", row)
	}

	listed := w.List("")
	if len(listed) != 1 || listed[0].ID != row.ID {
		t.Fatalf("list %#v", listed)
	}
	got, err := w.Get(row.ID, "")
	if err != nil || got.Display != "build-1" || got.AgentID != "ag_1" {
		t.Fatalf("get %#v %v", got, err)
	}

	updated, err := w.Update(row.ID, "", Input{Display: "build-east", Port: 2222}, false, false, base.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Display != "build-east" || updated.Port != 2222 || updated.IdentityRef != "~/.ssh/id_ed25519" {
		t.Fatalf("update %#v", updated)
	}

	cleared, err := w.Update(row.ID, "", Input{IdentityRef: ""}, true, false, base.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if cleared.IdentitySet || cleared.IdentityRef != "" {
		t.Fatalf("cleared identity %#v", cleared)
	}
}

func TestRejectKeyMaterialAndDockerDefaults(t *testing.T) {
	w := New()
	_, err := w.Create(Input{
		Display:     "bad",
		Backend:     "ssh",
		Host:        "10.0.0.8",
		User:        "ops",
		IdentityRef: "-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----",
	})
	if !errors.Is(err, ErrKeyMaterial) && !errors.Is(err, ErrIdentity) {
		t.Fatalf("pem %v", err)
	}
	_, err = w.Create(Input{Display: "bad", Backend: "ssh", Host: "10.0.0.8", User: "ops", IdentityRef: "PRIVATE KEY"})
	if !errors.Is(err, ErrKeyMaterial) && !errors.Is(err, ErrIdentity) {
		t.Fatalf("phrase %v", err)
	}

	row, err := w.Create(Input{Display: "dind", Backend: "docker", Host: "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if row.Port != defaultDockPort || row.User != "" || row.Backend != backendDocker {
		t.Fatalf("docker %#v", row)
	}
}

func TestTestNeverDialsAndConstrainsOutput(t *testing.T) {
	w := New()
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	row, err := w.Create(Input{Display: "lab", Backend: "ssh", Host: "192.0.2.10", User: "ops", IdentityRef: "/etc/goso/ssh/ops", At: base})
	if err != nil {
		t.Fatal(err)
	}
	tr, pub, err := w.Test(row.ID, "", base.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !tr.OK || tr.Health != healthOK || tr.Summary != "ssh config valid" {
		t.Fatalf("test %#v", tr)
	}
	if tr.Host != "192.0.2.10" || tr.Port != 22 || !tr.IdentitySet {
		t.Fatalf("test fields %#v", tr)
	}
	b, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, leak := range []string{`"identity_ref"`, `"private_key"`, `"password"`, `"secret"`, `"token"`, "BEGIN", "/etc/goso/ssh/ops"} {
		if strings.Contains(body, leak) {
			t.Fatalf("test leaked %s in %s", leak, body)
		}
	}
	if pub.Health != healthOK || pub.LastTested == nil {
		t.Fatalf("public after test %#v", pub)
	}
}

func TestDisconnectDeleteConfirmAndTenant(t *testing.T) {
	w := New()
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	row, err := w.Create(Input{Display: "lab", Backend: "ssh", Host: "10.0.0.8", User: "ops", TenantID: "acme", At: base})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Disconnect(row.ID, "acme", "", base); !errors.Is(err, ErrConfirmRequired) {
		t.Fatalf("confirm required %v", err)
	}
	if _, err := w.Disconnect(row.ID, "acme", "nope", base); !errors.Is(err, ErrConfirm) {
		t.Fatalf("mismatch %v", err)
	}
	if _, err := w.Get(row.ID, "other"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross tenant %v", err)
	}
	if len(w.List("other")) != 0 {
		t.Fatal("other tenant list")
	}

	off, err := w.Disconnect(row.ID, "acme", "lab", base.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if off.Health != healthOffline {
		t.Fatalf("disconnected %#v", off)
	}
	if _, err := w.Disconnect(row.ID, "acme", row.ID, base.Add(2*time.Minute)); !errors.Is(err, ErrNotDisconnected) {
		t.Fatalf("already disconnected %v", err)
	}

	if _, err := w.Delete(row.ID, "acme", "nope"); !errors.Is(err, ErrConfirm) {
		t.Fatalf("delete mismatch %v", err)
	}
	gone, err := w.Delete(row.ID, "acme", row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gone.ID != row.ID {
		t.Fatalf("deleted %#v", gone)
	}
	if _, err := w.Get(row.ID, "acme"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete %v", err)
	}
}

func TestPublicJSONOmitsKeys(t *testing.T) {
	w := New()
	row, err := w.Create(Input{Display: "desk", Backend: "ssh", Host: "10.0.0.8", User: "ops", IdentityRef: "~/.ssh/id_ed25519"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, leak := range []string{`"private_key"`, `"password"`, `"secret"`, `"token"`, `"key"`, `"pem"`} {
		if strings.Contains(body, leak) {
			t.Fatalf("leaked %s in %s", leak, body)
		}
	}
	if !strings.Contains(body, `"identity_ref"`) || !strings.Contains(body, `"identity_set":true`) {
		t.Fatalf("missing identity_ref %s", body)
	}
}

func TestLooksLikeKey(t *testing.T) {
	if LooksLikeKey("~/.ssh/id_ed25519") || LooksLikeKey("/home/ops/.ssh/ops") || LooksLikeKey("ssh:ops-laptop") {
		t.Fatal("path flagged as key")
	}
	if !LooksLikeKey("-----BEGIN OPENSSH PRIVATE KEY-----") || !LooksLikeKey("BEGIN RSA PRIVATE KEY") {
		t.Fatal("pem not flagged")
	}
}

func TestCap(t *testing.T) {
	w := New()
	for i := 0; i < maxRows; i++ {
		if _, err := w.Create(Input{Display: "d", Backend: "docker", Host: "10.0.0.8"}); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if _, err := w.Create(Input{Display: "overflow", Backend: "docker", Host: "10.0.0.8"}); !errors.Is(err, ErrCap) {
		t.Fatalf("cap %v", err)
	}
}
