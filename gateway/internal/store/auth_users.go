// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// AuthUser is one login row for the control plane and the API.
// PasswordHash is bcrypt (the GoClaw format), never returned by JSON.
type AuthUser struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	DisplayName  string    `json:"display_name,omitempty"`
	Status       string    `json:"status"`
	FailedCount  int       `json:"failed_login_count,omitempty"`
	LockedUntil  time.Time `json:"locked_until,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Auth user statuses.
const (
	AuthUserActive    = "active"
	AuthUserSuspended = "suspended"
)

var (
	// ErrAuthUserExists is a duplicate email.
	ErrAuthUserExists = errors.New("auth user already exists")
	// ErrAuthUserNoEmail is a missing email on create.
	ErrAuthUserNoEmail = errors.New("email is required")
)

// AuthUserStore is implemented by Store, SQLiteStore and PostgresStore.
type AuthUserStore interface {
	CreateAuthUser(AuthUser) (*AuthUser, error)
	GetAuthUser(id string) (*AuthUser, error)
	GetAuthUserByEmail(email string) (*AuthUser, error)
	ListAuthUsers() []*AuthUser
	UpdateAuthUser(AuthUser) (*AuthUser, error)
	DeleteAuthUser(id string) error
	// NoteAuthUserFailure increments the failed-login counter and reports the
	// new count. ResetAuthUserFailure clears the counter and the lock.
	NoteAuthUserFailure(id string) (int, error)
	ResetAuthUserFailure(id string) error
}

var (
	_ AuthUserStore = (*Store)(nil)
	_ AuthUserStore = (*SQLiteStore)(nil)
)

// AuthUsers returns the auth-user capability of st, or nil.
func AuthUsers(st StoreIface) AuthUserStore {
	if u, ok := st.(AuthUserStore); ok {
		return u
	}
	return nil
}

func normalizeAuthUser(u AuthUser) (AuthUser, error) {
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	if u.Email == "" {
		return AuthUser{}, ErrAuthUserNoEmail
	}
	u.DisplayName = strings.TrimSpace(u.DisplayName)
	u.Status = strings.ToLower(strings.TrimSpace(u.Status))
	if u.Status == "" {
		u.Status = AuthUserActive
	}
	if u.Status != AuthUserActive && u.Status != AuthUserSuspended {
		u.Status = AuthUserActive
	}
	return u, nil
}

// --- SQLite / Postgres ---

func (s *SQLiteStore) migrateAuthUsers() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS auth_users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL,
			password_hash TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			failed_login_count INTEGER NOT NULL DEFAULT 0,
			locked_until TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_auth_users_email ON auth_users(email)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) CreateAuthUser(u AuthUser) (*AuthUser, error) {
	u, err := normalizeAuthUser(u)
	if err != nil {
		return nil, err
	}
	if existing, _ := s.GetAuthUserByEmail(u.Email); existing != nil {
		return nil, ErrAuthUserExists
	}
	if u.ID == "" {
		u.ID = "usr_" + newID()
	}
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now
	_, err = s.db.Exec(`INSERT INTO auth_users(id, email, password_hash, display_name, status, failed_login_count, locked_until, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		u.ID, u.Email, u.PasswordHash, u.DisplayName, u.Status, u.FailedCount, lockText(u.LockedUntil),
		formatTime(u.CreatedAt), formatTime(u.UpdatedAt))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, ErrAuthUserExists
		}
		return nil, err
	}
	cp := u
	return &cp, nil
}

func lockText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return formatTime(t)
}

func (s *SQLiteStore) GetAuthUser(id string) (*AuthUser, error) {
	return scanAuthUser(s.db.QueryRow(`SELECT id, email, password_hash, display_name, status, failed_login_count, locked_until, created_at, updated_at
		FROM auth_users WHERE id=?`, strings.TrimSpace(id)))
}

func (s *SQLiteStore) GetAuthUserByEmail(email string) (*AuthUser, error) {
	return scanAuthUser(s.db.QueryRow(`SELECT id, email, password_hash, display_name, status, failed_login_count, locked_until, created_at, updated_at
		FROM auth_users WHERE email=?`, strings.ToLower(strings.TrimSpace(email))))
}

func (s *SQLiteStore) ListAuthUsers() []*AuthUser {
	rows, err := s.db.Query(`SELECT id, email, password_hash, display_name, status, failed_login_count, locked_until, created_at, updated_at
		FROM auth_users ORDER BY email`)
	if err != nil {
		return []*AuthUser{}
	}
	defer rows.Close()
	out := []*AuthUser{}
	for rows.Next() {
		u, err := scanAuthUserRow(rows)
		if err != nil {
			continue
		}
		out = append(out, u)
	}
	return out
}

