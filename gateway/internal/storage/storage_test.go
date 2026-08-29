// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupWS(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", dir)
	t.Setenv("GOSO_STORAGE_MAX_BYTES", "1048576")
	return dir
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestList_NotConfigured(t *testing.T) {
	t.Setenv("GOSO_WORKSPACE", "")
	_, err := List("", false)
	if err != ErrNotConfigured {
		t.Fatalf("err %v", err)
	}
}

func TestList_MetadataHidesSecrets(t *testing.T) {
	dir := setupWS(t)
	writeFile(t, dir, "readme.txt", "hello")
	writeFile(t, dir, ".env", "SECRET=super-secret-value")
	writeFile(t, dir, "id_rsa", "-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n")
	if err := os.Mkdir(filepath.Join(dir, "secrets"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "secrets/token.txt", "sk-live-abcdefghijk")
	if err := os.Mkdir(filepath.Join(dir, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}

	lst, err := List("", false)
	if err != nil {
		t.Fatal(err)
	}
	if !lst.Configured {
		t.Fatal("configured")
	}
	names := map[string]bool{}
	for _, e := range lst.Entries {
		names[e.Name] = true
		if e.Name == "readme.txt" && e.Dir {
			t.Fatal("readme should be file")
		}
		if strings.Contains(e.Name, "sk-") || strings.Contains(e.Path, "SECRET") {
			t.Fatalf("secret leaked in listing %+v", e)
		}
	}
	if !names["readme.txt"] || !names["notes"] {
		t.Fatalf("visible %v", names)
	}
	if names[".env"] || names["id_rsa"] || names["secrets"] {
		t.Fatalf("hidden leaked %v skipped=%d", names, lst.HiddenSkipped)
	}
	if lst.HiddenSkipped < 3 {
		t.Fatalf("skipped %d", lst.HiddenSkipped)
	}
	if lst.UsedBytes <= 0 {
		t.Fatalf("used %d", lst.UsedBytes)
	}
	if len(lst.Breadcrumbs) != 1 || lst.Breadcrumbs[0].Path != "" {
		t.Fatalf("crumbs %+v", lst.Breadcrumbs)
	}
}

func TestJail_PathEscape(t *testing.T) {
	dir := setupWS(t)
	writeFile(t, dir, "ok.txt", "x")
	outside := filepath.Dir(dir)
	if _, _, err := Jail("../etc/passwd"); err != ErrPathEscape {
		t.Fatalf("dotdot %v", err)
	}
	if _, _, err := Jail(outside); err != ErrPathEscape {
		t.Fatalf("abs outside %v", err)
	}
	abs, rel, err := Jail("ok.txt")
	root, rerr := workspaceAbs()
	if err != nil || rerr != nil || rel != "ok.txt" || !strings.HasPrefix(abs, root) {
		t.Fatalf("jail abs=%s rel=%s root=%s err=%v rerr=%v", abs, rel, root, err, rerr)
	}
}

func TestJail_SymlinkEscape(t *testing.T) {
	dir := setupWS(t)
	outside := t.TempDir()
	writeFile(t, outside, "secret.txt", "nope")
	if err := os.Symlink(outside, filepath.Join(dir, "out")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Jail("out/secret.txt"); err != ErrPathEscape {
		t.Fatalf("symlink %v", err)
	}
}

func TestPreview_BoundedNoSecrets(t *testing.T) {
	dir := setupWS(t)
	writeFile(t, dir, "note.txt", "plain note")
	p, err := PreviewFile("note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "text" || p.Text != "plain note" {
		t.Fatalf("preview %+v", p)
	}
	writeFile(t, dir, "big.txt", strings.Repeat("a", PreviewBytes+80))
	p, err = PreviewFile("big.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Truncated || len(p.Text) > PreviewBytes {
		t.Fatalf("cap %+v len=%d", p, len(p.Text))
	}
	writeFile(t, dir, "leaky.txt", "token sk-live-abcdefghijk")
	p, err = PreviewFile("leaky.txt")
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "denied" || p.Text != "" {
		t.Fatalf("secret preview %+v", p)
	}
	f, _, _, _, err := OpenFile("leaky.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := GuardContent(f); err != ErrSecret {
		t.Fatalf("guard %v", err)
	}
	if _, err := PreviewFile(".env"); err != ErrSecret && err != ErrHidden {
		t.Fatalf(".env %v", err)
	}
}

func TestUpload_TypeSizeQuota(t *testing.T) {
	dir := setupWS(t)
	ent, err := Upload("", "hello.txt", strings.NewReader("hi"), 2)
	if err != nil {
		t.Fatal(err)
	}
	if ent.Name != "hello.txt" || ent.Size != 2 {
		t.Fatalf("ent %+v", ent)
	}
	if _, err := os.Stat(filepath.Join(dir, "hello.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := Upload("", "evil.sh", strings.NewReader("echo"), 4); err != ErrType {
		t.Fatalf("sh %v", err)
	}
	if _, err := Upload("", ".env", strings.NewReader("x=1"), 3); err != ErrSecret && err != ErrName {
		t.Fatalf("env %v", err)
	}
	if _, err := Upload("", "key.pem", strings.NewReader("x"), 1); err != ErrSecret && err != ErrType {
		t.Fatalf("pem %v", err)
	}
	big := bytes.Repeat([]byte("a"), MaxFileBytes+1)
	if _, err := Upload("", "too.txt", bytes.NewReader(big), int64(len(big))); err != ErrTooLarge {
		t.Fatalf("size %v", err)
	}
	if _, err := Upload("", "secret.txt", strings.NewReader("-----BEGIN OPENSSH PRIVATE KEY-----\n"), 40); err != ErrSecret {
		t.Fatalf("body secret %v", err)
	}
	t.Setenv("GOSO_STORAGE_MAX_BYTES", "8")
	if _, err := Upload("", "more.txt", strings.NewReader("123456789"), 9); err != ErrQuota {
		t.Fatalf("quota %v", err)
	}
}

func TestDelete_Confirm(t *testing.T) {
	dir := setupWS(t)
	writeFile(t, dir, "gone.txt", "x")
	if _, err := Delete("gone.txt", ""); err != ErrConfirmRequired {
		t.Fatalf("empty %v", err)
	}
	if _, err := Delete("gone.txt", "nope"); err != ErrConfirm {
		t.Fatalf("mismatch %v", err)
	}
	ent, err := Delete("gone.txt", "gone.txt")
	if err != nil {
		t.Fatal(err)
	}
	if ent.Name != "gone.txt" {
		t.Fatalf("ent %+v", ent)
	}
	if _, err := os.Stat(filepath.Join(dir, "gone.txt")); !os.IsNotExist(err) {
		t.Fatalf("still there %v", err)
	}
	if _, err := Delete("", "workspace"); err != ErrRoot {
		t.Fatalf("root %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "box"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "box/a.txt", "a")
	if _, err := Delete("box", "box"); err != ErrNotEmpty {
		t.Fatalf("not empty %v", err)
	}
}

func TestSecretName(t *testing.T) {
	if !SecretName(".env") || !SecretName("id_rsa") || !SecretName("foo.pem") || !SecretName("runtime") {
		t.Fatal("secret names")
	}
	if SecretName("readme.txt") || HiddenName("readme.txt") {
		t.Fatal("visible")
	}
	if !HiddenName(".hidden") {
		t.Fatal("dot")
	}
	if !AllowedExt("a.txt") || AllowedExt("a.exe") {
		t.Fatal("ext")
	}
}
