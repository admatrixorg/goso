// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// ValidateRuntime refuses to boot a production bridge that cannot verify
// signatures, seal API keys, or persist them in Postgres.
func ValidateRuntime() error {
	if !security.Production() {
		return nil
	}
	if strings.TrimSpace(os.Getenv("GOSO_SHARED_SECRET")) == "" {
		return errors.New("GOSO_SHARED_SECRET is required when GOSO_ENV=production")
	}
	if _, _, err := secrets.Encrypt([]byte("x")); err != nil {
		return errors.New("GOSO_MASTER_KEY must be 32-byte hex when GOSO_ENV=production")
	}
	if !store.IsPostgresDSN(os.Getenv("GOSO_DATABASE_URL")) {
		return errors.New("GOSO_DATABASE_URL must be a postgres DSN when GOSO_ENV=production")
	}
	return nil
}

// ListenAddr is host:port for the bridge. Port defaults to 8088 when
// GOSO_PORT is unset. flagPort >= 0 and a non-empty flagHost win.
func ListenAddr(flagHost string, flagPort int) string {
	host := strings.TrimSpace(flagHost)
	if host == "" {
		host = strings.TrimSpace(os.Getenv("GOSO_HOST"))
	}
	if host == "" {
		host = "127.0.0.1"
	}
	port := flagPort
	if port < 0 {
		port = 8088
		if v := strings.TrimSpace(os.Getenv("GOSO_PORT")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				port = n
			}
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// NewMux serves health, signed HTTP, and the gRPC bridge on one handler.
// Wrap it with h2c so connect.WithGRPC clients can use cleartext HTTP/2.
func NewMux(svc *Service) http.Handler {
	if svc == nil {
		svc = &Service{}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", svc.handleHealth)
	mux.HandleFunc("GET /v1/health", svc.handleHealth)
	mux.HandleFunc("POST /v1/chat/reply", svc.handleReply)
	mux.HandleFunc("PUT /v1/tenants/{tenantId}/providers/{providerId}", svc.handlePutProvider)
	mux.HandleFunc("DELETE /v1/tenants/{tenantId}/providers/{providerId}", svc.handleDeleteProvider)
	mux.HandleFunc("PUT /v1/tenants/{tenantId}/agents/{agentId}/knowledge", svc.handlePutKnowledge)
	mux.HandleFunc("PUT /v1/tenants/{tenantId}/agents/{agentId}", svc.handlePutAgent)
	mux.HandleFunc("DELETE /v1/tenants/{tenantId}/agents/{agentId}", svc.handleDeleteAgent)
	mux.Handle(procGenerate, connect.NewUnaryHandler(procGenerate, svc.generateReply))
	mux.Handle(procStream, connect.NewServerStreamHandler(procStream, svc.streamReply))
	mux.Handle(procHealth, connect.NewUnaryHandler(procHealth, svc.checkHealth))
	return accessLog(mux)
}

// Run listens until ctx is cancelled. gRPC is HTTP/2 cleartext (h2c);
// HTTP/1.1 still serves /v1/chat/reply and the config routes.
func Run(ctx context.Context, addr string, svc *Service) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	// WriteTimeout stays unset. A single deadline covers the whole response,
	// including an SSE body, and would cut a long upstream stream. Header
	// wait is bounded by the outbound ResponseHeaderTimeout; the request
	// context cancels the rest. ReadHeaderTimeout still bounds slow clients.
	srv := &http.Server{
		Handler:           h2c.NewHandler(NewMux(svc), &http2.Server{}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() {
		errc <- srv.Serve(ln)
	}()
	select {
	case <-ctx.Done():
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
		return nil
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
