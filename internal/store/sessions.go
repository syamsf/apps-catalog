package store

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

type User struct {
	Subject  string `json:"subject"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	// portal groups
	Groups  []string `json:"groups"`
	IsAdmin bool     `json:"is_admin"`
	// null when they never signed in
	LastLogin *time.Time `json:"last_login"`
	// sso or local
	Source string `json:"source"`
}

// DisplayName is the name, else the username.
func (u User) DisplayName() string {
	if u.Name != "" {
		return u.Name
	}
	return u.Username
}

type Login struct {
	State    string
	Nonce    string
	Verifier string
	Redirect string
}

const loginTTL = 10 * time.Minute

const selectUser = `
	SELECT u.subject, u.username, u.name, u.email,
	       coalesce((SELECT array_agg(m.group_name ORDER BY m.group_name) FROM group_members m WHERE m.subject = u.subject), '{}'),
	       u.is_admin, u.last_login, u.source
	FROM users u`

func scanUser(r pgx.CollectableRow) (User, error) {
	var u User
	err := r.Scan(&u.Subject, &u.Username, &u.Name, &u.Email, &u.Groups, &u.IsAdmin, &u.LastLogin, &u.Source)
	return u, err
}

// SaveLoginUser records an sso sign-in: creates the user or updates its
// profile and admin flag. Local users are never touched.
func (s *Store) SaveLoginUser(ctx context.Context, u User) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (subject, username, name, email, is_admin, last_login)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (subject) DO UPDATE SET username = EXCLUDED.username, name = EXCLUDED.name,
			email = EXCLUDED.email, is_admin = EXCLUDED.is_admin, last_login = now()
		WHERE users.source = 'sso'`,
		u.Subject, u.Username, u.Name, u.Email, u.IsAdmin)
	return err
}

func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, subject string, expires time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sessions (token_hash, subject, expires_at) VALUES ($1, $2, $3)`,
		tokenHash, subject, expires)
	return err
}

// Session returns the user of a session, with its current portal groups.
// ErrNotFound for unknown and expired sessions.
func (s *Store) Session(ctx context.Context, tokenHash []byte) (User, error) {
	rows, err := s.pool.Query(ctx, selectUser+`
		JOIN sessions s ON s.subject = u.subject
		WHERE s.token_hash = $1 AND s.expires_at > now()`, tokenHash)
	if err != nil {
		return User{}, err
	}
	u, err := pgx.CollectExactlyOneRow(rows, scanUser)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *Store) SaveLogin(ctx context.Context, l Login) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO oidc_logins (state, nonce, verifier, redirect) VALUES ($1, $2, $3, $4)`,
		l.State, l.Nonce, l.Verifier, l.Redirect)
	return err
}

// TakeLogin returns the pending login for state and deletes it, so each
// state works once.
func (s *Store) TakeLogin(ctx context.Context, state string) (Login, error) {
	var l Login
	err := s.pool.QueryRow(ctx, `
		DELETE FROM oidc_logins WHERE state = $1 AND created_at > $2
		RETURNING state, nonce, verifier, redirect`, state, time.Now().Add(-loginTTL)).
		Scan(&l.State, &l.Nonce, &l.Verifier, &l.Redirect)
	if errors.Is(err, pgx.ErrNoRows) {
		return Login{}, ErrNotFound
	}
	return l, err
}

// DeleteExpired cleans up old sessions and abandoned logins.
func (s *Store) DeleteExpired(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= now()`); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM oidc_logins WHERE created_at <= $1`, time.Now().Add(-loginTTL))
	return err
}

// Users lists everyone who has signed in, by name.
func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, selectUser+` ORDER BY lower(coalesce(nullif(u.name, ''), u.username)), u.subject`)
	if err != nil {
		return nil, err
	}
	users, err := pgx.CollectRows(rows, scanUser)
	if users == nil {
		users = []User{}
	}
	return users, err
}

func (s *Store) user(ctx context.Context, q querier, subject string) (User, error) {
	rows, err := q.Query(ctx, selectUser+` WHERE u.subject = $1`, subject)
	if err != nil {
		return User{}, err
	}
	u, err := pgx.CollectExactlyOneRow(rows, scanUser)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

// SetUserGroups replaces the portal groups of a user.
func (s *Store) SetUserGroups(ctx context.Context, by Actor, subject string, groups []string) (User, error) {
	var u User
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		before, err := s.user(ctx, tx, subject)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM group_members WHERE subject = $1`, subject); err != nil {
			return err
		}
		if len(groups) > 0 {
			_, err := tx.Exec(ctx, `
				INSERT INTO group_members (group_name, subject) SELECT unnest($2::text[]), $1 ON CONFLICT DO NOTHING`,
				subject, groups)
			if err != nil {
				return dbErr(err)
			}
		}
		if u, err = s.user(ctx, tx, subject); err != nil {
			return err
		}
		added, removed := diff(before.Groups, u.Groups)
		var summary string
		switch {
		case len(added) > 0 && len(removed) > 0:
			summary = "changed the groups of " + u.DisplayName()
		case len(added) > 0:
			summary = "added " + u.DisplayName() + " to " + joinAnd(added)
		case len(removed) > 0:
			summary = "removed " + u.DisplayName() + " from " + joinAnd(removed)
		default:
			return nil
		}
		return logChange(ctx, tx, by, summary)
	})
	return u, err
}

// diff returns what is in b but not a, and what is in a but not b.
func diff(a, b []string) (added, removed []string) {
	for _, x := range b {
		if !slices.Contains(a, x) {
			added = append(added, x)
		}
	}
	for _, x := range a {
		if !slices.Contains(b, x) {
			removed = append(removed, x)
		}
	}
	return added, removed
}
