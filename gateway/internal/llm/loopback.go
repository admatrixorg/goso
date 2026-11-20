// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"net"
	"net/url"
	"os"
	"strings"
)

// rewriteLoopbackURL maps localhost/127.0.0.1/::1 onto GOSO_LOOPBACK_HOST.
// Compose sets that to host.docker.internal so a containerized gateway can
// reach a host-side OpenAI-compat server (9AI / 9Router on :20128). Empty env
// leaves the URL unchanged so host-run tests and httptest stay on loopback.
func rewriteLoopbackURL(raw string) string {
	target := strings.TrimSpace(os.Getenv("GOSO_LOOPBACK_HOST"))
	raw = strings.TrimSpace(raw)
	if target == "" || raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	switch strings.ToLower(u.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
	default:
		return raw
	}
	if port := u.Port(); port != "" {
		u.Host = net.JoinHostPort(target, port)
	} else {
		u.Host = target
	}
	return u.String()
}
