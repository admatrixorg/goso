package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/tenant"
)

const (
	loginLockThreshold = 6
	loginLockDuration  = 15 * time.Minute
	loginSessionTTL    = 7 * 24 * time.Hour
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name,omitempty"`
}

type loginTenant struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name,omitempty"`
	Role string `json:"role"`
}

type loginResponse struct {
	AccessToken string      `json:"access_token"`
	TokenType   string      `json:"token_type"`
	ExpiresIn   int         `json:"expires_in"`
	User        loginUser   `json:"user"`
	Tenant      loginTenant `json:"tenant"`
}

type loginAccount struct {
	email string
	pass  string
	id    string
}

type loginFailState struct {
	count int
	until time.Time
}

var (
	loginFailMu sync.Mutex
	loginFails  = map[string]*loginFailState{}
)

func registerAuthLoginRoutes(mux *http.ServeMux, st store.StoreIface) {
	aliasAPI(mux, "POST /api/auth/login", handleAuthLogin(st))
}

func configuredLoginAccounts() []loginAccount {
	out := make([]loginAccount, 0, 4)
	add := func(email, pass, id string) {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" || pass == "" {
			return
		}
		if id == "" {
			id = email
		}
		out = append(out, loginAccount{email: email, pass: pass, id: id})
	}
	add(os.Getenv("GOSO_LOGIN_EMAIL"), os.Getenv("GOSO_LOGIN_PASSWORD"), os.Getenv("GOSO_LOGIN_USER_ID"))
	add(os.Getenv("GOSO_LOGIN_EMAIL_2"), os.Getenv("GOSO_LOGIN_PASSWORD_2"), os.Getenv("GOSO_LOGIN_USER_ID_2"))
	add(os.Getenv("GOSO_LOGIN_EMAIL_3"), os.Getenv("GOSO_LOGIN_PASSWORD_3"), os.Getenv("GOSO_LOGIN_USER_ID_3"))
	return out
}

// loginTenantFor picks the tenant a signed-in user works in: an owner
// membership first, then any membership. Falls back to the master tenant.
func loginTenantFor(email, userID string) loginTenant {
	reg := tenant.DefaultRegistry()
	if reg == nil {
		return loginTenant{ID: "master", Slug: "master", Name: "Master", Role: tenant.RoleOwner}
	}
	var fallback *loginTenant
	for _, t := range reg.List("") {
		row, gerr := reg.Get(t.ID)
		if gerr != nil {
			continue
		}
		for _, m := range row.Members {
			if !strings.EqualFold(m.Subject, email) && m.Subject != userID {
				continue
			}
			candidate := loginTenant{ID: row.ID, Slug: row.ID, Name: row.Name, Role: m.Role}
			if m.Role == tenant.RoleOwner {
				return candidate
			}
			if fallback == nil {
				c := candidate
				fallback = &c
			}
		}
	}
	if fallback != nil {
		return *fallback
	}
	return loginTenant{ID: "master", Slug: "master", Name: "Master", Role: tenant.RoleOwner}
}

// issueToken returns a per-user session token. The admin token stays a
// break-glass fallback when sessions cannot be written.
func issueToken(st store.StoreIface, userID, email string, tn loginTenant) string {
	if sessions := store.AuthSessions(st); sessions != nil {
		if token, _, err := sessions.CreateAuthSession(userID, email, tn.ID, tn.Role, loginSessionTTL); err == nil && token != "" {
			return token
		}
	}
	return strings.TrimSpace(os.Getenv("GOSO_ADMIN_TOKEN"))
}

func handleAuthLogin(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "email and password required")
			return
		}
		email := strings.ToLower(strings.TrimSpace(req.Email))
		pass := req.Password
		if email == "" || pass == "" {
			writeErr(w, http.StatusBadRequest, "email and password required")
			return
		}

		admin := strings.TrimSpace(os.Getenv("GOSO_ADMIN_TOKEN"))
		if admin == "" {
			writeErr(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		if loginIsLocked(email) {
			writeErr(w, http.StatusLocked, "account temporarily locked")
			return
		}

		if users := store.AuthUsers(st); users != nil {
			if u, err := users.GetAuthUserByEmail(email); err == nil && u != nil {
				if u.Status != store.AuthUserActive {
					writeErr(w, http.StatusForbidden, "account not active")
					return
				}
				if u.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(pass)) != nil {
					if loginNoteFail(email) {
						writeErr(w, http.StatusLocked, "account temporarily locked")
						return
					}
					_, _ = users.NoteAuthUserFailure(u.ID)
					writeErr(w, http.StatusUnauthorized, "invalid credentials")
					return
				}
				_ = users.ResetAuthUserFailure(u.ID)
				loginClearFail(email)
				membership := loginTenantFor(u.Email, u.ID)
				writeJSON(w, http.StatusOK, loginResponse{
					AccessToken: issueToken(st, u.ID, u.Email, membership),
					TokenType:   "bearer",
					ExpiresIn:   int(loginSessionTTL.Seconds()),
					User:        loginUser{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName},
					Tenant:      membership,
				})
				return
			}
		}

		for _, acc := range configuredLoginAccounts() {
			if email == acc.email || email == strings.Split(acc.email, "@")[0] {
				if security.Equal(pass, acc.pass) {
					loginClearFail(email)
					membership := loginTenantFor(acc.email, acc.id)
					writeJSON(w, http.StatusOK, loginResponse{
						AccessToken: issueToken(st, acc.id, acc.email, membership),
						TokenType:   "bearer",
						ExpiresIn:   int(loginSessionTTL.Seconds()),
						User:        loginUser{ID: acc.id, Email: acc.email},
						Tenant:      membership,
					})
					return
				}
			}
		}

		if loginNoteFail(email) {
			writeErr(w, http.StatusLocked, "account temporarily locked")
			return
		}
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
	}
}

func loginIsLocked(email string) bool {
	loginFailMu.Lock()
	defer loginFailMu.Unlock()
	st := loginFails[email]
	return st != nil && !st.until.IsZero() && time.Now().Before(st.until)
}

func loginNoteFail(email string) bool {
	loginFailMu.Lock()
	defer loginFailMu.Unlock()
	st := loginFails[email]
	if st == nil {
		st = &loginFailState{}
		loginFails[email] = st
	}
	st.count++
	if st.count >= loginLockThreshold {
		st.until = time.Now().Add(loginLockDuration)
		return true
	}
	return false
}

func loginClearFail(email string) {
	loginFailMu.Lock()
	defer loginFailMu.Unlock()
	delete(loginFails, email)
}

func loginResetFailsForTest() {
	loginFailMu.Lock()
	defer loginFailMu.Unlock()
	loginFails = map[string]*loginFailState{}
}
