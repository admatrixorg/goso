// Copyright (c) 2026 MQ Global — GOSO Desktop. Clean-room implementation.
//go:build wails

package main

import (
	"context"
	"net/http"
	"sync"

	"github.com/mqglobal/goso/desktop/internal/host"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// App is the Wails bind target. Control Plane talks to the embedded gateway over HTTP;
// these methods are optional helpers for the window (path/version).
type App struct {
	ctx context.Context
	mu  sync.Mutex
	rt  *host.Runtime
}

func NewApp() *App {
	return &App{}
}

func (a *App) ensure() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rt != nil {
		return nil
	}
	rt, err := host.Start()
	if err != nil {
		return err
	}
	a.rt = rt
	return nil
}

func (a *App) middleware() assetserver.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := a.ensure(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			host.Middleware(a.rt.Handler)(next).ServeHTTP(w, r)
		})
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	_ = a.ensure()
}

func (a *App) shutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rt != nil {
		_ = a.rt.Close()
		a.rt = nil
	}
}

// DBPath returns the local SQLite path (Wails binding).
func (a *App) DBPath() string {
	if err := a.ensure(); err != nil || a.rt == nil {
		return ""
	}
	return a.rt.DBPath
}

// Version returns the desktop/gateway version (Wails binding).
func (a *App) Version() string {
	return host.Version
}
