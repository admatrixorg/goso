// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package connector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mqglobal/goso/gateway/internal/security"
)

// StdioPipes lets tests inject an in-process MCP stdio pair (no subprocess).
type StdioPipes struct {
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
}

type mcpClient struct {
	name     string
	mode     string // http | stdio
	endpoint string
	token    string
	client   *http.Client
	stdio    *StdioPipes
	cmd      *exec.Cmd
	overlay  *Manifest
	rawMan   json.RawMessage
	manURL   string
	idSeq    atomic.Int64
	mu       sync.Mutex
	stdioR   *bufio.Reader
}

func newMCPHTTP(cfg Config) (Connector, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("mcp-http endpoint is required")
	}
	c := &mcpClient{
		name:     cfg.Name,
		mode:     "http",
		endpoint: strings.TrimRight(cfg.Endpoint, "/"),
		token:    cfg.BearerToken,
		client:   cfg.Client,
		rawMan:   cfg.ManifestJSON,
		manURL:   cfg.ManifestURL,
	}
	if c.client == nil {
		c.client = &http.Client{Timeout: cfg.Timeout}
	}
	security.GuardClient(c.client)
	if len(cfg.ManifestJSON) > 0 {
		m, err := ParseManifest(cfg.ManifestJSON)
		if err != nil {
			return nil, err
		}
		c.overlay = m
	}
	return c, nil
}

func newMCPStdio(cfg Config) (Connector, error) {
	c := &mcpClient{
		name:     cfg.Name,
		mode:     "stdio",
		endpoint: cfg.Endpoint,
		stdio:    cfg.Stdio,
		rawMan:   cfg.ManifestJSON,
		manURL:   cfg.ManifestURL,
	}
	if len(cfg.ManifestJSON) > 0 {
		m, err := ParseManifest(cfg.ManifestJSON)
		if err != nil {
			return nil, err
		}
		c.overlay = m
	}
	if c.stdio == nil && strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("mcp-stdio endpoint (command) is required")
	}
	return c, nil
}

func (c *mcpClient) Name() string { return c.name }

func (c *mcpClient) ListTools(ctx context.Context) ([]Tool, error) {
	if c.overlay != nil {
		return append([]Tool(nil), c.overlay.Tools...), nil
	}
	res, err := c.rpc(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	tools, err := toolsFromMCP(res)
	if err != nil {
		return nil, err
	}
	return tools, nil
}

func (c *mcpClient) Invoke(ctx context.Context, tool string, args map[string]any) (*InvokeResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	if c.overlay != nil {
		if t, ok := c.overlay.Tool(tool); ok {
			if err := ValidateArgs(t.InputSchema, args); err != nil {
				return nil, err
			}
		}
	}
	start := time.Now()
	res, err := c.rpc(ctx, "tools/call", map[string]any{"name": tool, "arguments": args})
	if err != nil {
		return nil, err
	}
	content, raw := mcpCallContent(res)
	lat := time.Since(start)
	return &InvokeResult{
		Tool:      tool,
		Connector: c.name,
		Content:   content,
		Raw:       raw,
		Latency:   lat,
		LatencyMS: lat.Milliseconds(),
		Status:    "ok",
	}, nil
}

func (c *mcpClient) Health(ctx context.Context) error {
	if c.overlay != nil {
		if err := c.overlay.Validate(); err != nil {
			return err
		}
	}
	_, err := c.rpc(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "goso", "version": "0.1.0"},
	})
	if err != nil {
		return unavailable(err)
	}
	if c.overlay == nil {
		tools, err := c.ListTools(ctx)
		if err != nil {
			return err
		}
		m := &Manifest{Tools: tools}
		if err := m.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c *mcpClient) rpc(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.idSeq.Add(1)
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return nil, err
	}
	if c.mode == "http" {
		return c.rpcHTTP(ctx, payload, id)
	}
	return c.rpcStdio(ctx, payload, id)
}

func (c *mcpClient) rpcHTTP(ctx context.Context, payload []byte, id int64) (json.RawMessage, error) {
	if err := security.CheckURL(c.endpoint); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, unavailable(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, unavailable(err)
	}
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadGateway {
		return nil, unavailable(fmt.Errorf("mcp http %d", resp.StatusCode))
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mcp http %d: %s", resp.StatusCode, bytesTrimSpace(body))
	}
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		body = extractSSEData(body)
	}
	return parseRPCResult(body, id)
}

func (c *mcpClient) rpcStdio(ctx context.Context, payload []byte, id int64) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ensureStdio(ctx); err != nil {
		return nil, unavailable(err)
	}
	if err := writeMCPFrame(c.stdio.Stdin, payload); err != nil {
		return nil, unavailable(err)
	}
	if c.stdioR == nil {
		c.stdioR = bufio.NewReader(c.stdio.Stdout)
	}
	type readRes struct {
		raw json.RawMessage
		err error
	}
	ch := make(chan readRes, 1)
	go func() {
		frame, err := readMCPFrame(c.stdioR)
		if err != nil {
			ch <- readRes{err: err}
			return
		}
		raw, err := parseRPCResult(frame, id)
		ch <- readRes{raw: raw, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, unavailable(ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return nil, unavailable(r.err)
		}
		return r.raw, nil
	}
}

