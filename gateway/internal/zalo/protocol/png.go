package protocol

import "encoding/base64"

// PNGDataURI wraps the QR bytes in the data URI the dialog renders.
func PNGDataURI(png []byte) string {
	if len(png) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}
