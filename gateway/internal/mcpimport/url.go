// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package mcpimport

import "net/url"

type cleanedURL struct {
	endpoint string
	needs    bool
	hold     bool
}

func urlParse(raw string) (cleanedURL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		// url.Parse errors can quote the input. Callers only see errBadURL.
		return cleanedURL{}, errBadURL
	}
	needs := false
	if u.User != nil {
		needs = true
		u.User = nil
	}
	if u.RawQuery != "" || u.Fragment != "" {
		needs = true
	}
	if pathHasKey(u.Path) || pathHasKey(u.RawPath) || pathHasKey(raw) {
		// Drop the URL. /s/<segment>/ and any key-like segment are not stored.
		return cleanedURL{needs: true, hold: true}, nil
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.ForceQuery = false
	return cleanedURL{endpoint: u.String(), needs: needs}, nil
}

type badURL struct{}

func (badURL) Error() string { return "invalid-endpoint" }

var errBadURL = badURL{}
