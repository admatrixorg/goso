// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package team

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	ToolSpawn     = "spawn"
	ToolDelegate  = "delegate"
	ToolTeamTasks = "team_tasks"

	ModeManual   = "manual"
	ModeExplicit = "explicit"
	ModeAuto     = "auto"

	SyncTimeout = 10 * time.Second
)

// ChatFn runs a child session chat (used for sync delegate / spawn).
type ChatFn func(ctx context.Context, sessionID, message string) (string, error)

// Service dispatches spawn / delegate / team_tasks.
type Service struct {
	Store store.StoreIface
	Chat  ChatFn
}

type depthKey struct{}

// IsTool reports whether name is an orchestration tool (not a connector).
func IsTool(name string) bool {
	n := strings.TrimSpace(name)
	if i := strings.Index(n, "__"); i > 0 {
		n = n[i+2:]
	} else if i := strings.Index(n, "."); i > 0 {
		n = n[i+1:]
	}
	switch n {
	case ToolSpawn, ToolDelegate, ToolTeamTasks:
		return true
	default:
		return false
	}
}

// ParseMode accepts auto / explicit / manual (empty = unset).
func ParseMode(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", nil
	}
	switch s {
	case ModeAuto, ModeExplicit, ModeManual:
		return s, nil
	default:
		return "", fmt.Errorf("unknown orchestration_mode %q", s)
	}
}

// ResolveMode: persisted field, else team → auto, else links → explicit, else manual.
func ResolveMode(st store.StoreIface, agentID string) string {
	if st == nil || agentID == "" {
		return ModeManual
	}
	if a, err := st.GetAgent(agentID); err == nil && a != nil {
		if m, err := ParseMode(a.OrchestrationMode); err == nil && m != "" {
			return m
		}
	}
	if tm, err := st.TeamOfAgent(agentID); err == nil && tm != nil {
		return ModeAuto
	}
	if links, err := st.ListAgentLinks(agentID); err == nil && len(links) > 0 {
		return ModeExplicit
	}
	return ModeManual
}

// ToolSpecs advertised for a resolved mode.
func ToolSpecs(mode string) []llm.ToolSpec {
	delegate := llm.ToolSpec{
		Name:        ToolDelegate,
		Description: "Delegate work to another agent of your tenant. Args: to_agent_id (agent id or agent_key), message, mode=sync|async. When this agent has links, only linked agents are allowed.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"to_agent_id":{"type":"string"},"message":{"type":"string"},"mode":{"type":"string"}},"required":["to_agent_id","message"]}`),
	}
	spawn := llm.ToolSpec{
		Name:        ToolSpawn,
		Description: "Create a child agent clone (same model) and session. Optional message starts a child chat.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}}}`),
	}
	tasks := llm.ToolSpec{
		Name:        ToolTeamTasks,
		Description: "List, create, or update Kanban team tasks. Args: action=list|create|update, title, status, id, assignee_agent_id.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"action":{"type":"string"},"title":{"type":"string"},"status":{"type":"string"},"id":{"type":"string"},"assignee_agent_id":{"type":"string"}}}`),
	}
	switch mode {
	case ModeAuto:
		return []llm.ToolSpec{spawn, delegate, tasks}
	case ModeExplicit:
		return []llm.ToolSpec{delegate}
	default:
		return nil
	}
}

// Note is the short system line prepended when the agent is on a team.
func Note(st store.StoreIface, agentID string) string {
	if st == nil || agentID == "" {
		return ""
	}
	tm, err := st.TeamOfAgent(agentID)
	if err != nil || tm == nil {
		return ""
	}
	members, _ := st.ListTeamMembers(tm.ID)
	names := make([]string, 0, len(members))
	for _, m := range members {
		if m == nil {
			continue
		}
		label := m.AgentID
		if a, err := st.GetAgent(m.AgentID); err == nil && a != nil {
			if strings.TrimSpace(a.DisplayName) != "" {
				label = a.DisplayName
			} else {
				label = a.AgentKey
			}
		}
		names = append(names, label)
	}
	return "Team: " + tm.Name + "; members: " + strings.Join(names, ", ")
}