func (s *SQLiteStore) UpdateAuthUser(u AuthUser) (*AuthUser, error) {
	u, err := normalizeAuthUser(u)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(u.ID) == "" {
		return nil, ErrNotFound
	}
	u.UpdatedAt = time.Now().UTC()
	res, err := s.db.Exec(`UPDATE auth_users SET email=?, password_hash=?, display_name=?, status=?, failed_login_count=?, locked_until=?, updated_at=? WHERE id=?`,
		u.Email, u.PasswordHash, u.DisplayName, u.Status, u.FailedCount, lockText(u.LockedUntil), formatTime(u.UpdatedAt), u.ID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	cp := u
	return &cp, nil
}

func (s *SQLiteStore) DeleteAuthUser(id string) error {
	res, err := s.db.Exec(`DELETE FROM auth_users WHERE id=?`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) NoteAuthUserFailure(id string) (int, error) {
	u, err := s.GetAuthUser(id)
	if err != nil {
		return 0, err
	}
	u.FailedCount++
	u.UpdatedAt = time.Now().UTC()
	if _, err := s.db.Exec(`UPDATE auth_users SET failed_login_count=?, updated_at=? WHERE id=?`, u.FailedCount, formatTime(u.UpdatedAt), id); err != nil {
		return 0, err
	}
	return u.FailedCount, nil
}

func (s *SQLiteStore) ResetAuthUserFailure(id string) error {
	_, err := s.db.Exec(`UPDATE auth_users SET failed_login_count=0, locked_until='', updated_at=? WHERE id=?`, formatTime(time.Now().UTC()), id)
	return err
}

type authUserScanner interface {
	Scan(dest ...any) error
}

func scanAuthUser(row *sql.Row) (*AuthUser, error) {
	u, err := scanAuthUserRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func scanAuthUserRow(sc authUserScanner) (*AuthUser, error) {
	var (
		u       AuthUser
		created string
		updated string
		locked  string
	)
	if err := sc.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Status, &u.FailedCount, &locked, &created, &updated); err != nil {
		return nil, err
	}
	u.CreatedAt = parseTime(created)
	u.UpdatedAt = parseTime(updated)
	u.LockedUntil = parseTime(locked)
	return &u, nil
}

// --- memory ---

func (s *Store) CreateAuthUser(u AuthUser) (*AuthUser, error) {
	u, err := normalizeAuthUser(u)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.authUsers == nil {
		s.authUsers = map[string]*AuthUser{}
	}
	for _, cur := range s.authUsers {
		if cur.Email == u.Email {
			return nil, ErrAuthUserExists
		}
	}
	if u.ID == "" {
		u.ID = "usr_" + newID()
	}
	now := time.Now().UTC()
	u.CreatedAt = now
	u.UpdatedAt = now
	cp := u
	s.authUsers[u.ID] = &cp
	out := cp
	return &out, nil
}

func (s *Store) GetAuthUser(id string) (*AuthUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.authUsers[strings.TrimSpace(id)]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *u
	return &cp, nil
}

func (s *Store) GetAuthUserByEmail(email string) (*AuthUser, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, u := range s.authUsers {
		if u.Email == email {
			cp := *u
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) ListAuthUsers() []*AuthUser {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*AuthUser, 0, len(s.authUsers))
	for _, u := range s.authUsers {
		cp := *u
		out = append(out, &cp)
	}
	return out
}

func (s *Store) UpdateAuthUser(u AuthUser) (*AuthUser, error) {
	u, err := normalizeAuthUser(u)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.authUsers[u.ID]
	if !ok {
		return nil, ErrNotFound
	}
	u.CreatedAt = cur.CreatedAt
	u.UpdatedAt = time.Now().UTC()
	cp := u
	s.authUsers[u.ID] = &cp
	out := cp
	return &out, nil
}

func (s *Store) DeleteAuthUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.authUsers[strings.TrimSpace(id)]; !ok {
		return ErrNotFound
	}
	delete(s.authUsers, strings.TrimSpace(id))
	return nil
}

func (s *Store) NoteAuthUserFailure(id string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.authUsers[id]
	if !ok {
		return 0, ErrNotFound
	}
	u.FailedCount++
	u.UpdatedAt = time.Now().UTC()
	return u.FailedCount, nil
}

func (s *Store) ResetAuthUserFailure(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.authUsers[id]
	if !ok {
		return ErrNotFound
	}
	u.FailedCount = 0
	u.LockedUntil = time.Time{}
	u.UpdatedAt = time.Now().UTC()
	return nil
}
