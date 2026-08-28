// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import (
	"fmt"
	"net"
	"net/http"
	"testing"
)

func TestCheckURL_DefaultOff(t *testing.T) {
	t.Setenv("GOSO_SSRF", "")
	t.Setenv("GOSO_ENV", "demo")
	if err := CheckURL("http://127.0.0.1:9/tools/x"); err != nil {
		t.Fatalf("default off: %v", err)
	}
	if err := CheckURL("http://127.0.0.1:20127/v1"); err != nil {
		t.Fatalf("demo router9: %v", err)
	}
}

func TestCheckURL_ProductionDefaultOn(t *testing.T) {
	t.Setenv("GOSO_ENV", "production")
	t.Setenv("GOSO_SSRF", "")
	if err := CheckURL("http://127.0.0.1:20127/v1"); err == nil {
		t.Fatal("production should DNS/IP-block loopback when GOSO_SSRF unset")
	}
	t.Setenv("GOSO_SSRF", "0")
	if err := CheckURL("http://127.0.0.1:20127/v1"); err != nil {
		t.Fatalf("explicit off: %v", err)
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
		"http://0.0.0.0/x",
		"http://224.0.0.1/x",
		"http://169.254.169.254/latest/meta-data",
		"http://[fd12:3456:789a::1]/x",
	}
	for _, u := range blocked {
		if err := CheckURL(u); err == nil {
			t.Fatalf("expected block %s", u)
		}
	}
}

func TestCheckURL_DNSLoopbackBlocked(t *testing.T) {
	t.Setenv("GOSO_SSRF", "1")
	orig := lookupIP
	t.Cleanup(func() { lookupIP = orig })
	lookupIP = func(host string) ([]net.IP, error) {
		switch host {
		case "evil.internal":
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		case "example.com":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		default:
			return nil, fmt.Errorf("no such host")
		}
	}
	if err := CheckURL("http://evil.internal/x"); err == nil {
		t.Fatal("hostname→127.0.0.1 should be blocked")
	}
	if err := CheckURL("https://example.com/manifest"); err != nil {
		t.Fatalf("public host: %v", err)
	}
	if err := CheckURL("http://unknown.example/x"); err == nil {
		t.Fatal("lookup failure should deny")
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
	t.Setenv("GOSO_ENV", "demo")
	c2 := &http.Client{}
	GuardClient(c2)
	if err := c2.CheckRedirect(req, nil); err != nil {
		t.Fatalf("default off: %v", err)
	}
}
