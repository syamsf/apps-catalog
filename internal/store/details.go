package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// DetailsInput is what members of the owner group can change.
type DetailsInput struct {
	Description string            `json:"description"`
	Contact     string            `json:"contact"`
	Version     string            `json:"version"`
	Access      string            `json:"access"`
	HealthURL   string            `json:"health_url"`
	URLs        map[string]string `json:"urls"`
	Links       map[string]string `json:"links"`
	IconColor   string            `json:"icon_color"`
	Icon        string            `json:"icon"`
}

// Apply copies the details over in, so ValidateApp can check them.
func (d DetailsInput) Apply(in *AppInput) {
	in.Description, in.Contact, in.Version, in.Access, in.HealthURL = d.Description, d.Contact, d.Version, d.Access, d.HealthURL
	in.URLs, in.Links, in.IconColor, in.Icon = d.URLs, d.Links, d.IconColor, d.Icon
}

// UpdateDetails changes only the details of an app. The caller checks
// that by may do this.
func (s *Store) UpdateDetails(ctx context.Context, by Actor, id string, in DetailsInput) (App, error) {
	var app App
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `
			UPDATE apps SET description = $2, contact = $3, version = $4, access = $5, health_url = $6,
			       urls = $7, links = $8, icon_color = $9, icon = $10, updated_by = $11, updated_at = now()
			WHERE id = $1 RETURNING name`,
			id, in.Description, in.Contact, in.Version, in.Access, in.HealthURL,
			nonNil(in.URLs), nonNil(in.Links), in.IconColor, in.Icon, by.Name).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := logChange(ctx, tx, by, "edited the app "+name); err != nil {
			return err
		}
		app, err = s.get(ctx, tx, by.Subject, id)
		return err
	})
	return app, err
}

// Team is a group as any signed-in user sees it. Members are names only.
type Team struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Members     []string `json:"members"`
	// apps the group owns that the viewer can see
	Apps []App `json:"apps"`
}

func (s *Store) Team(ctx context.Context, v Viewer, name string) (Team, error) {
	g, err := s.Group(ctx, name)
	if err != nil {
		return Team{}, err
	}
	apps, err := s.List(ctx, v, Filter{Owner: g.Name})
	if err != nil {
		return Team{}, err
	}
	members := make([]string, 0, len(g.Members))
	for _, m := range g.Members {
		members = append(members, m.Name)
	}
	return Team{Name: g.Name, Description: g.Description, Members: members, Apps: apps}, nil
}
