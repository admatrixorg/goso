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

// lookupIP is net.LookupIP; tests replace it so hostname checks stay offline.
var lookupIP = net.LookupIP

var metadataIPv4 = net.IPv4(169, 254, 169, 254)

// SSRFEnabled is GOSO_SSRF=1 (default off so local fake e2e still works).
func SSRFEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_SSRF")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// CheckURL blocks localhost, private, link-local, unspecified, multicast,
// IPv6 unique-local, and the cloud metadata address when SSRF is enabled.
// Hostnames are resolved; any blocked address or a failed lookup denies.
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
