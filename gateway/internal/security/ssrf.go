// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// SSRFEnabled is GOSO_SSRF=1 (default off so local fake e2e still works).
func SSRFEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_SSRF")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// CheckURL blocks literal localhost and private IPs when SSRF is enabled.
func CheckURL(raw string) error {
	if !SSRFEnabled() {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("ssrf: invalid url")
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return fmt.Errorf("ssrf: invalid host")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("ssrf: localhost blocked")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
		return fmt.Errorf("ssrf: private address blocked")
	}
	return nil
}

// GuardClient re-checks redirect targets when SSRF is enabled.
func GuardClient(c *http.Client) {
	if c == nil {
		return
	}
	prev := c.CheckRedirect
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req != nil && req.URL != nil {
			if err := CheckURL(req.URL.String()); err != nil {
				return err
			}
		}
		if prev != nil {
			return prev(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
}
