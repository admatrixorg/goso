package protocol

import "testing"

// The login reply carries the payload key as zpw_enk, and a session that has no
// key must adopt it, because every API payload is crypted with it.
func TestLoginInfoCarriesSecretKey(t *testing.T) {
	if got := (LoginInfo{UID: "u", SecretKey: "k"}).SecretKey; got != "k" {
		t.Fatalf("secret key %q", got)
	}
}
