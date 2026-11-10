package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type UserRef struct {
	Subject string `json:"subject"`
	Name    string `json:"name"`
	Email   string `json:"email"`
}

type AppRef struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
}

type Group struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Members     []UserRef `json:"members"`
	Apps        []AppRef  `json:"apps"`
	CreatedAt   time.Time `json:"created_at"`
}

type GroupInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Members     []string `json:"members"`
	Apps        []string `json:"apps"`
}

const selectGroup = `
	SELECT g.name, g.description, g.created_at,
	       coalesce((SELECT json_agg(json_build_object('subject', u.subject,
	                        'name', coalesce(nullif(u.name, ''), u.username), 'email', u.email)
	                        ORDER BY lower(coalesce(nullif(u.name, ''), u.username)))
	                 FROM group_members m JOIN users u ON u.subject = m.subject WHERE m.group_name = g.name), '[]'),
	       coalesce((SELECT json_agg(json_build_object('id', a.id, 'name', a.name, 'visibility', a.visibility)
	                        ORDER BY lower(a.name))
	                 FROM app_groups ag JOIN apps a ON a.id = ag.app_id WHERE ag.group_name = g.name), '[]')
	FROM groups g`

func scanGroup(r pgx.CollectableRow) (Group, error) {
	var g Group
	err := r.Scan(&g.Name, &g.Description, &g.CreatedAt, &g.Members, &g.Apps)
	return g, err
}

func (s *Store) Groups(ctx context.Context) ([]Group, error) {
	rows, err := s.pool.Query(ctx, selectGroup+` ORDER BY g.name`)
	if err != nil {
		return nil, err
	}
	groups, err := pgx.CollectRows(rows, scanGroup)
	if groups == nil {
		groups = []Group{}
	}
	return groups, err
}

func (s *Store) Group(ctx context.Context, name string) (Group, error) {
	return s.group(ctx, s.pool, name)
}

func (s *Store) group(ctx context.Context, q querier, name string) (Group, error) {
	rows, err := q.Query(ctx, selectGroup+` WHERE g.name = $1`, name)
	if err != nil {
		return Group{}, err
	}
	g, err := pgx.CollectExactlyOneRow(rows, scanGroup)
	if errors.Is(err, pgx.ErrNoRows) {
		return Group{}, ErrNotFound
	}
	return g, err
}

func (s *Store) CreateGroup(ctx context.Context, by Actor, in GroupInput) (Group, error) {
	var g Group
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO groups (name, description) VALUES ($1, $2)`, in.Name, in.Description)
		if err != nil {
			return dbErr(err)
		}
		if err := setGroupContents(ctx, tx, in); err != nil {
			return err
		}
		if err := logChange(ctx, tx, by, "created the group "+in.Name); err != nil {
			return err
		}
		g, err = s.group(ctx, tx, in.Name)
		return err
	})
	return g, err
}

// UpdateGroup can rename the group; members and apps follow the rename.
func (s *Store) UpdateGroup(ctx context.Context, by Actor, name string, in GroupInput) (Group, error) {
	var g Group
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE groups SET name = $2, description = $3 WHERE name = $1`,
			name, in.Name, in.Description)
		if err != nil {
			return dbErr(err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM group_members WHERE group_name = $1`, in.Name); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM app_groups WHERE group_name = $1`, in.Name); err != nil {
			return err
		}
		if err := setGroupContents(ctx, tx, in); err != nil {
			return err
		}
		summary := "edited the group " + in.Name
		if name != in.Name {
			summary = "renamed the group " + name + " to " + in.Name
		}
		if err := logChange(ctx, tx, by, summary); err != nil {
			return err
		}
		g, err = s.group(ctx, tx, in.Name)
		return err
	})
	return g, err
}

func setGroupContents(ctx context.Context, tx pgx.Tx, in GroupInput) error {
	if len(in.Members) > 0 {
		_, err := tx.Exec(ctx, `
			INSERT INTO group_members (group_name, subject) SELECT $1, unnest($2::text[]) ON CONFLICT DO NOTHING`,
			in.Name, in.Members)
		if err != nil {
			return dbErr(err)
		}
	}
	if len(in.Apps) > 0 {
		_, err := tx.Exec(ctx, `
			INSERT INTO app_groups (group_name, app_id) SELECT $1, unnest($2::text[]) ON CONFLICT DO NOTHING`,
			in.Name, in.Apps)
		if err != nil {
			return dbErr(err)
		}
	}
	return nil
}

func (s *Store) DeleteGroup(ctx context.Context, by Actor, name string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM groups WHERE name = $1`, name)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return logChange(ctx, tx, by, "deleted the group "+name)
	})
}
