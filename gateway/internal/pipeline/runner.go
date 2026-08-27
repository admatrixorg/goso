// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/observe"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/team"
)

const (
	MaxIterations      = 20
	MaxIterationsReply = "max_iterations"
)

// ToolTrace is one act-stage tool attempt.
type ToolTrace struct {
	Connector  string
	Tool       string
	LatencyMS  int64
	Status     string
	ApprovalID string
	Error      string
}

// CallOutcome is returned by ToolService.Call.
type CallOutcome struct {
	Content string
	Trace   ToolTrace
	Pending bool
}

// ToolService lists advertised tools and executes model-requested calls.
type ToolService interface {
	List(ctx context.Context, agentID string) ([]llm.ToolSpec, error)
	Call(ctx context.Context, agentID string, call llm.ToolCall) (CallOutcome, error)
}

// StageFunc is a named plug-in point (memory / summarize).
type StageFunc func(ctx context.Context, st *State) error

// State is the per-run context passed through stages.
type State struct {
	SessionID      string
	AgentID        string
	Mode           Mode
	DisplayName    string
	Instructions   string
	TeamNote       string
	TeamFile       string
	Messages       []llm.Message
	Tools          []llm.ToolSpec
	Traces         []ToolTrace
	Reply          string
	Pending        bool
	ForceSummarize bool
	Provider       llm.Provider
}

// Result is returned to Runtime.Chat.
type Result struct {
	Reply     string
	SessionID string
	Trace     []ToolTrace
	Spans     []observe.Span
}

// Runner is the 8-stage chat pipeline.
type Runner struct {
	Store          store.StoreIface
	Tools          ToolService
	LLM            llm.Provider
	Hooks          *Dispatcher
	MaxIter        int
	Cap            int
	Memory         StageFunc
	Summarize      StageFunc
	ForceSummarize bool
}

// NewRunner fills defaults (max 20, cap 50, empty hooks).
func NewRunner(st store.StoreIface, tools ToolService, provider llm.Provider, hooks *Dispatcher) *Runner {
	if hooks == nil {
		hooks = NewDispatcher()
	}
	if provider == nil {
		provider = llm.Echo{}
	}
	return &Runner{
		Store:     st,
		Tools:     tools,
		LLM:       provider,
		Hooks:     hooks,
		MaxIter:   MaxIterations,
		Cap:       HistoryCap,
		Memory:    MemoryStage(st),
		Summarize: SummarizeStage(st),
	}
}

// Run executes context → history → prompt → loop(think, act, observe) → memory → summarize.
func (r *Runner) Run(ctx context.Context, sessionID, userText string, mode Mode) (res *Result, err error) {
	if r == nil || r.Store == nil {
		return nil, fmt.Errorf("store required")
	}
	if r.MaxIter <= 0 {
		r.MaxIter = MaxIterations
	}
	if r.Cap <= 0 {
		r.Cap = HistoryCap
	}
	if mode == "" {
		mode = ModeFull
	}

	ctx = observe.WithCollector(ctx, observe.NewCollector())
	ctx, agentSpan := observe.StartSpan(ctx, observe.KindAgent, "agent")
	agentSpan.SetAttr("session_id", sessionID)
	if rid := observe.RequestIDFromContext(ctx); rid != "" {
		agentSpan.SetAttr("request_id", rid)
	}
	defer func() {
		agentSpan.End(err)
		spans := observe.SpansFrom(ctx)
		if res == nil {
			res = &Result{SessionID: sessionID}
		}
		res.Spans = spans
	}()

	st, err := r.loadContext(sessionID, mode)
	if err != nil {
		return nil, err
	}
	st.ForceSummarize = r.ForceSummarize
	st.Provider = r.LLM
	if isFirstUserTurn(r.Store, sessionID) {
		r.Hooks.Fire(ctx, Event{Name: SessionStart, SessionID: st.SessionID, AgentID: st.AgentID})
	}

	if _, err := r.Store.AddMessage(store.Message{SessionID: sessionID, Role: "user", Content: userText}); err != nil {
		return nil, err
	}
	r.Hooks.Fire(ctx, Event{Name: UserPromptSubmit, SessionID: st.SessionID, AgentID: st.AgentID})

	if err := r.loadHistory(st); err != nil {
		return nil, err
	}
	r.attachPrompt(st)
	if r.Tools != nil {
		st.Tools, _ = r.Tools.List(ctx, st.AgentID)
	}

	finished := false
	for i := 0; i < r.MaxIter; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		reply, err := r.think(ctx, st)
		if err != nil {
			return nil, err
		}
		st.Reply = reply.Text
		if len(reply.ToolCalls) == 0 {
			_, _ = r.Store.AddMessage(store.Message{SessionID: sessionID, Role: "assistant", Content: reply.Text})
			finished = true
			break
		}
		pending, err := r.actObserve(ctx, st, i, reply)
		if err != nil {
			return nil, err
		}
		if pending {
			st.Pending = true
			text := st.Reply
			if id := firstPending(st.Traces); id != "" && !strings.Contains(text, "pending_approval") {
				if text != "" {
					text = text + "\npending_approval: " + id
				} else {
					text = "pending_approval: " + id
				}
			}
			st.Reply = text
			_, _ = r.Store.AddMessage(store.Message{SessionID: sessionID, Role: "assistant", Content: text})
			finished = true
			break
		}
	}
	if !finished {
		text := st.Reply
		if strings.TrimSpace(text) == "" {
			text = MaxIterationsReply
		}
		st.Reply = text
		_, _ = r.Store.AddMessage(store.Message{SessionID: sessionID, Role: "assistant", Content: text})
	}

	r.memory(ctx, st)
	r.summarize(ctx, st)
	r.Hooks.Fire(ctx, Event{Name: Stop, SessionID: st.SessionID, AgentID: st.AgentID})
	res = &Result{Reply: st.Reply, SessionID: sessionID, Trace: st.Traces}
	return res, nil
}

