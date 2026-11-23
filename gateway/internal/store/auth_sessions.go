package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// AuthSession is one signed-in browser or API client. Token is never stored;
// TokenHash is sha256(token) hex. A session carries the tenant the user
// signed in to, so requests never pick a tenant from a header alone.
type AuthSession struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email,omitempty"`
	TenantID  string    `json:"tenant_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ErrSessionInvalid is a missing, expired, or revoked session.
var ErrSessionInvalid = errors.New("session invalid")

// AuthSessionStore is implemented by Store and SQLiteStore (Postgres embeds it).
type AuthSessionStore interface {
	CreateAuthSession(userID, email, tenantID, role string, ttl time.Duration) (string, AuthSession, error)
	LookupAuthSession(token string) (*AuthSession, error)
	DeleteAuthSession(token string) error
	DeleteAuthSessionsForUser(userID string) error
}

var (
	_ AuthSessionStore = (*Store)(nil)
	_ AuthSessionStore = (*SQLiteStore)(nil)
)

// AuthSessions returns the session capability of st, or nil.
func AuthSessions(st StoreIface) AuthSessionStore {
	if s, ok := st.(AuthSessionStore); ok {
		return s
	}
	return nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func newSessionToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (s *SQLiteStore) migrateAuthSessions() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS auth_sessions (
			id TEXT PRIMARY KEY,
			token_hash TEXT NOT NULL,
			user_id TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT 'default',
			role TEXT NOT NULL DEFAULT 'member',
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_auth_sessions_token ON auth_sessions(token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_auth_sessions_user ON auth_sessions(user_id)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) CreateAuthSession(userID, email, tenantID, role string, ttl time.Duration) (string, AuthSession, error) {
	token, err := newSessionToken()
	if err != nil {
		return "", AuthSession{}, err
	}
	now := time.Now().UTC()
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	sess := AuthSession{
		ID:        "ses_" + newID(),
		UserID:    strings.TrimSpace(userID),
		Email:     strings.ToLower(strings.TrimSpace(email)),
		TenantID:  NormalizeTenant(tenantID),
		Role:      strings.ToLower(strings.TrimSpace(role)),
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if sess.Role == "" {
		sess.Role = "member"
	}
	_, err = s.db.Exec(`INSERT INTO auth_sessions(id, token_hash, user_id, email, tenant_id, role, created_at, expires_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		sess.ID, hashSessionToken(token), sess.UserID, sess.Email, sess.TenantID, sess.Role,
		formatTime(sess.CreatedAt), formatTime(sess.ExpiresAt))
	if err != nil {
		return "", AuthSession{}, err
	}
	return token, sess, nil
}

func (s *SQLiteStore) LookupAuthSession(token string) (*AuthSession, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrSessionInvalid
	}
	var sess AuthSession
	var created, expires string
	err := s.db.QueryRow(`SELECT id, user_id, email, tenant_id, role, created_at, expires_at
		FROM auth_sessions WHERE token_hash=?`, hashSessionToken(token)).
		Scan(&sess.ID, &sess.UserID, &sess.Email, &sess.TenantID, &sess.Role, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionInvalid
	}
	if err != nil {
		return nil, err
	}
	sess.CreatedAt = parseTime(created)
	sess.ExpiresAt = parseTime(expires)
	if !sess.ExpiresAt.IsZero() && time.Now().UTC().After(sess.ExpiresAt) {
		return nil, ErrSessionInvalid
	}
	return &sess, nil
}

func (s *SQLiteStore) DeleteAuthSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM auth_sessions WHERE token_hash=?`, hashSessionToken(token))
	return err
}

func (s *SQLiteStore) DeleteAuthSessionsForUser(userID string) error {
	_, err := s.db.Exec(`DELETE FROM auth_sessions WHERE user_id=?`, strings.TrimSpace(userID))
	return err
}

// --- memory ---

func (s *Store) CreateAuthSession(userID, email, tenantID, role string, ttl time.Duration) (string, AuthSession, error) {
	token, err := newSessionToken()
	if err != nil {
		return "", AuthSession{}, err
	}
	now := time.Now().UTC()
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	sess := AuthSession{
		ID: "ses_" + newID(), UserID: strings.TrimSpace(userID),
		Email: strings.ToLower(strings.TrimSpace(email)), TenantID: NormalizeTenant(tenantID),
		Role: strings.ToLower(strings.TrimSpace(role)), CreatedAt: now, ExpiresAt: now.Add(ttl),
	}
	if sess.Role == "" {
		sess.Role = "member"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.authSessions == nil {
		s.authSessions = map[string]*AuthSession{}
	}
	cp := sess
	s.authSessions[hashSessionToken(token)] = &cp
	return token, sess, nil
}

func (s *Store) LookupAuthSession(token string) (*AuthSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.authSessions[hashSessionToken(token)]
	if !ok || time.Now().UTC().After(sess.ExpiresAt) {
		return nil, ErrSessionInvalid
	}
	cp := *sess
	return &cp, nil
}

func (s *Store) DeleteAuthSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.authSessions, hashSessionToken(token))
	return nil
}

func (s *Store) DeleteAuthSessionsForUser(userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.authSessions {
		if v.UserID == strings.TrimSpace(userID) {
			delete(s.authSessions, k)
		}
	}
	return nil
}
