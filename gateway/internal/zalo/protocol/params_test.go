package protocol

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Every Zalo API call is signed with a per-request key derived from a fresh
// zcid. Without it the server answers with a session key error, so the round
// trip has to hold.
func TestEncryptParamRoundTrip(t *testing.T) {
	sess := &Session{IMEI: "imei-qc", Language: "vi", UserAgent: DefaultUserAgent}
	params, replyKey, err := sess.EncryptParam("getlogininfo")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"zcid", "enc_ver", "zcid_ext", "params", "type", "client_version", "signkey"} {
		if params[key] == "" {
			t.Fatalf("parameter %q is missing from the signed set", key)
		}
	}
	if params["enc_ver"] != DefaultEncryptVer {
		t.Fatalf("enc version %q", params["enc_ver"])
	}
	// The server can rebuild the request key from the zcid and its extension,
	// which is how it opens the payload.
	derived, err := DeriveEncryptKey(params["zcid_ext"], params["zcid"])
	if err != nil {
		t.Fatal(err)
	}
	if derived != replyKey {
		t.Fatalf("derived key %q does not match the reply key %q", derived, replyKey)
	}
	plain, err := DecodeAESCBC([]byte(derived), params["params"])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(plain, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["imei"] != "imei-qc" || payload["computer_name"] != DefaultComputerName {
		t.Fatalf("payload %v", payload)
	}
	if _, ok := payload["ts"]; !ok {
		t.Fatalf("payload has no timestamp: %v", payload)
	}
}

func TestDeriveEncryptKeyIsStableAndFullLength(t *testing.T) {
	first, err := DeriveEncryptKey("a1b2c3d4e5f6", "3F9A1C7B2D8E4A6B0C5D9E2F7A4B8C13")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := DeriveEncryptKey("a1b2c3d4e5f6", "3F9A1C7B2D8E4A6B0C5D9E2F7A4B8C13")
	if first != second || len(first) != 32 {
		t.Fatalf("key %q len %d", first, len(first))
	}
	// A zcid too short to split yields fewer characters than an AES key needs,
	// which the caller rejects when it builds the cipher.
	if short, _ := DeriveEncryptKey("ab", "XY"); len(short) >= 16 {
		t.Fatalf("a short zcid produced %d key characters", len(short))
	}
}

// Zalo compresses most replies, so the reader has to undo that before parsing.
func TestReadBodyUndoesGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte(`{"error_code":0}`))
		_ = gz.Close()
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := ReadBody(resp, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != `{"error_code":0}` {
		t.Fatalf("body %q", body)
	}
}

func TestReadBodyPassesPlainReplies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "plain")
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := ReadBody(resp, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "plain" {
		t.Fatalf("body %q", body)
	}
}

var _ = url.PathUnescape