func (r *Runner) loadContext(sessionID string, mode Mode) (*State, error) {
	sess, err := r.Store.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	st := &State{SessionID: sessionID, AgentID: sess.AgentID, Mode: mode}
	if a, err := r.Store.GetAgent(sess.AgentID); err == nil && a != nil {
		st.DisplayName = a.DisplayName
		st.Instructions = a.Instructions
	}
	st.TeamNote = team.Note(r.Store, sess.AgentID)
	st.TeamFile = team.ReadTEAMFile()
	return st, nil
}

func (r *Runner) loadHistory(st *State) error {
	raw, err := r.Store.ListMessages(st.SessionID)
	if err != nil {
		return err
	}
	st.Messages = prependSummary(r.Store, st.SessionID, Sanitize(CapLast(ToLLM(raw), r.Cap)))
	return nil
}

func (r *Runner) attachPrompt(st *State) {
	sys := SystemPrompt(st.Mode, st.DisplayName)
	if note := strings.TrimSpace(st.TeamNote); note != "" {
		if sys == "" {
			sys = note
		} else {
			sys = note + "\n" + sys
		}
	}
	if extra := strings.TrimSpace(st.TeamFile); extra != "" && st.Mode != ModeNone {
		sys = strings.TrimSpace(sys + "\n" + extra)
	}
	if ins := strings.TrimSpace(st.Instructions); ins != "" && st.Mode != ModeNone {
		sys = strings.TrimSpace(sys + "\n" + ins)
	}
	if boot := strings.TrimSpace(BootstrapText()); boot != "" && st.Mode != ModeNone {
		sys = strings.TrimSpace(sys + "\n" + boot)
	}
	if sys == "" {
		return
	}
	st.Messages = append([]llm.Message{{Role: "system", Content: sys}}, st.Messages...)
}

func (r *Runner) think(ctx context.Context, st *State) (reply llm.Reply, err error) {
	name := "llm"
	if r.LLM != nil {
		name = r.LLM.Name()
	}
	_, span := observe.StartSpan(ctx, observe.KindLLM, name)
	defer func() { span.End(err) }()
	if tc, ok := r.LLM.(llm.ToolChat); ok {
		reply, err = tc.ChatTools(ctx, st.Messages, st.Tools)
	} else {
		var text string
		var usage llm.Usage
		text, usage, err = llm.ChatUsage(ctx, r.LLM, st.Messages)
		reply = llm.Reply{Text: text, Usage: usage}
	}
	span.SetCacheReadTokens(reply.Usage.CacheReadTokens)
	return reply, err
}

