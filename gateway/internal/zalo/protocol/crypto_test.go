package protocol

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestEncodeDecodeAESCBCRoundtrip(t *testing.T) {
	key := []byte("0123456789ABCDEF")
	cases := []string{
		"hello",
		"0123456789ABCDEF",
		"this is a longer test string that spans multiple AES blocks",
		`{"imei":"abc","ts":12345}`,
		"",
	}
	for _, in := range cases {
		enc, err := EncodeAESCBC(key, in, false)
		if err != nil {
			t.Fatalf("encode %q: %v", in, err)
		}
		out, err := DecodeAESCBC(key, enc)
		if err != nil {
			t.Fatalf("decode %q: %v", in, err)
		}
		if string(out) != in {
			t.Fatalf("roundtrip %q -> %q", in, out)
		}
	}
}

func TestEncodeAESCBCIsDeterministic(t *testing.T) {
	key := []byte("0123456789ABCDEF")
	a, _ := EncodeAESCBC(key, "deterministic test", false)
	b, _ := EncodeAESCBC(key, "deterministic test", false)
	if a != b {
		t.Fatal("zero IV encryption must be deterministic")
	}
}

func TestEncodeAESCBCHexDiffersFromBase64(t *testing.T) {
	key := []byte("0123456789ABCDEF")
	hexEnc, _ := EncodeAESCBC(key, "test data", true)
	b64Enc, _ := EncodeAESCBC(key, "test data", false)
	if hexEnc == b64Enc {
		t.Fatal("hex and base64 should differ")
	}
	if b64Enc == "" || hexEnc == "" {
		t.Fatal("both encodings must be non empty")
	}
}

func TestDecodeAESCBCRejectsGarbage(t *testing.T) {
	key := []byte("0123456789ABCDEF")
	if _, err := DecodeAESCBC(key, "not-base64!!"); err == nil {
		t.Fatal("expected a base64 error")
	}
	bogus := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, err := DecodeAESCBC(key, bogus); err == nil {
		t.Fatal("expected a block size error")
	}
}

func TestAESGCMWith16ByteNonce(t *testing.T) {
	key := []byte("0123456789ABCDEF")
	iv := bytes.Repeat([]byte{7}, 16)
	aad := []byte("zalo")
	plain := []byte("message body")
	ct, err := EncodeAESGCM(key, iv, aad, plain)
	if err != nil {
		t.Fatalf("encode gcm: %v", err)
	}
	out, err := DecodeAESGCM(key, iv, aad, ct)
	if err != nil {
		t.Fatalf("decode gcm: %v", err)
	}
	if !bytes.Equal(out, plain) {
		t.Fatalf("gcm roundtrip %q", out)
	}
}

func TestDecodeSecretKeyAndResolve(t *testing.T) {
	raw := bytes.Repeat([]byte{3}, 16)
	secret := base64.StdEncoding.EncodeToString(raw)
	got, err := DecodeSecretKey(secret)
	if err != nil {
		t.Fatalf("decode secret: %v", err)
	}
	key, ok := ResolveKey(got)
	if !ok || len(key) != 16 {
		t.Fatalf("resolve key %d %v", len(key), ok)
	}
	if _, ok := ResolveKey([]byte("short")); ok {
		t.Fatal("short key must be rejected")
	}
}
