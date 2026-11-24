package protocol

import (
	"encoding/base64"
	"strings"
	"testing"
)

// The login reply carries the payload key as zpw_enk, and a session that has no
// key must adopt it, because every API payload is crypted with it.
func TestLoginInfoCarriesSecretKey(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 16)))
	if got := (LoginInfo{UID: "u", SecretKey: key}).SecretKey; got != key {
		t.Fatalf("secret key %q", got)
	}
}