// ReadTEAMFile returns GOSO_VAULT_DIR/TEAM.md when present.
func ReadTEAMFile() string {
	root := strings.TrimSpace(os.Getenv("GOSO_VAULT_DIR"))
	if root == "" {
		root = filepath.Join("data", "vault")
	}
	b, err := os.ReadFile(filepath.Join(root, "TEAM.md"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Dispatch runs spawn / delegate / team_tasks for the calling agent.
func (svc *Service) Dispatch(ctx context.Context, agentID string, call llm.ToolCall) (string, error) {
	if svc == nil || svc.Store == nil {
		return "", errors.New("store required")
	}
	name := strings.TrimSpace(call.Name)
	if i := strings.Index(name, "__"); i > 0 {
		name = name[i+2:]
	} else if i := strings.Index(name, "."); i > 0 {
		name = name[i+1:]
	}
	args := call.Arguments
	if args == nil {
		args = map[string]any{}
	}
	switch name {
	case ToolSpawn:
		return svc.spawn(ctx, agentID, args)
	case ToolDelegate:
		return svc.delegate(ctx, agentID, args)
	case ToolTeamTasks:
		return svc.tasks(agentID, args)
	default:
		return "", fmt.Errorf("unknown orchestration tool %q", call.Name)
	}
}

func argString(args map[string]any, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func asJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func (svc *Service) spawn(ctx context.Context, agentID string, args map[string]any) (string, error) {
	parent, err := svc.Store.GetAgent(agentID)
	if err != nil {
		return "", err
	}
	child, err := svc.Store.CreateAgent(store.Agent{
		TenantID:     parent.TenantID,
		AgentKey:     parent.AgentKey + "-spawn-" + fmt.Sprintf("%d", time.Now().UnixNano()),
		DisplayName:  parent.DisplayName,
		Model:        parent.Model,
		Instructions: parent.Instructions,
	})
	if err != nil {
		return "", err
	}
	sess, err := svc.Store.CreateSession(store.Session{AgentID: child.ID, TenantID: child.TenantID, Label: "spawn"})
	if err != nil {
		return "", err
	}
	out := map[string]any{"agent_id": child.ID, "session_id": sess.ID, "model": child.Model}
	msg := argString(args, "message")
	if msg != "" && svc.Chat != nil {
		cctx, cancel := withTimeout(ctx)
		reply, err := svc.Chat(cctx, sess.ID, msg)
		cancel()
		if err != nil {
			out["error"] = err.Error()
		} else {
			out["reply"] = reply
		}
	}
	return asJSON(out), nil
}

func (svc *Service) delegate(ctx context.Context, agentID string, args map[string]any) (string, error) {
	toID := argString(args, "to_agent_id")
	msg := argString(args, "message")
	if toID == "" || msg == "" {
		return "", errors.New("to_agent_id and message are required")
	}
	target, err := svc.sameTenantAgent(agentID, toID)
	if err != nil {
		return "", err
	}
	toID = target.ID
	if err := svc.linkAllows(agentID, toID); err != nil {
		return "", err
	}
	mode := strings.ToLower(argString(args, "mode"))
	if mode == "" {
		mode = "sync"
	}
	if mode == "async" {
		id, kind := svc.enqueueAsync(agentID, toID, target.TenantID, msg)
		return asJSON(map[string]any{"id": id, "kind": kind, "mode": "async"}), nil
	}
	if n := depth(ctx); n >= 2 {
		return "", errors.New("delegation depth exceeded")
	}
	sess, err := svc.Store.CreateSession(store.Session{AgentID: toID, TenantID: target.TenantID, Label: "delegate"})
	if err != nil {
		return "", err
	}
	if svc.Chat == nil {
		return asJSON(map[string]any{"session_id": sess.ID, "mode": "sync", "reply": ""}), nil
	}
	cctx, cancel := withTimeout(ctx)
	defer cancel()
	reply, err := svc.Chat(withDepth(cctx), sess.ID, msg)
	if err != nil {
		return "", err
	}
	return asJSON(map[string]any{"session_id": sess.ID, "mode": "sync", "reply": reply}), nil
}

// sameTenantAgent returns the target agent when it lives in the caller's
// tenant. The target is an agent id, or an agent_key looked up inside the
// caller's tenant only. An agent in another tenant answers "not found", the
// same as a missing one, so a caller cannot probe other tenants' agent ids.
func (svc *Service) sameTenantAgent(callerID, targetID string) (*store.Agent, error) {
	notFound := errors.New("target agent not found")
	caller, err := svc.Store.GetAgent(callerID)
	if err != nil || caller == nil {
		return nil, notFound
	}
	target, err := svc.Store.GetAgent(targetID)
	if err != nil || target == nil {
		target = store.FindAgentByKey(svc.Store, caller.TenantID, targetID)
	}
	if target == nil || !store.SameTenant(caller.TenantID, target.TenantID) {
		return nil, notFound
	}
	return target, nil
}

// linkAllows applies agent links (GoClaw agent_links): an agent with at
// least one outgoing link may delegate only to the agents it links to. An
// agent with no links may delegate to any agent of its tenant. Delegating
// never creates a link; links are managed through the agent links API.
func (svc *Service) linkAllows(fromID, toID string) error {
	links, err := svc.Store.ListAgentLinks(fromID)
	if err != nil || len(links) == 0 {
		return nil
	}
	if svc.Store.HasAgentLink(fromID, toID) {
		return nil
	}
	return errors.New("target agent is not linked: this agent may delegate only to its linked agents")
}

func (svc *Service) enqueueAsync(fromID, toID, tenantID, msg string) (id, kind string) {
	if tm, err := svc.Store.TeamOfAgent(fromID); err == nil && tm != nil {
		if other, err := svc.Store.TeamOfAgent(toID); err == nil && other != nil && other.ID == tm.ID {
			m, err := svc.Store.CreateTeamMessage(store.TeamMessage{TeamID: tm.ID, FromAgentID: fromID, Body: msg})
			if err == nil {
				return m.ID, "mailbox"
			}
		}
		task, err := svc.Store.CreateTeamTask(store.TeamTask{
			TeamID: tm.ID, Title: msg, Status: "todo", AssigneeAgentID: toID,
		})
		if err == nil {
			return task.ID, "task"
		}
	}
	sess, err := svc.Store.CreateSession(store.Session{AgentID: toID, TenantID: tenantID, Label: "delegate-async"})
	if err != nil {
		return "", "error"
	}
	m, err := svc.Store.AddMessage(store.Message{SessionID: sess.ID, Role: "user", Content: msg})
	if err != nil {
		return sess.ID, "session"
	}
	return m.ID, "message"
}

func (svc *Service) tasks(agentID string, args map[string]any) (string, error) {
	tm, err := svc.Store.TeamOfAgent(agentID)
	if err != nil || tm == nil {
		return "", errors.New("agent is not on a team")
	}
	action := strings.ToLower(argString(args, "action"))
	if action == "" {
		action = "list"
	}
	switch action {
	case "list":
		list, err := svc.Store.ListTeamTasks(tm.ID, argString(args, "status"))
		if err != nil {
			return "", err
		}
		if list == nil {
			list = []*store.TeamTask{}
		}
		return asJSON(map[string]any{"tasks": list}), nil
	case "create":
		if err := svc.assigneeOK(agentID, argString(args, "assignee_agent_id")); err != nil {
			return "", err
		}
		task, err := svc.Store.CreateTeamTask(store.TeamTask{
			TeamID:          tm.ID,
			Title:           argString(args, "title"),
			Status:          argString(args, "status"),
			AssigneeAgentID: argString(args, "assignee_agent_id"),
		})
		if err != nil {
			return "", err
		}
		return asJSON(task), nil
	case "update":
		id := argString(args, "id")
		if id == "" {
			return "", errors.New("id is required")
		}
		// Only a task of the caller's own team can be changed.
		if cur, err := svc.Store.GetTeamTask(id); err != nil || cur == nil || cur.TeamID != tm.ID {
			return "", errors.New("task not found")
		}
		if err := svc.assigneeOK(agentID, argString(args, "assignee_agent_id")); err != nil {
			return "", err
		}
		task, err := svc.Store.UpdateTeamTask(store.TeamTask{
			ID:              id,
			Title:           argString(args, "title"),
			Status:          argString(args, "status"),
			AssigneeAgentID: argString(args, "assignee_agent_id"),
		})
		if err != nil {
			return "", err
		}
		return asJSON(task), nil
	default:
		return "", fmt.Errorf("unknown action %q", action)
	}
}

// assigneeOK refuses a task assignee from another tenant. Empty means
// unassigned and is allowed.
func (svc *Service) assigneeOK(callerID, assigneeID string) error {
	if strings.TrimSpace(assigneeID) == "" {
		return nil
	}
	if _, err := svc.sameTenantAgent(callerID, assigneeID); err != nil {
		return errors.New("assignee agent not found")
	}
	return nil
}

func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, SyncTimeout)
}

func withDepth(ctx context.Context) context.Context {
	return context.WithValue(ctx, depthKey{}, depth(ctx)+1)
}

func depth(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	if n, ok := ctx.Value(depthKey{}).(int); ok {
		return n
	}
	return 0
}
