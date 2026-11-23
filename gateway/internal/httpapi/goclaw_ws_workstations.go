package httpapi

import (
	"context"
	"time"
	"encoding/json"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/workstation"
)

func wsWorkstationView(w *store.Workstation) map[string]any {
	if w == nil {
		return map[string]any{}
	}
	meta := map[string]any{}
	if len(w.Metadata) > 0 {
		_ = json.Unmarshal(w.Metadata, &meta)
	}
	return map[string]any{
		"id": w.ID, "workstation_key": w.Key, "name": w.Name, "display": w.Name,
		"backend_type": w.BackendType, "backend": w.BackendType,
		"default_cwd": w.DefaultCWD, "active": w.Active,
		"created_by": w.CreatedBy, "created_at": w.CreatedAt, "updated_at": w.UpdatedAt,
		"agent_ids": w.AgentIDs, "permission_count": w.Permissions,
		"host": meta["host"], "port": meta["port"], "user": meta["user"],
		"identity_ref": meta["identity_ref"], "metadata": meta,
	}
}

func handleGoClawWorkstations(conn *websocket.Conn, st store.StoreIface, req goclawReq, who *auth.Identity) error {
	x := store.Workstations(st)
	tenant := tenantOfIdentity(who)
	if x == nil {
		return writeGoClawErr(conn, req.ID, "NOT_IMPLEMENTED", "workstation store unavailable")
	}
	switch req.Method {
	case "workstations.list":
		rows := x.ListWorkstations(tenant)
		out := make([]map[string]any, 0, len(rows))
		for _, w := range rows {
			out = append(out, wsWorkstationView(w))
		}
		return writeGoClaw(conn, req.ID, map[string]any{"workstations": out, "total": len(out)})

	case "workstations.get":
		var p struct {
			ID  string `json:"id"`
			Key string `json:"workstationId"`
		}
		_ = json.Unmarshal(req.Params, &p)
		w, err := x.GetWorkstation(tenant, firstNonEmpty(p.ID, p.Key))
		if err != nil || w == nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "workstation not found")
		}
		return writeGoClaw(conn, req.ID, map[string]any{"workstation": wsWorkstationView(w)})

	case "workstations.create":
		var p struct {
			Key      string `json:"workstation_key"`
			Name     string `json:"name"`
			Display  string `json:"display"`
			Backend  string `json:"backend"`
			Backend2 string `json:"backend_type"`
			Host     string `json:"host"`
			Port     int    `json:"port"`
			User     string `json:"user"`
			Identity string `json:"identity_ref"`
			CWD      string `json:"default_cwd"`
			AgentID  string `json:"agent_id"`
			Active   *bool  `json:"active"`
		}
		_ = json.Unmarshal(req.Params, &p)
		name := firstNonEmpty(p.Name, p.Display)
		key := firstNonEmpty(p.Key, slugifyKey(name))
		if key == "" {
			return writeGoClawErr(conn, req.ID, "INVALID", "name or workstation_key is required")
		}
		meta, _ := json.Marshal(map[string]any{
			"host": strings.TrimSpace(p.Host), "port": p.Port,
			"user": strings.TrimSpace(p.User), "identity_ref": strings.TrimSpace(p.Identity),
		})
		active := true
		if p.Active != nil {
			active = *p.Active
		}
		w, err := x.PutWorkstation(store.Workstation{
			TenantID: tenant, Key: key, Name: name,
			BackendType: firstNonEmpty(p.Backend, p.Backend2), DefaultCWD: p.CWD,
			Metadata: meta, Active: active, CreatedBy: identityLabel(who),
		})
		if err != nil {
			return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
		}
		if strings.TrimSpace(p.AgentID) != "" {
			_ = x.LinkWorkstationAgent(tenant, w.ID, p.AgentID)
			w.AgentIDs = []string{p.AgentID}
		}
		return writeGoClaw(conn, req.ID, map[string]any{"workstation": wsWorkstationView(w)})

	case "workstations.update":
		var p struct {
			ID       string  `json:"id"`
			Key      string  `json:"workstation_key"`
			Name     *string `json:"name"`
			Display  *string `json:"display"`
			Backend  *string `json:"backend"`
			CWD      *string `json:"default_cwd"`
			Active   *bool   `json:"active"`
			Host     *string `json:"host"`
			Port     *int    `json:"port"`
			User     *string `json:"user"`
			Identity *string `json:"identity_ref"`
			AgentID  *string `json:"agent_id"`
		}
		_ = json.Unmarshal(req.Params, &p)
		w, err := x.GetWorkstation(tenant, firstNonEmpty(p.ID, p.Key))
		if err != nil || w == nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "workstation not found")
		}
		if p.Name != nil {
			w.Name = strings.TrimSpace(*p.Name)
		}
		if p.Display != nil {
			w.Name = strings.TrimSpace(*p.Display)
		}
		if p.Backend != nil {
			w.BackendType = strings.TrimSpace(*p.Backend)
		}
		if p.CWD != nil {
			w.DefaultCWD = strings.TrimSpace(*p.CWD)
		}
		if p.Active != nil {
			w.Active = *p.Active
		}
		meta := map[string]any{}
		_ = json.Unmarshal(w.Metadata, &meta)
		if p.Host != nil {
			meta["host"] = strings.TrimSpace(*p.Host)
		}
		if p.Port != nil {
			meta["port"] = *p.Port
		}
		if p.User != nil {
			meta["user"] = strings.TrimSpace(*p.User)
		}
		if p.Identity != nil {
			meta["identity_ref"] = strings.TrimSpace(*p.Identity)
		}
		raw, _ := json.Marshal(meta)
		w.Metadata = raw
		saved, err := x.PutWorkstation(*w)
		if err != nil {
			return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
		}
		if p.AgentID != nil && strings.TrimSpace(*p.AgentID) != "" {
			_ = x.LinkWorkstationAgent(tenant, saved.ID, strings.TrimSpace(*p.AgentID))
			saved.AgentIDs = []string{strings.TrimSpace(*p.AgentID)}
		}
		return writeGoClaw(conn, req.ID, map[string]any{"workstation": wsWorkstationView(saved)})

	case "workstations.delete":
		var p struct {
			ID  string `json:"id"`
			Key string `json:"workstation_key"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if err := x.DeleteWorkstation(tenant, firstNonEmpty(p.ID, p.Key)); err != nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "workstation not found")
		}
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true})

	case "workstations.activity.list":
		var p struct {
			ID  string `json:"workstationId"`
			ID2 string `json:"id"`
			Lim int    `json:"limit"`
		}
		_ = json.Unmarshal(req.Params, &p)
		w, err := x.GetWorkstation(tenant, firstNonEmpty(p.ID, p.ID2))
		if err != nil || w == nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "workstation not found")
		}
		rows := x.ListWorkstationActivity(tenant, w.ID, p.Lim)
		return writeGoClaw(conn, req.ID, map[string]any{"activity": rows, "total": len(rows)})

	case "workstations.permissions.list":
		var p struct {
			ID  string `json:"workstationId"`
			ID2 string `json:"id"`
		}
		_ = json.Unmarshal(req.Params, &p)
		w, err := x.GetWorkstation(tenant, firstNonEmpty(p.ID, p.ID2))
		if err != nil || w == nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "workstation not found")
		}
		rows := x.ListWorkstationPermissions(tenant, w.ID)
		return writeGoClaw(conn, req.ID, map[string]any{"permissions": rows, "total": len(rows)})

	case "workstations.permissions.add":
		var p struct {
			ID      string `json:"workstationId"`
			ID2     string `json:"id"`
			Pattern string `json:"pattern"`
			Enabled *bool  `json:"enabled"`
		}
		_ = json.Unmarshal(req.Params, &p)
		w, err := x.GetWorkstation(tenant, firstNonEmpty(p.ID, p.ID2))
		if err != nil || w == nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "workstation not found")
		}
		enabled := true
		if p.Enabled != nil {
			enabled = *p.Enabled
		}
		perm, err := x.PutWorkstationPermission(store.WorkstationPermission{
			TenantID: tenant, WorkstationID: w.ID, Pattern: p.Pattern, Enabled: enabled, CreatedBy: identityLabel(who),
		})
		if err != nil {
			return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
		}
		return writeGoClaw(conn, req.ID, map[string]any{"permission": perm})

	case "workstations.permissions.remove":
		var p struct {
			ID      string `json:"id"`
			PermID  string `json:"permissionId"`
			Pattern string `json:"pattern"`
		}
		_ = json.Unmarshal(req.Params, &p)
		target := firstNonEmpty(p.PermID, p.ID)
		if target == "" && p.Pattern != "" {
			for _, perm := range x.ListWorkstationPermissions(tenant, "") {
				if perm.Pattern == p.Pattern {
					target = perm.ID
				}
			}
		}
		if err := x.DeleteWorkstationPermission(tenant, target); err != nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "permission not found")
		}
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true})

	case "workstations.permissions.toggle":
		var p struct {
			ID      string `json:"id"`
			PermID  string `json:"permissionId"`
			Enabled *bool  `json:"enabled"`
		}
		_ = json.Unmarshal(req.Params, &p)
		target := firstNonEmpty(p.PermID, p.ID)
		enabled := true
		if p.Enabled != nil {
			enabled = *p.Enabled
		}
		if err := x.SetWorkstationPermissionEnabled(tenant, target, enabled); err != nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "permission not found")
		}
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true, "enabled": enabled})

	case "workstations.link_agent":
		var p struct {
			ID      string `json:"workstationId"`
			ID2     string `json:"id"`
			AgentID string `json:"agentId"`
			Agent2  string `json:"agent_id"`
		}
		_ = json.Unmarshal(req.Params, &p)
		w, err := x.GetWorkstation(tenant, firstNonEmpty(p.ID, p.ID2))
		if err != nil || w == nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "workstation not found")
		}
		agent := firstNonEmpty(p.AgentID, p.Agent2)
		if agent == "" {
			return writeGoClawErr(conn, req.ID, "INVALID", "agentId is required")
		}
		if err := x.LinkWorkstationAgent(tenant, w.ID, agent); err != nil {
			return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
		}
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true, "agent_id": agent})

	case "workstations.unlink_agent":
		var p struct {
			ID      string `json:"workstationId"`
			ID2     string `json:"id"`
			AgentID string `json:"agentId"`
			Agent2  string `json:"agent_id"`
		}
		_ = json.Unmarshal(req.Params, &p)
		w, err := x.GetWorkstation(tenant, firstNonEmpty(p.ID, p.ID2))
		if err != nil || w == nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "workstation not found")
		}
		if err := x.UnlinkWorkstationAgent(tenant, w.ID, firstNonEmpty(p.AgentID, p.Agent2)); err != nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "link not found")
		}
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true})

	case "workstations.test":
		var p struct {
			ID  string `json:"id"`
			ID2 string `json:"workstationId"`
		}
		_ = json.Unmarshal(req.Params, &p)
		w, err := x.GetWorkstation(tenant, firstNonEmpty(p.ID, p.ID2))
		if err != nil || w == nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "workstation not found")
		}
		res, rerr := workstation.Run(timeoutCtx(15), workstation.ExecRequest{
			Workstation: w, Command: "echo", Args: []string{"goso-probe"}, Timeout: 15 * time.Second,
		})
		if rerr != nil {
			return writeGoClaw(conn, req.ID, map[string]any{
				"ok": false, "connected": false, "backend_type": w.BackendType,
				"message": rerr.Error(),
			})
		}
		return writeGoClaw(conn, req.ID, map[string]any{
			"ok": res.OK, "connected": res.OK, "backend_type": res.Backend,
			"output": strings.TrimSpace(res.Stdout), "duration_ms": res.DurationMS,
		})

	default:
		return writeGoClawErr(conn, req.ID, "NOT_IMPLEMENTED", req.Method)
	}
}

func slugifyKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func identityLabel(who *auth.Identity) string {
	if who == nil {
		return "system"
	}
	return firstNonEmpty(who.Email, who.UserID, "system")
}

func timeoutCtx(sec int) context.Context {
	ctx, _ := context.WithTimeout(context.Background(), time.Duration(sec)*time.Second)
	return ctx
}
