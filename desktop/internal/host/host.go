// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package host

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	goso "github.com/mqglobal/goso/gateway"
)

// Version is the desktop/gateway build identity shown in /healthz.
const Version = "0.1.0"

// Runtime is a local gateway bound to a SQLite (or memory) store.
type Runtime struct {
	DBPath  string
	Handler http.Handler
	Status  goso.LocalStatus
	close   func() error
	once    sync.Once
}

// DefaultDBPath returns the desktop SQLite path.
// GOSO_DB_PATH overrides. Otherwise:
//
//	macOS:   ~/Library/Application Support/GOSO/goso.db
//	Windows: %APPDATA%/GOSO/goso.db
//	other:   $XDG_DATA_HOME/GOSO/goso.db or ~/.local/share/GOSO/goso.db
func DefaultDBPath() string {
	if p := os.Getenv("GOSO_DB_PATH"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join("data", "goso.db")
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "GOSO", "goso.db")
	case "windows":
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "GOSO", "goso.db")
		}
		return filepath.Join(home, "AppData", "Roaming", "GOSO", "goso.db")
	default:
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "GOSO", "goso.db")
		}
		return filepath.Join(home, ".local", "share", "GOSO", "goso.db")
	}
}

// Start opens the gateway store (via gateway.OpenLocal — no domain duplicate) and assembles the HTTP handler.
func Start() (*Runtime, error) {
	path := DefaultDBPath()
	if path != "" && path != ":memory:" {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
	}
	h, closeFn, status, err := goso.OpenLocal(path, Version)
	if err != nil {
		return nil, err
	}
	return &Runtime{DBPath: path, Handler: h, Status: status, close: closeFn}, nil
}

// Close releases the store (idempotent).
func (r *Runtime) Close() error {
	if r == nil || r.close == nil {
		return nil
	}
	var err error
	r.once.Do(func() { err = r.close() })
	return err
}

// IsGatewayPath reports whether the request should hit the embedded gateway instead of UI assets.
func IsGatewayPath(p string) bool {
	return p == "/healthz" || p == "/ws" || strings.HasPrefix(p, "/api/")
}

// Middleware routes /healthz, /api/*, /ws to the gateway handler; everything else to next (Wails assets).
func Middleware(gw http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if IsGatewayPath(r.URL.Path) {
				gw.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