func (r *Runner) actObserve(ctx context.Context, st *State, iter int, reply llm.Reply) (pending bool, err error) {
	for i := range reply.ToolCalls {
		if reply.ToolCalls[i].ID == "" {
			reply.ToolCalls[i].ID = fmt.Sprintf("call_%d_%d", iter, i)
		}
	}
	_, _ = r.Store.AddMessage(store.Message{
		SessionID: st.SessionID,
		Role:      "assistant",
		Content:   EncodeAssistant(reply.Text, reply.ToolCalls),
	})
	st.Messages = append(st.Messages, llm.Message{Role: "assistant", Content: reply.Text, ToolCalls: reply.ToolCalls})

	for _, call := range reply.ToolCalls {
		conn, tool := ResolveCall(call)
		args := cloneArgs(call.Arguments)
		r.Hooks.Fire(ctx, Event{
			Name:      PreToolUse,
			SessionID: st.SessionID,
			AgentID:   st.AgentID,
			Tool:      tool,
			Connector: conn,
			Arguments: args,
		})
		_, toolSpan := observe.StartSpan(ctx, observe.KindTool, call.Name)
		var out CallOutcome
		if !isAdvertised(call, st.Tools) {
			out.Trace = ToolTrace{Connector: conn, Tool: tool, Status: "error", Error: "tool not advertised"}
			out.Content = out.Trace.Error
			err = nil
		} else if r.Tools != nil {
			// spawn / delegate / team_tasks stay in the act stage (not connector).
			out, err = r.Tools.Call(ctx, st.AgentID, call)
		} else {
			out.Trace = ToolTrace{Connector: conn, Tool: tool, Status: "error", Error: "no tool service"}
			out.Content = out.Trace.Error
			err = nil
		}
		if out.Trace.Status != "" || out.Trace.Error != "" {
			toolSpan.SetStatus(out.Trace.Status, out.Trace.Error)
		}
		toolSpan.End(err)
		if out.Trace.Connector == "" {
			out.Trace.Connector = conn
		}
		if out.Trace.Tool == "" {
			out.Trace.Tool = tool
		}
		content := out.Content
		if err != nil && content == "" {
			content = err.Error()
		}
		st.Traces = append(st.Traces, out.Trace)
		_, _ = r.Store.AddMessage(store.Message{
			SessionID: st.SessionID,
			Role:      "tool",
			Content:   EncodeTool(call.ID, content),
		})
		st.Messages = append(st.Messages, llm.Message{
			Role:       "tool",
			Content:    security.WrapUntrusted(content),
			ToolCallID: call.ID,
		})
		r.Hooks.Fire(ctx, Event{
			Name:      PostToolUse,
			SessionID: st.SessionID,
			AgentID:   st.AgentID,
			Tool:      tool,
			Connector: conn,
			Arguments: call.Arguments,
			Result:    content,
			Error:     out.Trace.Error,
		})
		if out.Pending {
			pending = true
			break
		}
		err = nil
	}
	return pending, nil
}

func isFirstUserTurn(st store.StoreIface, sessionID string) bool {
	if st == nil {
		return true
	}
	raw, err := st.ListMessages(sessionID)
	if err != nil {
		return true
	}
	for _, m := range raw {
		if m != nil && m.Role == "user" {
			return false
		}
	}
	return true
}

func isAdvertised(call llm.ToolCall, tools []llm.ToolSpec) bool {
	conn, tool := ResolveCall(call)
	want := AdvertiseName(conn, tool)
	for _, t := range tools {
		if t.Name == call.Name || t.Name == want {
			return true
		}
		c, n, ok := SplitAdvertised(t.Name)
		if ok && c == conn && n == tool {
			return true
		}
		if t.Connector != "" && t.Connector == conn && (t.Name == tool || strings.HasSuffix(t.Name, "__"+tool)) {
			return true
		}
	}
	return false
}

func cloneArgs(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (r *Runner) memory(ctx context.Context, st *State) {
	if r.Memory != nil {
		_ = r.Memory(ctx, st)
	}
}

func (r *Runner) summarize(ctx context.Context, st *State) {
	if r.Summarize != nil {
		_ = r.Summarize(ctx, st)
	}
}

func firstPending(tr []ToolTrace) string {
	for _, t := range tr {
		if t.Status == "pending_approval" && t.ApprovalID != "" {
			return t.ApprovalID
		}
	}
	return ""
}
