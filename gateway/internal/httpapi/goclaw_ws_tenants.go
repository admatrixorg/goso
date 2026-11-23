package httpapi

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/tenant"
)

func handleGoClawTenants(conn *websocket.Conn, req goclawReq, who *auth.Identity) error {
	reg := tenant.DefaultRegistry()
	switch req.Method {
	case "tenants.list", "tenants.mine":
		rows := make([]map[string]any, 0)
		if reg != nil {
			for _, p := range reg.List("") {
				if !tenantVisible(who, p.ID) {
					continue
				}
				role := memberRole(who, p)
				rows = append(rows, goclawTenantView(p, role))
			}
		}
		return writeGoClaw(conn, req.ID, map[string]any{"tenants": rows})
	case "tenants.get":
		var p struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if reg == nil || !tenantVisible(who, p.ID) {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "tenant not found")
		}
		row, err := reg.Get(p.ID)
		if err != nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "tenant not found")
		}
		return writeGoClaw(conn, req.ID, goclawTenantView(row, memberRole(who, row)))
	case "tenants.create":
		if !isOwnerIdentity(who) {
			return writeGoClawErr(conn, req.ID, "FORBIDDEN", "owner role required")
		}
		var p struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		}
		_ = json.Unmarshal(req.Params, &p)
		if reg == nil {
			return writeGoClawErr(conn, req.ID, "UNAVAILABLE", "tenant registry unavailable")
		}
		row, err := reg.Create(p.Slug, p.Name)
		if err != nil {
			return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
		}
		return writeGoClaw(conn, req.ID, goclawTenantView(row, tenant.RoleOwner))
	case "tenants.users.list":
		return wsTenantUsersList(conn, reg, req, who)
	case "tenants.users.add":
		return wsTenantUsersAdd(conn, reg, req, who)
	case "tenants.users.remove":
		return wsTenantUsersRemove(conn, reg, req, who)
	default:
		return writeGoClawErr(conn, req.ID, "NOT_IMPLEMENTED", req.Method+" is not implemented on GOSO WS")
	}
}

func isOwnerIdentity(who *auth.Identity) bool {
	return who == nil || who.IsOwner()
}

// memberSubjectMatches reports the membership row that belongs to who.
func memberSubjectMatches(who *auth.Identity, m tenant.Member) bool {
	if who == nil {
		return false
	}
	if strings.TrimSpace(who.Email) != "" && strings.EqualFold(m.Subject, who.Email) {
		return true
	}
	return who.UserID != "" && m.Subject == who.UserID
}

// memberRole returns the signed-in user's role in a tenant, "owner" for an
// operator token, and "member" for a tenant they were not granted.
func memberRole(who *auth.Identity, p tenant.Public) string {
	if isOwnerIdentity(who) {
		return tenant.RoleOwner
	}
	reg := tenant.DefaultRegistry()
	if reg != nil {
		if row, err := reg.Get(p.ID); err == nil {
			p = row
		}
	}
	for _, m := range p.Members {
		if memberSubjectMatches(who, m) && strings.TrimSpace(m.Role) != "" {
			return m.Role
		}
	}
	return "member"
}

// tenantVisible reports whether who may see a tenant. An owner sees every
// tenant; someone else sees only tenants where they hold a membership.
func tenantVisible(who *auth.Identity, tenantID string) bool {
	if isOwnerIdentity(who) {
		return true
	}
	reg := tenant.DefaultRegistry()
	if reg == nil {
		return false
	}
	row, err := reg.Get(tenantID)
	if err != nil {
		return false
	}
	for _, m := range row.Members {
		if memberSubjectMatches(who, m) {
			return true
		}
	}
	return false
}

func canManageTenantUsers(who *auth.Identity, tenantID string) bool {
	if isOwnerIdentity(who) {
		return true
	}
	reg := tenant.DefaultRegistry()
	if reg == nil {
		return false
	}
	row, err := reg.Get(tenantID)
	if err != nil {
		return false
	}
	for _, m := range row.Members {
		if memberSubjectMatches(who, m) && (m.Role == tenant.RoleOwner || m.Role == tenant.RoleAdmin) {
			return true
		}
	}
	return false
}

