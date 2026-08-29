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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mqglobal/goso/gateway/internal/backup"
	"github.com/mqglobal/goso/gateway/internal/config"
	"github.com/mqglobal/goso/gateway/internal/health"
	"github.com/mqglobal/goso/gateway/internal/security"
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
	case "restore":
		runRestore(os.Args[2:])
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
  restore    Restore a VACUUM INTO snapshot to a temp db (optional --apply)
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
  GOSO_ROUTER9_BASE_URL    Construct named provider router9 when set (key optional)
  GOSO_LLM_PROVIDER        Force Preferred() name when that provider exists
  GOSO_WEB_SEARCH          ddg or 1 = DuckDuckGo Instant Answer for builtin web_search (empty = not_configured)
  GOSO_SANDBOX_IMAGE       docker image for builtin sandbox; also needs docker on PATH (empty = not_configured)
  GOSO_BROWSER_BIN         existing Chrome/Chromium file (or CHROME_PATH); empty = browser not_configured
  GOSO_FFMPEG              existing ffmpeg file for builtin media (or PATH ffmpeg + GOSO_MEDIA=1)
  GOSO_MEDIA / GOSO_MEDIA_* 1 = PATH ffmpeg for media, and image_gen/tts only with an injected test double (never a paid API)
  GOSO_SKILLS_DIR          One-level SKILL.md folders for use_skill / skill_search / POST-DELETE /api/skills; empty = fail-closed
  GOSO_CONTEXT_DIR         Direct children SOUL.md IDENTITY.md AGENTS.md (optional USER.md); empty = no inject
  GOSO_TELEGRAM_BOT_TOKEN  Telegram bot token (optional)
  GOSO_ZALO_OA_ACCESS_TOKEN Zalo OA access token (optional)
  GOSO_ZALO_PERSONAL_TOKEN Zalo Personal token (optional)
  GOSO_DISCORD_BOT_TOKEN   Discord bot token (optional)
  GOSO_SLACK_BOT_TOKEN     Slack bot token (optional)
  GOSO_FEISHU_APP_SECRET   Feishu/Lark app secret (optional)
  GOSO_WHATSAPP_ACCESS_TOKEN WhatsApp Cloud API token (optional; native vs Business = DI-01)
  GOSO_WS_ORIGINS          WS Origin allowlist, comma-separated (empty = allow all; required when GOSO_ENV=production)
  GOSO_ENV                 Environment (default development; production = injection default block, no query token, WS origins required)
  GOSO_DB_PATH             SQLite path (default :memory:) when GOSO_DATABASE_URL is unset
  GOSO_DATABASE_URL        postgres://… opens pgx (fail-closed on connect; no sqlite fallback). Unset = SQLite. See docs/qa/085-postgres-local.md
  GOSO_MULTI_TENANT        1 = honor X-Goso-Tenant (admin token for non-default). Unset/demo = always default
  GOSO_KG_EXTRACT          1 = after chat, insert L2 entity from Name:/Entity: lines. Default off
  GOSO_EVOLUTION_AUTO      1 = in-process auto-adapt ticker (per-agent auto_adapt still required). Default off
  GOSO_BACKUP_DIR          Snapshot dir for VACUUM INTO (default ./var/backups)
  GOSO_VAULT_DIR           Knowledge vault root (default data/vault)
  GOSO_LITE                1 = cap 5 agents / 1 team; Channels page lite-off (SPEC 038/055)
  GOSO_ADMIN_TOKEN         Bearer token for /api/* and /ws (required unless GOSO_DEV_MODE=1)
  GOSO_VIEW_TOKEN          Optional GET-only token for /healthz /api/agents /api/sessions /api/nodes /api/workstations /api/storage /api/events /api/activity /api/logs
  Activity                 GET /api/activity append-only admin audit (action/actor/entity/IP/time, before cursor). Separate from Events. GET never returns secrets
  Logs                     GET /api/logs redacted tail (component/q/level, limit). SSE GET /api/logs/stream. GET never returns credentials
  Pairing                  Admin POST /api/pairing → one-time code (10 min); POST /api/pairing/exchange → view grant
  Nodes                    POST /api/nodes/request (no Bearer) pending device; GET list; admin approve/deny/revoke
  Workstations             GET/POST /api/workstations; PATCH/test/disconnect/delete. Identity is a path/ref; GET never returns keys
  Storage                  GET /api/storage list/preview/download; POST upload/delete. Jailed to GOSO_WORKSPACE; GET never returns credential values
  GOSO_DEV_MODE            1 = explicit passthrough when token is empty (default: refuse 401)
  GOSO_INJECTION           log or block prompt-injection matches on /api/chat (production default block)
  GOSO_SSRF                1 = DNS-aware block of localhost/private IPs on connector, LLM HTTP, web_fetch, and browser
  GOSO_WORKSPACE           Write jail; tools/vault cannot write outside. Empty = filesystem tools fail-closed
  GOSO_MASTER_KEY          32-byte hex AES-256-GCM key for secrets table (empty = refuse store)
  GOSO_OTEL_ENDPOINT       Optional OTLP HTTP JSON URL. Empty = no export (noop). Local Jaeger: docker compose --profile otel up -d jaeger then http://127.0.0.1:4318/v1/traces (compose-network http://jaeger:4318/v1/traces). No Grafana Cloud keys.

`, name, version)
}

func printVersion() {
	out, _ := json.Marshal(map[string]string{"name": name, "version": version})
	fmt.Println(string(out))
}

func runRestore(args []string) {
	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	file := fs.String("file", "", "snapshot basename under GOSO_BACKUP_DIR")
	apply := fs.Bool("apply", false, "replace dest (stop gateway first)")
	dest := fs.String("dest", "", "dest db path (default GOSO_DB_PATH)")
	_ = fs.Parse(args)
	if strings.TrimSpace(*file) == "" {
		log.Fatal("--file is required")
	}
	if *apply {
		d := strings.TrimSpace(*dest)
		if d == "" {
			d = os.Getenv("GOSO_DB_PATH")
		}
		if err := backup.Apply(*file, d); err != nil {
			log.Fatalf("restore --apply: %v", err)
		}
		out, _ := json.Marshal(map[string]any{"file": filepath.Base(*file), "integrity": "ok", "applied": true})
		fmt.Println(string(out))
		return
	}
	tmp, cleanup, err := backup.RestoreToTemp(*file)
	if err != nil {
		log.Fatalf("restore: %v", err)
	}
	defer cleanup()
	st, err := os.Stat(tmp)
	if err != nil {
		log.Fatalf("restore temp: %v", err)
	}
	out, _ := json.Marshal(map[string]any{
		"file":      filepath.Base(*file),
		"bytes":     st.Size(),
		"integrity": "ok",
		"applied":   false,
	})
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
		log.Fatalf("open store: %v", err)
	}
	if closeDB != nil {
		defer closeDB()
	}
	if err := security.CheckProduction(); err != nil {
		log.Fatalf("%v", err)
	}
	switch {
	case store.IsPostgresDSN(os.Getenv("GOSO_DATABASE_URL")) || store.IsPostgresDSN(dbPath):
		fmt.Println("store: postgres")
	case dbPath == "" || dbPath == ":memory:":
		fmt.Println("store: memory")
	default:
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