func (c *mcpClient) ensureStdio(ctx context.Context) error {
	if c.stdio != nil {
		return nil
	}
	parts := strings.Fields(c.endpoint)
	if len(parts) == 0 {
		return fmt.Errorf("empty stdio command")
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd = cmd
	c.stdio = &StdioPipes{Stdin: stdin, Stdout: io.NopCloser(stdout)}
	c.stdioR = bufio.NewReader(c.stdio.Stdout)
	return nil
}

func parseRPCResult(body []byte, id int64) (json.RawMessage, error) {
	var resp struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      any             `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("mcp rpc json: %w (%s)", err, bytesTrimSpace(body))
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("mcp rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	if len(resp.Result) == 0 || string(resp.Result) == "null" {
		return json.RawMessage(`{}`), nil
	}
	_ = id
	return resp.Result, nil
}

func toolsFromMCP(result json.RawMessage) ([]Tool, error) {
	var wrap struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(result, &wrap); err != nil {
		return nil, err
	}
	if len(wrap.Tools) == 0 {
		return nil, fmt.Errorf("mcp tools/list returned no tools")
	}
	out := make([]Tool, 0, len(wrap.Tools))
	for i, raw := range wrap.Tools {
		t, err := parseTool(raw)
		if err != nil {
			return nil, fmt.Errorf("mcp tools[%d]: %w", i, err)
		}
		out = append(out, t)
	}
	return out, nil
}

func mcpCallContent(result json.RawMessage) (any, json.RawMessage) {
	var wrap struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(result, &wrap); err != nil || len(wrap.Content) == 0 {
		var v any
		_ = json.Unmarshal(result, &v)
		return v, result
	}
	text := wrap.Content[0].Text
	var parsed any
	if json.Unmarshal([]byte(text), &parsed) == nil {
		return parsed, result
	}
	return map[string]any{"text": text, "is_error": wrap.IsError}, result
}

func extractSSEData(body []byte) []byte {
	var last []byte
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data:") {
			last = []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(last) > 0 {
		return last
	}
	return body
}

func writeMCPFrame(w io.Writer, payload []byte) error {
	hdr := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	if _, err := io.WriteString(w, hdr); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func readMCPFrame(r io.Reader) ([]byte, error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	var contentLen int
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "content-length:") {
			n, err := strconv.Atoi(strings.TrimSpace(line[len("Content-Length:"):]))
			if err != nil {
				return nil, err
			}
			contentLen = n
		}
	}
	if contentLen <= 0 {
		return nil, fmt.Errorf("mcp stdio: missing Content-Length")
	}
	if contentLen > 1<<20 {
		return nil, fmt.Errorf("mcp stdio: frame too large")
	}
	buf := make([]byte, contentLen)
	_, err := io.ReadFull(br, buf)
	return buf, err
}

// ServeFakeMCP is an in-process MCP JSON-RPC handler (HTTP). Used by tests.
func ServeFakeMCP(tools []Tool, call func(name string, args map[string]any) (any, error)) http.Handler {
	if call == nil {
		call = func(name string, args map[string]any) (any, error) {
			return map[string]any{"ok": true, "tool": name, "args": args}, nil
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      any             `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]string{"name": "fake-mcp", "version": "0.0.1"},
			}
		case "tools/list":
			mcpTools := make([]map[string]any, 0, len(tools))
			for _, t := range tools {
				var schema any
				_ = json.Unmarshal(t.InputSchema, &schema)
				mcpTools = append(mcpTools, map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"inputSchema": schema,
				})
			}
			result = map[string]any{"tools": mcpTools}
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			out, err := call(p.Name, p.Arguments)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"error":   map[string]any{"code": -32000, "message": err.Error()},
				})
				return
			}
			text, _ := json.Marshal(out)
			result = map[string]any{
				"content": []map[string]string{{"type": "text", "text": string(text)}},
				"isError": false,
			}
		default:
			result = map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		})
	})
}

// StartFakeMCPStdio runs an in-process MCP stdio server on pipes.
func StartFakeMCPStdio(tools []Tool, call func(name string, args map[string]any) (any, error)) (*StdioPipes, func()) {
	serverIn, clientOut := io.Pipe() // client writes clientOut → server reads serverIn
	clientIn, serverOut := io.Pipe() // server writes serverOut → client reads clientIn
	handler := ServeFakeMCP(tools, call)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer serverIn.Close()
		defer serverOut.Close()
		br := bufio.NewReader(serverIn)
		for {
			frame, err := readMCPFrame(br)
			if err != nil {
				return
			}
			rec := httptestRecorder{header: make(http.Header)}
			req, _ := http.NewRequest(http.MethodPost, "http://stdio/mcp", bytes.NewReader(frame))
			req.Header.Set("Content-Type", "application/json")
			handler.ServeHTTP(&rec, req)
			_ = writeMCPFrame(serverOut, rec.body.Bytes())
		}
	}()
	pipes := &StdioPipes{Stdin: clientOut, Stdout: clientIn}
	stop := func() {
		_ = clientOut.Close()
		_ = clientIn.Close()
		<-done
	}
	return pipes, stop
}

// httptestRecorder is a tiny ResponseWriter so ServeFakeMCP can be reused for stdio.
type httptestRecorder struct {
	header http.Header
	body   bytes.Buffer
	code   int
}

func (r *httptestRecorder) Header() http.Header { return r.header }
func (r *httptestRecorder) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = 200
	}
	return r.body.Write(b)
}
func (r *httptestRecorder) WriteHeader(status int) { r.code = status }
