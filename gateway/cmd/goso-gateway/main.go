// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.
// GOSO Gateway — commands: version, doctor, gateway (HTTP+WS+Session+LLM+4 channels+auth/ratelimit).

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/config"
	"github.com/mqglobal/goso/gateway/internal/health"
	"github.com/mqglobal/goso/gateway/internal/httpapi"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/ratelimit"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	name    = "goso-gateway"
	version = "0.1.0"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(0)
	}
	switch os.Args[1] {
	case "version", "--version", "-v":
		printVersion()
	case "doctor":
		printDoctor()
	case "gateway", "serve":
		runGateway(os.Args[2:])
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Printf(`%s %s — GOSO Gateway (clean-room)

Usage:
  goso-gateway <command>

Commands:
  version    Print version (JSON)
  doctor     Run health checks (JSON)
  gateway    Start HTTP gateway (SPEC 006)
  help       Show this help

gateway flags:
  --port int    Port (default env GOSO_PORT or 8080; 0 = random)
  --host string Host (default env GOSO_HOST or 127.0.0.1)

Environment:
  GOSO_PORT                Port (default 8080)
  GOSO_HOST                Bind host (default 127.0.0.1; Docker uses 0.0.0.0)
  GOSO_ANTHROPIC_API_KEY   Anthropic key (optional, falls back to echo)
  GOSO_OPENAI_API_KEY      OpenAI key (optional)
  GOSO_TELEGRAM_BOT_TOKEN  Telegram bot token (optional)
  GOSO_ZALO_OA_ACCESS_TOKEN Zalo OA access token (optional)
  GOSO_ZALO_PERSONAL_TOKEN Zalo Personal token (optional)
  GOSO_ENV                 Environment (default development)
  GOSO_DB_PATH             SQLite path (default :memory:)
  GOSO_ADMIN_TOKEN         Bearer token for /api/* and /ws (empty = dev mode)

`, name, version)
}

func printVersion() {
	out, _ := json.Marshal(map[string]string{"name": name, "version": version})
	fmt.Println(string(out))
}

func printDoctor() {
	cfg := config.Load()
	rep := health.Run(cfg)
	out, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Println(string(out))
	if !rep.OK {
		os.Exit(1)
	}
}

func runGateway(args []string) {
	fs := flag.NewFlagSet("gateway", flag.ExitOnError)
	port := fs.Int("port", 0, "port (0 = random, else overrides GOSO_PORT)")
	host := fs.String("host", "", "host (default env GOSO_HOST or 127.0.0.1)")
	_ = fs.Parse(args)

	cfg := config.Load()
	if *port != 0 {
		cfg.Port = *port
	}
	bindHost := *host
	if bindHost == "" {
		bindHost = os.Getenv("GOSO_HOST")
	}
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}
	addr := fmt.Sprintf("%s:%d", bindHost, cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen %s: %v", addr, err)
	}
	actualAddr := ln.Addr().String()
	fmt.Printf("GOSO gateway listening on %s (version %s)\n", actualAddr, version)

	dbPath := os.Getenv("GOSO_DB_PATH")
	st, closeDB, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store %q: %v", dbPath, err)
	}
	if closeDB != nil {
		defer closeDB()
	}
	if dbPath == "" || dbPath == ":memory:" {
		fmt.Println("store: memory")
	} else {
		fmt.Printf("store: sqlite %s\n", dbPath)
	}
	reg := llm.NewRegistry()
	// Prefer anthropic if configured, else openai, else echo.
	var provider llm.Provider = reg.Get("anthropic")
	if !reg.HasReal() {
		provider = llm.Echo{}
	} else if reg.Get("anthropic").Name() == "echo" {
		provider = reg.Get("openai")
	}
	fmt.Printf("LLM provider: %s (hasReal=%v)\n", provider.Name(), reg.HasReal())

	tg := &channel.Telegram{Store: st, LLM: provider}
	zp := &channel.ZaloPersonal{Store: st, LLM: provider}
	zo := &channel.ZaloOA{Store: st, LLM: provider}
	mux := httpapi.RouterWithAllChannels(st, version, provider, tg.HandleUpdate, zp.HandleUpdate, zo.HandleUpdate).(*http.ServeMux)
	httpapi.RegisterWS(mux)

	// Auth + rate limit (AC 01–03)
	adminToken := os.Getenv("GOSO_ADMIN_TOKEN")
	rateLimit := 60
	if v := os.Getenv("GOSO_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			rateLimit = n
		}
	}
	if adminToken == "" {
		fmt.Println("auth: dev mode (no GOSO_ADMIN_TOKEN)")
	} else {
		fmt.Println("auth: enabled")
	}
	if rateLimit > 0 {
		fmt.Printf("rate limit: %d req/min/IP\n", rateLimit)
	} else {
		fmt.Println("rate limit: off")
	}
	var handler http.Handler = mux
	if rateLimit > 0 {
		handler = ratelimit.New(rateLimit).Middleware(handler)
	}
	handler = auth.RequireToken(adminToken, []string{"/healthz"})(handler)

	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
