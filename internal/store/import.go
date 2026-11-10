package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"apps-catalog/internal/catalog"
)

type ImportResult struct {
	Added   []string `json:"added"`
	Updated []string `json:"updated"`
	Skipped []string `json:"skipped"`
}

// Import adds apps from the yaml catalog. Apps whose id already exists are
// skipped, so edits made in the portal are kept. Missing groups, categories
// and environments are created. An owner that is no group is left empty.
func (s *Store) Import(ctx context.Context, by Actor, apps []catalog.App) (ImportResult, error) {
	res := ImportResult{Added: []string{}, Updated: []string{}, Skipped: []string{}}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// added app id to owner, set once all groups exist
		owners := map[string]string{}
		for _, a := range apps {
			visibility := VisibilityEveryone
			var groups []string
			for _, g := range a.AllowedGroups {
				g = strings.ToLower(strings.TrimSpace(g))
				if catalog.ValidID(g) {
					groups = append(groups, g)
				}
			}
			if len(a.AllowedGroups) > 0 {
				visibility = VisibilityGroups
			}
			tag, err := tx.Exec(ctx, `
				INSERT INTO apps (id, name, description, contact, category, access, health_url, urls, links,
				                  visibility, created_by, updated_by)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
				ON CONFLICT (id) DO NOTHING`,
				a.ID, a.Name, a.Description, a.Contact, a.Category, a.Access, a.HealthURL,
				nonNil(a.URLs), nonNil(a.Links), visibility, by.Name)
			if err != nil {
				return fmt.Errorf("%s: %w", a.ID, err)
			}
			if tag.RowsAffected() == 0 {
				res.Skipped = append(res.Skipped, a.ID)
				continue
			}
			res.Added = append(res.Added, a.ID)
			if o := strings.ToLower(strings.TrimSpace(a.Owner)); o != "" {
				owners[a.ID] = o
			}
			if len(groups) > 0 {
				if _, err := tx.Exec(ctx, `INSERT INTO groups (name) SELECT unnest($1::text[]) ON CONFLICT DO NOTHING`, groups); err != nil {
					return err
				}
				if err := setAppGroups(ctx, tx, a.ID, groups); err != nil {
					return err
				}
			}
			if a.Category != "" && catalog.ValidID(a.Category) {
				if err := ensureOption(ctx, tx, KindCategory, a.Category); err != nil {
					return err
				}
			}
			for env := range a.URLs {
				if catalog.ValidID(env) {
					if err := ensureOption(ctx, tx, KindEnvironment, env); err != nil {
						return err
					}
				}
			}
		}
		for id, owner := range owners {
			if _, err := tx.Exec(ctx, `UPDATE apps SET owner = g.name FROM groups g WHERE apps.id = $1 AND g.name = $2`,
				id, owner); err != nil {
				return err
			}
		}
		if len(res.Added) == 0 {
			return nil
		}
		return logChange(ctx, tx, by, "imported "+plural(len(res.Added), "app")+" from the yaml catalog")
	})
	return res, err
}

func ensureOption(ctx context.Context, tx pgx.Tx, kind, key string) error {
	return ensureOptionLabel(ctx, tx, kind, key, labelFor(key))
}

// labelFor makes a label from a key, "google-cloud" is "Google cloud".
func labelFor(key string) string {
	return strings.ToUpper(key[:1]) + strings.ReplaceAll(key[1:], "-", " ")
}

func ensureOptionLabel(ctx context.Context, tx pgx.Tx, kind, key, label string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO options (kind, key, label, position)
		SELECT $1, $2, $3, coalesce(max(position), 0) + 1 FROM options WHERE kind = $1
		ON CONFLICT DO NOTHING`, kind, key, label)
	return err
}
