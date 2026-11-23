package httpapi

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mqglobal/goso/gateway/internal/cron"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func handleGoClawExtra(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	switch req.Method {
	case "agents.files.list":
		return wsFilesList(conn, st, req)
	case "agents.files.get":
		return wsFilesGet(conn, st, req)
	case "agents.files.set":
		return wsFilesSet(conn, st, req)
	case "cron.list":
		return wsCronList(conn, st, req)
	case "cron.create":
		return wsCronCreate(conn, st, req)
	case "cron.toggle":
		return wsCronToggle(conn, st, req)
	case "cron.delete":
		return wsCronDelete(conn, st, req)
	case "cron.run":
		return wsCronRun(conn, st, req)
	case "cron.runs":
		return writeGoClaw(conn, req.ID, map[string]any{"entries": []any{}, "total": 0})
	case "channels.status":
		return wsChannelsStatus(conn, st, req)
	case "heartbeat.get":
		return writeGoClaw(conn, req.ID, map[string]any{"heartbeat": nil})
	case "heartbeat.set", "heartbeat.toggle", "heartbeat.test", "heartbeat.checklist.set":
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
	case "heartbeat.logs":
		return writeGoClaw(conn, req.ID, map[string]any{"logs": []any{}})
	case "heartbeat.checklist.get":
		return writeGoClaw(conn, req.ID, map[string]any{"content": ""})
	case "heartbeat.targets":
		return writeGoClaw(conn, req.ID, map[string]any{"targets": []any{}})
	case "config.permissions.list":
		return writeGoClaw(conn, req.ID, map[string]any{"permissions": []any{}})
	case "config.permissions.grant", "config.permissions.revoke":
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
	case "config.permissions.check":
		return writeGoClaw(conn, req.ID, map[string]any{"allowed": true})
	default:
		return writeGoClawErr(conn, req.ID, "NOT_IMPLEMENTED", req.Method+" is not implemented on GOSO WS")
	}
}

func wsFindAgent(st store.StoreIface, id string) *store.Agent {
	id = strings.TrimSpace(id)
	if st == nil || id == "" {
		return nil
	}
	for _, a := range st.ListAgents() {
		if a == nil {
			continue
		}
		if a.ID == id || a.AgentKey == id {
			return a
		}
	}
	return nil
}

func wsFilesList(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		AgentID string `json:"agentId"`
	}
	_ = json.Unmarshal(req.Params, &p)
	ag := wsFindAgent(st, p.AgentID)
	files := []map[string]any{}
	if ag != nil && st != nil {
		listed, err := st.ListContextFiles(ag.TenantID, store.ContextScopeAgent, ag.ID)
		if err != nil || len(listed) == 0 {
			listed, _ = st.ListContextFiles(ag.TenantID, store.ContextScopeAgent, ag.AgentKey)
		}
		for _, f := range listed {
			if f == nil {
				continue
			}
			files = append(files, map[string]any{
				"name":        f.Name,
				"missing":     false,
				"size":        len(f.Body),
				"updatedAtMs": f.UpdatedAt.UnixMilli(),
			})
		}
	}
	return writeGoClaw(conn, req.ID, map[string]any{"files": files})
}

func wsFilesGet(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		AgentID string `json:"agentId"`
		Name    string `json:"name"`
	}
	_ = json.Unmarshal(req.Params, &p)
	ag := wsFindAgent(st, p.AgentID)
	if ag == nil || st == nil {
		return writeGoClaw(conn, req.ID, map[string]any{"file": map[string]any{"name": p.Name, "missing": true, "size": 0}})
	}
	f, err := st.GetContextFile(ag.TenantID, store.ContextScopeAgent, ag.ID, p.Name)
	if err != nil || f == nil {
		f, err = st.GetContextFile(ag.TenantID, store.ContextScopeAgent, ag.AgentKey, p.Name)
	}
	if err != nil || f == nil {
		return writeGoClaw(conn, req.ID, map[string]any{"file": map[string]any{"name": p.Name, "missing": true, "size": 0}})
	}
	return writeGoClaw(conn, req.ID, map[string]any{"file": map[string]any{
		"name": f.Name, "missing": false, "size": len(f.Body), "content": f.Body, "updatedAtMs": f.UpdatedAt.UnixMilli(),
	}})
}

func wsFilesSet(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		AgentID string `json:"agentId"`
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	_ = json.Unmarshal(req.Params, &p)
	ag := wsFindAgent(st, p.AgentID)
	if ag == nil || st == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "agent not found")
	}
	_, err := st.PutContextFile(store.ContextFile{
		TenantID: ag.TenantID, Scope: store.ContextScopeAgent, OwnerID: ag.ID, Name: p.Name, Body: p.Content,
	})
	if err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
}

