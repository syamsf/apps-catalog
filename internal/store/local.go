package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	SourceSSO   = "sso"
	SourceLocal = "local"
)

// ErrNotLocal is an action that only works on local users.
var ErrNotLocal = errors.New("not a local user")

// LocalSubject is the subject of a local user, kept apart from sso
// subjects by the prefix.
func LocalSubject(username string) string {
	return "local:" + strings.ToLower(username)
}

type LocalUserInput struct {
	Username string   `json:"username"`
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	IsAdmin  bool     `json:"is_admin"`
	Groups   []string `json:"groups"`
	Password string   `json:"password"`
}

// CreateLocalUser adds a user who signs in with a password. ErrConflict
// when the username is taken.
func (s *Store) CreateLocalUser(ctx context.Context, by Actor, in LocalUserInput, hash string) (User, error) {
	var u User
	subject := LocalSubject(in.Username)
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO users (subject, username, name, email, is_admin, source, password_hash, last_login)
			VALUES ($1, $2, $3, $4, $5, 'local', $6, NULL)`,
			subject, in.Username, in.Name, in.Email, in.IsAdmin, hash)
		if err != nil {
			return dbErr(err)
		}
		if len(in.Groups) > 0 {
			_, err := tx.Exec(ctx, `
				INSERT INTO group_members (group_name, subject) SELECT unnest($2::text[]), $1 ON CONFLICT DO NOTHING`,
				subject, in.Groups)
			if err != nil {
				return dbErr(err)
			}
		}
		if u, err = s.user(ctx, tx, subject); err != nil {
			return err
		}
		return logChange(ctx, tx, by, "added the local user "+u.DisplayName())
	})
	return u, err
}

// UpdateLocalUser changes the name, email and admin flag of a local user.
func (s *Store) UpdateLocalUser(ctx context.Context, by Actor, subject, name, email string, isAdmin bool) (User, error) {
	var u User
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		before, err := s.user(ctx, tx, subject)
		if err != nil {
			return err
		}
		if before.Source != SourceLocal {
			return ErrNotLocal
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET name = $2, email = $3, is_admin = $4 WHERE subject = $1`,
			subject, name, email, isAdmin); err != nil {
			return err
		}
		if u, err = s.user(ctx, tx, subject); err != nil {
			return err
		}
		summary := "edited the user " + u.DisplayName()
		switch {
		case isAdmin && !before.IsAdmin:
			summary = "made " + u.DisplayName() + " an admin"
		case !isAdmin && before.IsAdmin:
			summary = "removed admin from " + u.DisplayName()
		}
		return logChange(ctx, tx, by, summary)
	})
	return u, err
}

// SetPassword replaces the password hash of a local user. A nil actor
// means the user changed it themselves, which is not logged.
func (s *Store) SetPassword(ctx context.Context, by *Actor, subject, hash string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var name, source string
		err := tx.QueryRow(ctx, `
			UPDATE users SET password_hash = CASE WHEN source = 'local' THEN $2 ELSE password_hash END
			WHERE subject = $1 RETURNING coalesce(nullif(name, ''), username), source`, subject, hash).Scan(&name, &source)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if source != SourceLocal {
			return ErrNotLocal
		}
		if by == nil {
			return nil
		}
		return logChange(ctx, tx, *by, "set a new password for "+name)
	})
}

// LocalCredentials returns the subject and password hash for a local
// username, ErrNotFound when there is no such local user.
func (s *Store) LocalCredentials(ctx context.Context, username string) (subject, hash string, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT subject, password_hash FROM users WHERE source = 'local' AND lower(username) = lower($1)`,
		username).Scan(&subject, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return subject, hash, err
}

// PasswordHash returns the hash of a local user by subject.
func (s *Store) PasswordHash(ctx context.Context, subject string) (string, error) {
	var hash string
	err := s.pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE subject = $1 AND source = 'local'`, subject).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return hash, err
}

func (s *Store) TouchLogin(ctx context.Context, subject string) error {
	_, err := s.pool.Exec(ctx, `UPDATE users SET last_login = now() WHERE subject = $1`, subject)
	return err
}

// DeleteUser removes a user with their sessions, groups and favorites. An
// sso user comes back on their next sign-in, without groups.
func (s *Store) DeleteUser(ctx context.Context, by Actor, subject string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `DELETE FROM users WHERE subject = $1 RETURNING coalesce(nullif(name, ''), username)`,
			subject).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM favorites WHERE subject = $1`, subject); err != nil {
			return err
		}
		return logChange(ctx, tx, by, "removed the user "+name)
	})
}

// LocalAdmins counts local users with admin rights.
func (s *Store) LocalAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE source = 'local' AND is_admin`).Scan(&n)
	return n, err
}
