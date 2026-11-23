package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/tenant"
)

type gosoUserRec struct {
	Slug        string `json:"slug"`
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Email       string `json:"email"`
	Status      string `json:"status"`
}

var (
	gosoUsersOnce sync.Once
	gosoUsers     []gosoUserRec
)

func loadGosoUsers() []gosoUserRec {
	gosoUsersOnce.Do(func() {
		paths := []string{
			strings.TrimSpace(os.Getenv("GOSO_USERS_FILE")),
			"/data/goso_users.json",
			"/opt/omni/goso-full-deploy/goso_users.json",
		}
		for _, p := range paths {
			if p == "" {
				continue
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var recs []gosoUserRec
			if json.Unmarshal(raw, &recs) == nil && len(recs) > 0 {
				gosoUsers = recs
				return
			}
		}
	})
	return gosoUsers
}

func gosoUserLookup(userID, tenantID string) gosoUserRec {
	userID = strings.TrimSpace(userID)
	var fallback gosoUserRec
	for _, u := range loadGosoUsers() {
		if u.UserID != userID {
			continue
		}
		if tenantID == "" || u.Slug == tenantID {
			return u
		}
		if fallback.UserID == "" {
			fallback = u
		}
	}
	return fallback
}

func gosoUserDisplay(userID, tenantID string) string {
	u := gosoUserLookup(userID, tenantID)
	if n := strings.TrimSpace(u.DisplayName); n != "" {
		return n
	}
	if e := strings.TrimSpace(u.Email); e != "" {
		return e
	}
	return ""
}

// registerUserDirectoryRoutes serves GET|POST /api/users (alias /v1/users),
// GET|PATCH|DELETE /api/users/{id} and GET /api/tenant-users.
func registerUserDirectoryRoutes(mux *http.ServeMux, st store.StoreIface) {
	aliasAPI(mux, "GET /api/users", handleListDirectoryUsers(st))
	aliasAPI(mux, "POST /api/users", handleCreateDirectoryUser(st))
	aliasAPI(mux, "GET /api/users/{id}", handleGetDirectoryUser(st))
	aliasAPI(mux, "PATCH /api/users/{id}", handlePatchDirectoryUser(st))
	aliasAPI(mux, "DELETE /api/users/{id}", handleDeleteDirectoryUser(st))
	mux.HandleFunc("GET /v1/tenant-users", handleGoClawTenantUsers(st))
}

func userPublicView(u *store.AuthUser) map[string]any {
	out := map[string]any{
		"id":           u.ID,
		"user_id":      u.ID,
		"email":        u.Email,
		"display_name": u.DisplayName,
		"status":       u.Status,
		"created_at":   formatUserTime(u.CreatedAt),
		"updated_at":   formatUserTime(u.UpdatedAt),
	}
	return out
}

func formatUserTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func directoryMembership(u *store.AuthUser, slug string) map[string]any {
	label := u.DisplayName
	if strings.TrimSpace(label) == "" {
		label = u.Email
	}
	return map[string]any{"tenant_id": slug, "role": "member", "display_name": label}
}

func handleListDirectoryUsers(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tid := strings.TrimSpace(r.URL.Query().Get("tenant_id"))
		users := store.AuthUsers(st)
		if users == nil {
			writeJSON(w, http.StatusOK, map[string]any{"users": []any{}})
			return
		}
		list := users.ListAuthUsers()
		reg := tenant.DefaultRegistry()
		who, hasWho := auth.IdentityFrom(r.Context())
		restricted := hasWho && !who.IsOwner()
		allowed := map[string]bool{}
		if restricted && reg != nil {
			for _, t := range reg.List("") {
				row, gerr := reg.Get(t.ID)
				if gerr != nil {
					continue
				}
				for _, m := range row.Members {
					if (who.Email != "" && strings.EqualFold(m.Subject, who.Email)) || (who.UserID != "" && m.Subject == who.UserID) {
						allowed[row.ID] = true
					}
				}
			}
			if tid != "" && !allowed[tid] {
				writeErr(w, http.StatusForbidden, "forbidden")
				return
			}
		}
		out := []map[string]any{}
		for _, u := range list {
			view := userPublicView(u)
			memberships := []map[string]any{}
			if reg != nil {
				for _, t := range reg.List("") {
					if tid != "" && t.ID != tid {
						continue
					}
					if restricted && !allowed[t.ID] {
						continue
					}
					for _, m := range t.Members {
						if strings.EqualFold(m.Subject, u.Email) || m.Subject == u.ID {
							memberships = append(memberships, map[string]any{
								"tenant_id": t.ID, "role": m.Role,
								"display_name": firstNonEmpty(u.DisplayName, u.Email),
							})
						}
					}
				}
			}
			if len(memberships) == 0 {
				// Directory rows carry display names for members that were
				// imported without an account (see goso_users.json).
				for _, rec := range loadGosoUsers() {
					if restricted && !allowed[rec.Slug] {
						continue
					}
					if rec.Email != "" && strings.EqualFold(rec.Email, u.Email) && (tid == "" || rec.Slug == tid) {
						memberships = append(memberships, map[string]any{
							"tenant_id": rec.Slug, "role": rec.Role,
							"display_name": firstNonEmpty(rec.DisplayName, u.DisplayName, u.Email),
						})
					}
				}
			}
			if len(memberships) == 0 && (tid != "" || restricted) {
				continue
			}
			view["memberships"] = memberships
			out = append(out, view)
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": out})
	}
}

func handleGetDirectoryUser(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users := store.AuthUsers(st)
		if users == nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		u, err := users.GetAuthUser(r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, userPublicView(u))
	}
}

