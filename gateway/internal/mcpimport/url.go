// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package mcpimport

import "net/url"

type cleanedURL struct {
	endpoint string
	needs    bool
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
	u.RawQuery = ""
	u.Fragment = ""
	u.ForceQuery = false
	return cleanedURL{endpoint: u.String(), needs: needs}, nil
}

type badURL struct{}

func (badURL) Error() string { return "invalid-endpoint" }

var errBadURL = badURL{}