func wsCronUI(j *store.CronJob) map[string]any {
	if j == nil {
		return map[string]any{}
	}
	name := j.Message
	if len(name) > 80 {
		name = name[:80]
	}
	out := map[string]any{
		"id":          j.ID,
		"name":        name,
		"enabled":     j.Enabled,
		"schedule":    map[string]any{"kind": "cron", "expr": j.Spec},
		"payload":     map[string]any{"kind": "agentTurn", "message": j.Message},
		"createdAtMs": 0,
		"updatedAtMs": 0,
		"state":       map[string]any{},
	}
	if j.LastRun != nil {
		out["state"] = map[string]any{"lastRunAtMs": j.LastRun.UnixMilli(), "lastStatus": "ok"}
	}
	if strings.TrimSpace(j.LastError) != "" {
		out["state"] = map[string]any{"lastError": j.LastError, "lastStatus": "error"}
	}
	return out
}

func wsCronList(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	jobs := []map[string]any{}
	if st != nil {
		for _, j := range st.ListCronJobs() {
			jobs = append(jobs, wsCronUI(j))
		}
	}
	return writeGoClaw(conn, req.ID, map[string]any{"jobs": jobs})
}

func wsCronCreate(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		Name     string `json:"name"`
		Message  string `json:"message"`
		AgentID  string `json:"agentId"`
		Enabled  *bool  `json:"enabled"`
		Schedule struct {
			Kind    string `json:"kind"`
			Expr    string `json:"expr"`
			EveryMs int64  `json:"everyMs"`
		} `json:"schedule"`
	}
	_ = json.Unmarshal(req.Params, &p)
	msg := strings.TrimSpace(p.Message)
	if msg == "" {
		msg = strings.TrimSpace(p.Name)
	}
	spec := strings.TrimSpace(p.Schedule.Expr)
	if spec == "" && p.Schedule.EveryMs > 0 {
		spec = "every:" + time.Duration(p.Schedule.EveryMs*int64(time.Millisecond)).String()
	}
	if spec == "" {
		spec = "every:1h"
	}
	ag := wsFindAgent(st, p.AgentID)
	if ag == nil || st == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "agent not found")
	}
	sess, err := st.CreateSession(store.Session{TenantID: ag.TenantID, AgentID: ag.ID, Label: "cron:" + p.Name})
	if err != nil || sess == nil {
		return writeGoClawErr(conn, req.ID, "INVALID", "session create failed")
	}
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	job, err := st.CreateCronJob(store.CronJob{Spec: spec, SessionID: sess.ID, Message: msg, Enabled: enabled})
	if err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, wsCronUI(job))
}

func wsCronToggle(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		JobID   string `json:"jobId"`
		Enabled bool   `json:"enabled"`
	}
	_ = json.Unmarshal(req.Params, &p)
	if st == nil {
		return writeGoClawErr(conn, req.ID, "UNAVAILABLE", "store unavailable")
	}
	if err := st.SetCronEnabled(p.JobID, p.Enabled); err != nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "job not found")
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
}

func wsCronDelete(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		JobID string `json:"jobId"`
	}
	_ = json.Unmarshal(req.Params, &p)
	if st == nil {
		return writeGoClawErr(conn, req.ID, "UNAVAILABLE", "store unavailable")
	}
	if err := st.DeleteCronJob(p.JobID); err != nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "job not found")
	}
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
}

func wsCronRun(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	var p struct {
		JobID  string `json:"jobId"`
		JobID2 string `json:"id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	id := firstNonEmpty(p.JobID, p.JobID2)
	if st == nil || id == "" {
		return writeGoClawErr(conn, req.ID, "INVALID", "job is required")
	}
	job, err := st.GetCronJob(id)
	if err != nil || job == nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "job not found")
	}
	fire := FireSessionChat(wsD.Runtime, st, wsD.Provider, wsD.Meter)
	if err := cron.StartJob(st, job, fire); err != nil {
		if errors.Is(err, cron.ErrRunning) {
			return writeGoClawErr(conn, req.ID, "CONFLICT", "job already running")
		}
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	_ = st.MarkCronRun(id, time.Now().UTC())
	return writeGoClaw(conn, req.ID, map[string]any{"ok": true, "ran": true})
}

func wsChannelsStatus(conn *websocket.Conn, st store.StoreIface, req goclawReq) error {
	out := map[string]any{}
	if st != nil {
		for _, cfg := range st.ListChannelConfigs() {
			if cfg == nil || strings.TrimSpace(cfg.Name) == "" {
				continue
			}
			state := "stopped"
			if cfg.Enabled {
				state = "registered"
			}
			out[cfg.Name] = map[string]any{
				"enabled": cfg.Enabled,
				"running": false,
				"state":   state,
				"summary": cfg.Name,
			}
		}
	}
	return writeGoClaw(conn, req.ID, map[string]any{"channels": out})
}
