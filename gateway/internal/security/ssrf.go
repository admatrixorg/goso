// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/config"
)

// lookupIP is net.LookupIP; tests replace it so hostname checks stay offline.
var lookupIP = net.LookupIP

var metadataIPv4 = net.IPv4(169, 254, 169, 254)

// SSRFEnabled is GOSO_SSRF=1, or production when GOSO_SSRF is unset.
// Explicit 0/false/off keeps demo loopback (router9) working even if GOSO_ENV is production.
func SSRFEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(config.Lookup("GOSO_SSRF")))
	if v == "0" || v == "false" || v == "off" || v == "no" {
		return false
	}
	if v == "1" || v == "true" || v == "yes" || v == "on" {
		return true
	}
	return Production()
}

// CheckURL blocks localhost, private, link-local, unspecified, multicast,
// IPv6 unique-local, and the cloud metadata address when SSRF is enabled.
// Hostnames are resolved; any blocked address or a failed lookup denies.
//
// GOSO_SSRF_ALLOW_HOSTS is a comma-separated exact-host allowlist, optionally
// host:port (IPv6 as [::1] or [::1]:port). A listed host is allowed even when
// it resolves to a private address. Matching is the full hostname only:
// 9router does not match evil9router, 9router.internal, or not.9router.
// An entry without a port allows any port; host:port allows only that port.
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
	if hostAllowed(host, u.Port()) {
		return nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("ssrf: localhost blocked")
	}
	if ip := net.ParseIP(host); ip != nil {
		return denyIP(ip)
	}
	addrs, err := lookupIP(host)
	if err != nil || len(addrs) == 0 {
		return fmt.Errorf("ssrf: dns lookup failed")
	}
	for _, ip := range addrs {
		if err := denyIP(ip); err != nil {
			return err
		}
	}
	return nil
}

type allowHost struct {
	host string
	port string
}

func allowHosts() []allowHost {
	raw := config.Lookup("GOSO_SSRF_ALLOW_HOSTS")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []allowHost
	for _, part := range strings.Split(raw, ",") {
		if h, ok := parseAllowHost(part); ok {
			out = append(out, h)
		}
	}
	return out
}

func parseAllowHost(s string) (allowHost, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return allowHost{}, false
	}
	if strings.HasPrefix(s, "[") {
		host, port, err := net.SplitHostPort(s)
		if err != nil {
			host = strings.Trim(s, "[]")
			port = ""
		}
		host = strings.ToLower(strings.TrimSpace(host))
		if host == "" {
			return allowHost{}, false
		}
		return allowHost{host: host, port: port}, true
	}
	if host, port, ok := splitHostPort(s); ok {
		return allowHost{host: strings.ToLower(host), port: port}, true
	}
	return allowHost{host: strings.ToLower(s)}, true
}

func splitHostPort(s string) (string, string, bool) {
	i := strings.LastIndex(s, ":")
	if i <= 0 || i == len(s)-1 {
		return "", "", false
	}
	host, port := s[:i], s[i+1:]
	if strings.Contains(host, ":") || host == "" {
		return "", "", false
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return "", "", false
		}
	}
	return host, port, true
}

func hostAllowed(host, port string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, a := range allowHosts() {
		if a.host != host {
			continue
		}
		if a.port == "" || a.port == port {
			return true
		}
	}
	return false
}

func denyIP(ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("ssrf: private address blocked")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
		return fmt.Errorf("ssrf: private address blocked")
	}
	if ip4 := ip.To4(); ip4 != nil && ip4.Equal(metadataIPv4) {
		return fmt.Errorf("ssrf: private address blocked")
	}
	if ip.To4() == nil && len(ip) == net.IPv6len && ip[0]&0xfe == 0xfc {
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
