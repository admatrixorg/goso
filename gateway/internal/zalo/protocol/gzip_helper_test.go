package protocol

import (
	"bytes"
	"compress/gzip"
)

// gzipForTest compresses a payload for the listener decode tests.
func gzipForTest(b []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(b)
	_ = w.Close()
	return buf.Bytes()
}

