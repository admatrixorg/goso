// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.
// GOSO Gateway — commands: version, doctor, gateway (HTTP+WS+Session+LLM+7 channels+auth/ratelimit).

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
	"syscall"
	"time"

	"github.com/mqglobal/goso/gateway/internal/config"
	"github.com/mqglobal/goso/gateway/internal/health"
	"github.com/mqglobal/goso/gateway/internal/serve"
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
  --port int    Port (omit = env GOSO_PORT or 8080; 0 = random)
  --host string Host (default env GOSO_HOST or 127.0.0.1)

Environment:
  GOSO_PORT                Port (default 8080)
  GOSO_HOST                Bind host (default 127.0.0.1; Docker uses 0.0.0.0)
  GOSO_ANTHROPIC_API_KEY   Anthropic key (optional; empty → absent, echo fallback)
  GOSO_OPENAI_API_KEY      OpenAI key (optional)
  GOSO_OPENROUTER_API_KEY  Named OpenAI-compat (also GROQ, DEEPSEEK, GEMINI, MISTRAL, XAI, MINIMAX, DASHSCOPE)
  GOSO_TELEGRAM_BOT_TOKEN  Telegram bot token (optional)
  GOSO_ZALO_OA_ACCESS_TOKEN Zalo OA access token (optional)
  GOSO_ZALO_PERSONAL_TOKEN Zalo Personal token (optional)
  GOSO_DISCORD_BOT_TOKEN   Discord bot token (optional)
  GOSO_SLACK_BOT_TOKEN     Slack bot token (optional)
  GOSO_FEISHU_APP_SECRET   Feishu/Lark app secret (optional)
  GOSO_WHATSAPP_ACCESS_TOKEN WhatsApp Cloud API token (optional; native vs Business = DI-01)
  GOSO_WS_ORIGINS          WS Origin allowlist, comma-separated (empty = allow all)
  GOSO_ENV                 Environment (default development)
  GOSO_DB_PATH             SQLite path (default :memory:)
  GOSO_VAULT_DIR           Knowledge vault root (default data/vault)
  GOSO_LITE                1 = cap 5 agents / 1 team (SPEC 038)
  GOSO_ADMIN_TOKEN         Bearer token for /api/* and /ws (required unless GOSO_DEV_MODE=1)
  GOSO_VIEW_TOKEN          Optional GET-only token for /healthz /api/agents /api/sessions
  GOSO_DEV_MODE            1 = explicit passthrough when token is empty (default: refuse 401)
  GOSO_INJECTION           log (default) or block prompt-injection matches on /api/chat
  GOSO_SSRF                1 = block literal localhost/private IPs on connector HTTP
  GOSO_WORKSPACE           Optional write jail; tools/vault cannot write outside
  GOSO_MASTER_KEY          32-byte hex AES-256-GCM key for secrets table (empty = refuse store)
  GOSO_OTEL_ENDPOINT       Optional OTLP HTTP JSON URL. Empty = no export (noop). No Grafana Cloud keys.

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
	port := fs.Int("port", -1, "port (omit = env GOSO_PORT or 8080; 0 = random)")
	host := fs.String("host", "", "host (default env GOSO_HOST or 127.0.0.1)")
	_ = fs.Parse(args)

	cfg := config.Load()
	if *port >= 0 {
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
	handler, status := serve.New(st, version)
	fmt.Printf("LLM provider: %s (hasReal=%v)\n", status.Provider, status.HasReal)
	if status.Auth {
		fmt.Println("auth: enabled")
	} else if status.DevMode {
		fmt.Println("auth: GOSO_DEV_MODE=1 (passthrough)")
	} else {
		fmt.Println("auth: required (GOSO_ADMIN_TOKEN empty — /api returns 401)")
	}
	if status.RateLimit > 0 {
		fmt.Printf("rate limit: %d req/min/IP\n", status.RateLimit)
	} else {
		fmt.Println("rate limit: off")
	}

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
