// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import (
	"net/http"
	"testing"
)

func TestCheckURL_DefaultOff(t *testing.T) {
	t.Setenv("GOSO_SSRF", "")
	if err := CheckURL("http://127.0.0.1:9/tools/x"); err != nil {
		t.Fatalf("default off: %v", err)
	}
}

func TestCheckURL_SSRFOn(t *testing.T) {
	t.Setenv("GOSO_SSRF", "1")
	blocked := []string{
		"http://127.0.0.1/manifest",
		"http://localhost:8080/healthz",
		"http://10.0.0.5/x",
		"http://192.168.1.1/x",
		"http://172.16.0.1/x",
		"http://[::1]/x",
	}
	for _, u := range blocked {
		if err := CheckURL(u); err == nil {
			t.Fatalf("expected block %s", u)
		}
	}
	if err := CheckURL("https://example.com/manifest"); err != nil {
		t.Fatalf("public host: %v", err)
	}
}

func TestGuardClient_Redirect(t *testing.T) {
	t.Setenv("GOSO_SSRF", "1")
	c := &http.Client{}
	GuardClient(c)
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.CheckRedirect(req, nil); err == nil {
		t.Fatal("expected redirect block")
	}
	t.Setenv("GOSO_SSRF", "")
	c2 := &http.Client{}
	GuardClient(c2)
	if err := c2.CheckRedirect(req, nil); err != nil {
		t.Fatalf("default off: %v", err)
	}
}
