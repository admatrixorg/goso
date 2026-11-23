package protocol

import "encoding/base64"

// base64Of is a tiny helper so tests can express a secret key inline.
func base64Of(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
