package protocol

import "testing"

// A QR login stores cookies, and the payload key travels in the zpw_enk cookie
// rather than its own field, so the session must read it from there.
func TestKeyFromCookies(t *testing.T) {
	key := "MDEyMzQ1Njc4OUFCQ0RFRg=="
	got := KeyFromCookies([]Cookie{
		{Name: "zpsid", Value: "sid"},
		{Name: "ZPW_ENK", Value: key},
	})
	if got != key {
		t.Fatalf("key %q, want %q", got, key)
	}
	if got := KeyFromCookies([]Cookie{{Name: "zpsid", Value: "sid"}}); got != "" {
		t.Fatalf("a cookie set without the key returned %q", got)
	}
}

func TestSessionReadsSecretKeyFromCookies(t *testing.T) {
	key := "MDEyMzQ1Njc4OUFCQ0RFRg=="
	sess, err := NewSessionWithCredential(&Credential{
		Cookies: []Cookie{{Name: "zpw_enk", Value: key}, {Name: "zpsid", Value: "sid"}},
		IMEI:    "imei-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sess.SecretKey != key {
		t.Fatalf("secret key %q, want the cookie value", sess.SecretKey)
	}
	if sess.IMEI != "imei-1" {
		t.Fatalf("imei %q", sess.IMEI)
	}
}

// A credential that carries the key as its own field keeps it.
func TestSessionKeepsDeclaredSecretKey(t *testing.T) {
	sess, err := NewSessionWithCredential(&Credential{
		Cookies:   []Cookie{{Name: "zpw_enk", Value: "from-cookie"}},
		SecretKey: "declared",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sess.SecretKey != "declared" {
		t.Fatalf("secret key %q, want the declared value", sess.SecretKey)
	}
}
