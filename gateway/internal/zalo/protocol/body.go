package protocol

import (
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"strings"
)

// ReadBody returns the response body, undoing the compression Zalo applies to
// most API replies. Without this step a gzipped reply looks like garbage and
// fails to parse.
func ReadBody(resp *http.Response, limit int64) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, io.EOF
	}
	var reader io.Reader = resp.Body
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "gzip":
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	case "deflate":
		zr, err := zlib.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		reader = zr
	}
	return io.ReadAll(io.LimitReader(reader, limit))
}
