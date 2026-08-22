// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// BoundTool is a tool advertised to the LLM, namespaced by connector.
type BoundTool struct {
	Connector string         `json:"connector"`
	Tool      connector.Tool `json:"tool"`
}

// Trace records one tool attempt.
type Trace struct {
	Connector  string `json:"connector"`
	Tool       string `json:"tool"`
	LatencyMS  int64  `json:"latency_ms"`
	Status     string `json:"status"`
	ApprovalID string `json:"approval_id,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ChatResult is returned by Runtime.Chat.
type ChatResult struct {
	Reply     string  `json:"reply"`
	SessionID string  `json:"session_id"`
	Trace     []Trace `json:"trace,omitempty"`
}

// CallResult is a single tool-layer invocation (direct or from chat).
type CallResult struct {
	Result  *connector.InvokeResult `json:"result,omitempty"`
	Trace   Trace                   `json:"trace"`
	Pending bool                    `json:"pending,omitempty"`
}

// Runtime is the Tool Layer: connector tools + LLM + session messages.
type Runtime struct {
	Store    store.StoreIface
	Registry *connector.Registry
	Gate     *approval.Gate
	Events   *eventstore.Store
	LLM      llm.Provider
}

// New constructs a Runtime. Missing deps are allocated empty.
func New(st store.StoreIface, reg *connector.Registry, gate *approval.Gate, ev *eventstore.Store, provider llm.Provider) *Runtime {
	if reg == nil {
		reg = connector.NewRegistry()
	}
	if gate == nil {
		gate = approval.New(0)
	}
	if ev == nil {
		ev = eventstore.New(256)
	}
	if provider == nil {
		provider = llm.Echo{}
	}
	return &Runtime{Store: st, Registry: reg, Gate: gate, Events: ev, LLM: provider}
}

// ListTools returns tools from connectors linked to the agent (or all if none linked).
func (rt *Runtime) ListTools(ctx context.Context, agentID string) ([]BoundTool, error) {
	names := rt.connectorNames(agentID)
	var out []BoundTool
	for _, name := range names {
		c, err := rt.Registry.Lookup(name)
		if err != nil {
			continue
		}
		tools, err := c.ListTools(ctx)
		if err != nil {
			continue
		}
		for _, t := range tools {
			out = append(out, BoundTool{Connector: name, Tool: t})
		}
	}
	return out, nil
}

func (rt *Runtime) connectorNames(agentID string) []string {
	if agentID != "" && rt.Store != nil {
		if linked, err := rt.Store.ListAgentConnectors(agentID); err == nil && len(linked) > 0 {
			return linked
		}
	}
	var names []string
	for _, c := range rt.Registry.List() {
		names = append(names, c.Name())
	}
	return names
}

// CallTool is the Tool Layer entry: approval gate first, then Invoke.
func (rt *Runtime) CallTool(ctx context.Context, connectorName, tool string, args map[string]any) (*CallResult, error) {
	traceID := eventstore.NewTraceID()
	start := time.Now()
	rt.Events.Append(eventstore.Event{
		TraceID:   traceID,
		Connector: connectorName,
		Tool:      tool,
		Kind:      eventstore.KindAttempt,
		Summary:   eventstore.SummarizeArgs(args),
	})

	c, err := rt.Registry.Lookup(connectorName)
	if err != nil {
		return rt.fail(traceID, connectorName, tool, start, err)
	}
	tools, err := c.ListTools(ctx)
	if err != nil {
		return rt.fail(traceID, connectorName, tool, start, err)
	}
	var meta connector.Tool
	found := false
	for _, t := range tools {
		if t.Name == tool {
			meta = t
			found = true
			break
		}
	}
	if !found {
		return rt.fail(traceID, connectorName, tool, start, fmt.Errorf("unknown tool %q", tool))
	}

	if meta.RequiresApproval {
		req := rt.Gate.Submit(connectorName, tool, args, map[string]any{
			"requires_approval": true,
			"description":       meta.Description,
		})
		lat := time.Since(start)
		rt.Events.Append(eventstore.Event{
			TraceID:   traceID,
			Connector: connectorName,
			Tool:      tool,
			Kind:      eventstore.KindPendingApproval,
			Summary:   fmt.Sprintf(`{"approval_id":%q,"status":"pending_approval"}`, req.ID),
		})
		return &CallResult{
			Pending: true,
			Result: &connector.InvokeResult{
				Tool:             tool,
				Connector:        connectorName,
				Status:           "pending_approval",
				ApprovalID:       req.ID,
				RequiresApproval: true,
				Content:          map[string]any{"pending_approval": true, "approval_id": req.ID},
				Latency:          lat,
				LatencyMS:        lat.Milliseconds(),
			},
			Trace: Trace{
				Connector:  connectorName,
				Tool:       tool,
				LatencyMS:  lat.Milliseconds(),
				Status:     "pending_approval",
				ApprovalID: req.ID,
			},
		}, nil
	}

	res, err := c.Invoke(ctx, tool, args)
	lat := time.Since(start)
	if err != nil {
		return rt.fail(traceID, connectorName, tool, start, err)
	}
	if res == nil {
		res = &connector.InvokeResult{Tool: tool, Connector: connectorName, Status: "ok"}
	}
	res.Latency = lat
	res.LatencyMS = lat.Milliseconds()
	if res.Status == "" {
		res.Status = "ok"
	}
	rt.Events.Append(eventstore.Event{
		TraceID:   traceID,
		Connector: connectorName,
		Tool:      tool,
		Kind:      eventstore.KindSuccess,
		Summary:   eventstore.Redact(asJSON(map[string]any{"status": res.Status, "latency_ms": lat.Milliseconds()})),
	})
	return &CallResult{
		Result: res,
		Trace: Trace{
			Connector: connectorName,
			Tool:      tool,
			LatencyMS: lat.Milliseconds(),
			Status:    res.Status,
		},
	}, nil
}

func (rt *Runtime) fail(traceID, connectorName, tool string, start time.Time, err error) (*CallResult, error) {
	lat := time.Since(start)
	kind := eventstore.KindError
	if errors.Is(err, connector.ErrUnavailable) {
		kind = eventstore.KindUnavailable
	}
	rt.Events.Append(eventstore.Event{
		TraceID:   traceID,
		Connector: connectorName,
		Tool:      tool,
		Kind:      kind,
		Summary:   eventstore.Redact(err.Error()),
	})
	tr := Trace{
		Connector: connectorName,
		Tool:      tool,
		LatencyMS: lat.Milliseconds(),
		Status:    "error",
		Error:     err.Error(),
	}
	if errors.Is(err, connector.ErrUnavailable) {
		tr.Status = "connector_unavailable"
	}
	return &CallResult{Trace: tr}, err
}

// Chat persists the user message, optionally invokes matched tools, then asks the LLM.
func (rt *Runtime) Chat(ctx context.Context, sessionID, message string) (*ChatResult, error) {
	if rt.Store == nil {
		return nil, errors.New("store required")
	}
	sess, err := rt.Store.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	if _, err := rt.Store.AddMessage(store.Message{SessionID: sessionID, Role: "user", Content: message}); err != nil {
		return nil, err
	}

	var traces []Trace
	tools, _ := rt.ListTools(ctx, sess.AgentID)
	for _, call := range matchTools(message, tools) {
		cr, err := rt.CallTool(ctx, call.Connector, call.Tool, call.Args)
		if cr != nil {
			traces = append(traces, cr.Trace)
			content := ""
			if err != nil {
				content = asJSON(map[string]any{"error": err.Error(), "trace": cr.Trace})
			} else if cr.Result != nil {
				content = asJSON(cr.Result)
			}
			_, _ = rt.Store.AddMessage(store.Message{SessionID: sessionID, Role: "tool", Content: content})
		} else if err != nil {
			traces = append(traces, Trace{Connector: call.Connector, Tool: call.Tool, Status: "error", Error: err.Error()})
		}
	}

	history, err := rt.Store.ListMessages(sessionID)
	if err != nil {
		return nil, err
	}
	var msgs []llm.Message
	for _, m := range history {
		msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
	}
	reply, err := rt.LLM.Chat(ctx, msgs)
	if err != nil {
		return nil, err
	}
	if pending := firstPending(traces); pending != "" && !strings.Contains(reply, "pending_approval") {
		reply = reply + "\npending_approval: " + pending
	}
	_, _ = rt.Store.AddMessage(store.Message{SessionID: sessionID, Role: "assistant", Content: reply})
	return &ChatResult{Reply: reply, SessionID: sessionID, Trace: traces}, nil
}

type intendedCall struct {
	Connector string
	Tool      string
	Args      map[string]any
}

func matchTools(message string, tools []BoundTool) []intendedCall {
	lower := strings.ToLower(message)
	var out []intendedCall
	seen := map[string]struct{}{}
	for _, bt := range tools {
		key := bt.Connector + "/" + bt.Tool.Name
		name := strings.ToLower(bt.Tool.Name)
		matched := strings.Contains(lower, name)
		if !matched {
			matched = intentMatch(lower, name, bt.Tool)
		}
		if !matched {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, intendedCall{
			Connector: bt.Connector,
			Tool:      bt.Tool.Name,
			Args:      extractArgs(message, bt.Tool),
		})
	}
	return out
}

func intentMatch(lower, name string, tool connector.Tool) bool {
	if name == "contact_search" || strings.Contains(name, "contact") {
		if strings.Contains(lower, "tìm khách") || strings.Contains(lower, "tim khach") ||
			strings.Contains(lower, "search contact") || strings.Contains(lower, "find customer") {
			return true
		}
	}
	if strings.Contains(name, "order") && (strings.Contains(lower, "đơn") || strings.Contains(lower, "order")) {
		return true
	}
	if tool.RequiresApproval && (strings.Contains(name, "message_send") || strings.Contains(name, "send")) {
		if strings.Contains(lower, "gửi tin") || strings.Contains(lower, "gui tin") || strings.Contains(lower, "send message") {
			return true
		}
	}
	if strings.Contains(name, "price") && (strings.Contains(lower, "đổi giá") || strings.Contains(lower, "change price")) {
		return true
	}
	return false
}

func extractArgs(message string, tool connector.Tool) map[string]any {
	args := map[string]any{}
	if i := strings.Index(message, "{"); i >= 0 {
		var parsed map[string]any
		if json.Unmarshal([]byte(message[i:]), &parsed) == nil {
			return parsed
		}
	}
	lower := strings.ToLower(message)
	query := strings.TrimSpace(message)
	for _, prefix := range []string{"tìm khách", "tim khach", "search contact", "find customer"} {
		if idx := strings.Index(lower, prefix); idx >= 0 {
			query = strings.TrimSpace(message[idx+len(prefix):])
			break
		}
	}
	query = strings.TrimFunc(query, func(r rune) bool { return unicode.IsSpace(r) || r == ':' || r == '-' })
	if query == "" {
		query = strings.TrimSpace(message)
	}
	var schema map[string]any
	_ = json.Unmarshal(tool.InputSchema, &schema)
	req, _ := schema["required"].([]any)
	props, _ := schema["properties"].(map[string]any)
	if len(req) > 0 {
		for _, x := range req {
			name, _ := x.(string)
			if name == "" {
				continue
			}
			switch name {
			case "query", "q", "text", "message":
				args[name] = query
			case "contact_id":
				args[name] = query
			case "order_id":
				args[name] = query
			case "sku":
				args[name] = query
			case "price":
				args[name] = 0
			default:
				args[name] = query
			}
		}
		return args
	}
	if _, ok := props["query"]; ok {
		args["query"] = query
		return args
	}
	args["query"] = query
	return args
}

func firstPending(tr []Trace) string {
	for _, t := range tr {
		if t.Status == "pending_approval" && t.ApprovalID != "" {
			return t.ApprovalID
		}
	}
	return ""
}

func asJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
