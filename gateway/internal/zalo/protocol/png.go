package protocol

import (
	"encoding/base64"
	"strings"
)

// PNGBase64 returns the raw base64 payload the dialog prefixes itself.
func PNGBase64(png []byte) string {
	if len(png) == 0 {
		return ""
	}
	return strings.TrimPrefix(base64.StdEncoding.EncodeToString(png), "")
}
