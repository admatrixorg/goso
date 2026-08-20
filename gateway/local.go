// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package gateway

import (
	"net/http"

	"github.com/mqglobal/goso/gateway/internal/serve"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// LocalStatus describes an assembled local handler (no secrets).
type LocalStatus struct {
	Provider  string
	HasReal   bool
	Auth      bool
	RateLimit int
}

// OpenLocal opens the gateway store at path ("" / ":memory:" = in-memory) and
// returns the same HTTP stack as `goso-gateway`. Domain types stay in internal/.
func OpenLocal(path, version string) (http.Handler, func() error, LocalStatus, error) {
	st, closeFn, err := store.Open(path)
	if err != nil {
		return nil, nil, LocalStatus{}, err
	}
	h, status := serve.New(st, version)
	return h, closeFn, LocalStatus{
		Provider:  status.Provider,
		HasReal:   status.HasReal,
		Auth:      status.Auth,
		RateLimit: status.RateLimit,
	}, nil
}