func goclawTenantView(p tenant.Public, role string) map[string]any {
	created := ""
	if !p.CreatedAt.IsZero() {
		created = p.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	return map[string]any{
		"id": p.ID, "name": p.Name, "slug": p.ID, "status": p.Status, "role": role,
		"created_at": created, "updated_at": created,
	}
}

func wsMemberView(tenantID string, m tenant.Member) map[string]any {
	created := ""
	if !m.CreatedAt.IsZero() {
		created = m.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	return map[string]any{
		"id": m.ID, "tenant_id": tenantID, "user_id": m.Subject,
		"display_name": firstNonEmpty(gosoUserDisplay(m.Subject, tenantID), m.Subject),
		"email":        gosoUserLookup(m.Subject, tenantID).Email,
		"role":         m.Role, "created_at": created, "updated_at": created,
	}
}

func wsTenantUsersList(conn *websocket.Conn, reg *tenant.Registry, req goclawReq, who *auth.Identity) error {
	var p struct {
		TenantID string `json:"tenant_id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	tid := strings.TrimSpace(p.TenantID)
	if reg == nil || !tenantVisible(who, tid) {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "tenant not found")
	}
	row, err := reg.Get(tid)
	if err != nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "tenant not found")
	}
	users := make([]map[string]any, 0, len(row.Members))
	for _, m := range row.Members {
		users = append(users, wsMemberView(row.ID, m))
	}
	return writeGoClaw(conn, req.ID, map[string]any{"users": users})
}

func wsTenantUsersAdd(conn *websocket.Conn, reg *tenant.Registry, req goclawReq, who *auth.Identity) error {
	var p struct {
		TenantID string `json:"tenant_id"`
		UserID   string `json:"user_id"`
		Role     string `json:"role"`
	}
	_ = json.Unmarshal(req.Params, &p)
	if reg == nil {
		return writeGoClawErr(conn, req.ID, "UNAVAILABLE", "tenant registry unavailable")
	}
	if !canManageTenantUsers(who, p.TenantID) {
		return writeGoClawErr(conn, req.ID, "FORBIDDEN", "admin role required")
	}
	role := strings.ToLower(strings.TrimSpace(p.Role))
	if role == "operator" {
		role = tenant.RoleAdmin
	}
	_, mem, err := reg.AddMember(p.TenantID, p.UserID, role)
	if err != nil {
		return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
	}
	return writeGoClaw(conn, req.ID, wsMemberView(p.TenantID, mem))
}

func wsTenantUsersRemove(conn *websocket.Conn, reg *tenant.Registry, req goclawReq, who *auth.Identity) error {
	var p struct {
		TenantID string `json:"tenant_id"`
		UserID   string `json:"user_id"`
	}
	_ = json.Unmarshal(req.Params, &p)
	if reg == nil {
		return writeGoClawErr(conn, req.ID, "UNAVAILABLE", "tenant registry unavailable")
	}
	if !canManageTenantUsers(who, p.TenantID) {
		return writeGoClawErr(conn, req.ID, "FORBIDDEN", "admin role required")
	}
	row, err := reg.Get(p.TenantID)
	if err != nil {
		return writeGoClawErr(conn, req.ID, "NOT_FOUND", "tenant not found")
	}
	want := strings.TrimSpace(p.UserID)
	for _, m := range row.Members {
		if m.Subject == want || m.ID == want {
			if _, _, err := reg.RemoveMember(p.TenantID, m.ID, m.Subject); err != nil {
				return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
			}
			return writeGoClaw(conn, req.ID, map[string]any{"ok": true})
		}
	}
	return writeGoClawErr(conn, req.ID, "NOT_FOUND", "user not found")
}
