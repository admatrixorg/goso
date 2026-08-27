// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package secrets

import (
	"bytes"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestPutGet_RandomMasterKey(t *testing.T) {
	key, err := RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	st := store.New()
	plain := []byte("provider-key-blob-not-a-product-secret")
	if err := Put(st, "openai", plain); err != nil {
		t.Fatal(err)
	}
	got, err := Get(st, "openai")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q", got)
	}
	row, err := st.GetSecret("openai")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(row.CT, plain) {
		t.Fatal("plaintext leaked in ciphertext")
	}
}

func TestPut_EmptyMasterKeyRefuses(t *testing.T) {
	t.Setenv("GOSO_MASTER_KEY", "")
	st := store.New()
	if err := Put(st, "openai", []byte("x")); err != ErrNoMasterKey {
		t.Fatalf("want ErrNoMasterKey, got %v", err)
	}
	if _, err := st.GetSecret("openai"); err != store.ErrNotFound {
		t.Fatalf("must not store: %v", err)
	}
}

func TestEncrypt_BadKey(t *testing.T) {
	t.Setenv("GOSO_MASTER_KEY", "not-hex")
	if _, _, err := Encrypt([]byte("x")); err != ErrNoMasterKey {
		t.Fatalf("got %v", err)
	}
}

func TestSQLiteRoundTrip(t *testing.T) {
	key, err := RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	st, err := store.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	plain := []byte("compat-provider-blob")
	if err := Put(st, "groq", plain); err != nil {
		t.Fatal(err)
	}
	got, err := Get(st, "groq")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("got %q", got)
	}
}