type createUserBody struct {
	Email       string `json:"email"`
	TenantID    string `json:"tenant_id"`
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

func handleCreateDirectoryUser(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users := store.AuthUsers(st)
		if users == nil {
			writeErr(w, http.StatusNotImplemented, "user store unavailable")
			return
		}
		var body createUserBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		email := strings.ToLower(strings.TrimSpace(body.Email))
		if email == "" {
			writeErr(w, http.StatusBadRequest, "email is required")
			return
		}
		password := body.Password
		generated := false
		if len(password) < 8 {
			if password != "" {
				writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
				return
			}
			password = randomPassword()
			generated = true
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "hash failed")
			return
		}
		u, err := users.CreateAuthUser(store.AuthUser{
			Email: email, PasswordHash: string(hash),
			DisplayName: strings.TrimSpace(body.DisplayName), Status: store.AuthUserActive,
		})
		if err != nil {
			if err == store.ErrAuthUserExists {
				writeJSON(w, http.StatusConflict, map[string]string{"code": "ALREADY_EXISTS", "error": "email already exists"})
				return
			}
			writeErr(w, http.StatusInternalServerError, "create failed")
			return
		}
		tid := strings.TrimSpace(body.TenantID)
		role := strings.ToLower(strings.TrimSpace(body.Role))
		if role == "" {
			role = tenant.RoleMember
		}
		if role == "operator" {
			role = tenant.RoleAdmin
		}
		if tid != "" {
			if reg := tenant.DefaultRegistry(); reg != nil {
				if _, _, err := reg.AddMember(tid, u.Email, role); err != nil && err != tenant.ErrMemberExists && err != tenant.ErrNotFound {
					writeErr(w, http.StatusBadRequest, err.Error())
					return
				}
			}
			if store.AuthUsers(st) != nil {
				if f, ok := st.(interface {
					PutImportSource(string, string, string, string) error
				}); ok && f != nil {
					_ = f.PutImportSource(tid, "auth_user", u.Email, u.ID)
				}
			}
		}
		resp := map[string]any{"user": userPublicView(u)}
		if generated {
			resp["one_time_password"] = password
		}
		writeJSON(w, http.StatusCreated, resp)
	}
}

func handlePatchDirectoryUser(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users := store.AuthUsers(st)
		if users == nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		cur, err := users.GetAuthUser(r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		var body struct {
			DisplayName *string `json:"display_name"`
			Status      *string `json:"status"`
			Password    *string `json:"password"`
			Email       *string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		next := *cur
		if body.DisplayName != nil {
			next.DisplayName = strings.TrimSpace(*body.DisplayName)
		}
		if body.Status != nil {
			next.Status = strings.ToLower(strings.TrimSpace(*body.Status))
		}
		if body.Email != nil && strings.TrimSpace(*body.Email) != "" {
			next.Email = strings.ToLower(strings.TrimSpace(*body.Email))
		}
		if body.Password != nil && *body.Password != "" {
			if len(*body.Password) < 8 {
				writeErr(w, http.StatusBadRequest, "password must be at least 8 characters")
				return
			}
			hash, herr := bcrypt.GenerateFromPassword([]byte(*body.Password), 12)
			if herr != nil {
				writeErr(w, http.StatusInternalServerError, "hash failed")
				return
			}
			next.PasswordHash = string(hash)
		}
		updated, err := users.UpdateAuthUser(next)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, userPublicView(updated))
	}
}

func handleDeleteDirectoryUser(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users := store.AuthUsers(st)
		if users == nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		u, gerr := users.GetAuthUser(r.PathValue("id"))
		if gerr != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if err := users.DeleteAuthUser(u.ID); err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		// A deleted account must not stay on any tenant roster.
		if reg := tenant.DefaultRegistry(); reg != nil {
			for _, t := range reg.List("") {
				row, rerr := reg.Get(t.ID)
				if rerr != nil {
					continue
				}
				for _, m := range row.Members {
					if m.Subject == u.ID || (u.Email != "" && strings.EqualFold(m.Subject, u.Email)) {
						_, _, _ = reg.RemoveMember(row.ID, m.ID, m.Subject)
					}
				}
			}
		}
		if sessions := store.AuthSessions(st); sessions != nil {
			_ = sessions.DeleteAuthSessionsForUser(u.ID)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleGoClawTenantUsers(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tid := requestTenant(r)
		reg := tenant.DefaultRegistry()
		users := store.AuthUsers(st)
		out := []map[string]any{}
		if reg != nil {
			row, err := reg.Get(tid)
			if err == nil {
				for _, m := range row.Members {
					label := ""
					if users != nil {
						if u, uerr := users.GetAuthUserByEmail(m.Subject); uerr == nil {
							label = firstNonEmpty(u.DisplayName, u.Email)
						}
					}
					if label == "" {
						label = firstNonEmpty(gosoUserDisplay(m.Subject, tid), m.Subject)
					}
					out = append(out, map[string]any{
						"id": m.Subject, "tenant_id": tid, "user_id": m.Subject,
						"display_name": label, "role": m.Role,
					})
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": out})
	}
}

func randomPassword() string {
	const alphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 16)
	seed := time.Now().UnixNano()
	for i := range b {
		seed = seed*6364136223846793005 + 1442695040888963407
		b[i] = alphabet[int(seed>>33)%len(alphabet)]
	}
	return string(b)
}
