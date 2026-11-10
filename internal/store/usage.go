package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const maxSearchRunes = 100

// LogSearch records what a user searched for. Empty queries are skipped.
func (s *Store) LogSearch(ctx context.Context, subject, q string, found bool) error {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	if r := []rune(q); len(r) > maxSearchRunes {
		q = string(r[:maxSearchRunes])
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO searches (subject, q, found) VALUES ($1, $2, $3)`, subject, q, found)
	return err
}

// LogOpen records that the viewer opened an app. ErrNotFound for missing
// apps and apps the viewer can't see.
func (s *Store) LogOpen(ctx context.Context, v Viewer, id string) error {
	args := []any{v.Subject, id}
	vis, args := visibleSQL(v, args)
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO opens (subject, app_id)
		SELECT $1, a.id FROM apps a WHERE a.id = $2 AND `+vis, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecentOpens returns the ids of the apps the viewer opened, newest first.
func (s *Store) RecentOpens(ctx context.Context, v Viewer, limit int) ([]string, error) {
	args := []any{v.Subject, limit}
	vis, args := visibleSQL(v, args)
	rows, err := s.pool.Query(ctx, `
		SELECT o.app_id FROM opens o JOIN apps a ON a.id = o.app_id
		WHERE o.subject = $1 AND `+vis+`
		GROUP BY o.app_id ORDER BY max(o.at) DESC, max(o.id) DESC LIMIT $2`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DeleteOldUsage drops searches after 90 days and opens after 180.
func (s *Store) DeleteOldUsage(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM searches WHERE at < now() - interval '90 days'`); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM opens WHERE at < now() - interval '180 days'`)
	return err
}

// Usage counts the last 7 days.
type Usage struct {
	People   int `json:"people"`
	Opens    int `json:"opens"`
	Searches int `json:"searches"`
	Missed   int `json:"missed"`
}

// MissedSearch is a query that found nothing. Hidden is set when an app
// matches it but at least one of the people who searched can't see it.
type MissedSearch struct {
	Query  string    `json:"query"`
	People int       `json:"people"`
	LastAt time.Time `json:"last_at"`
	Hidden *AppRef   `json:"hidden"`
}

const maxMissed = 10

// subjectSeesApp is true when the user whose subject is sub can see app a.
// A subject with no users row counts as an admin (login is off).
// Add further access rules as more OR clauses.
func subjectSeesApp(sub string) string {
	return `(a.visibility = 'everyone'
		OR NOT EXISTS (SELECT 1 FROM users u WHERE u.subject = ` + sub + `)
		OR EXISTS (SELECT 1 FROM users u WHERE u.subject = ` + sub + ` AND u.is_admin)
		OR EXISTS (SELECT 1 FROM app_groups g JOIN group_members m ON m.group_name = g.group_name
		           WHERE g.app_id = a.id AND m.subject = ` + sub + `)
		OR EXISTS (SELECT 1 FROM group_members m WHERE m.group_name = a.owner AND m.subject = ` + sub + `))`
}

func (s *Store) usage(ctx context.Context) (Usage, error) {
	var u Usage
	err := s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM (
		          SELECT subject FROM opens WHERE at > now() - interval '7 days'
		          UNION SELECT subject FROM searches WHERE at > now() - interval '7 days') p),
		       (SELECT count(*) FROM opens WHERE at > now() - interval '7 days'),
		       (SELECT count(*) FROM searches WHERE at > now() - interval '7 days'),
		       (SELECT count(*) FROM searches WHERE NOT found AND at > now() - interval '7 days')`).
		Scan(&u.People, &u.Opens, &u.Searches, &u.Missed)
	return u, err
}

func (s *Store) missedSearches(ctx context.Context) ([]MissedSearch, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.q, m.people, m.last_at, h.id, h.name, h.visibility
		FROM (
			SELECT q, count(DISTINCT subject) AS people, max(at) AS last_at
			FROM searches
			WHERE NOT found AND at > now() - interval '30 days'
			GROUP BY q
			HAVING NOT EXISTS (SELECT 1 FROM search_dismissals d WHERE d.q = searches.q AND d.at > max(searches.at))
		) m
		CROSS JOIN LATERAL (SELECT replace(replace(replace(m.q, E'\\', E'\\\\'), '%', E'\\%'), '_', E'\\_') AS q) esc
		LEFT JOIN LATERAL (
			SELECT a.id, a.name, a.visibility FROM apps a
			WHERE (a.id ILIKE '%' || esc.q || '%' OR a.name ILIKE '%' || esc.q || '%')
			  AND EXISTS (
				SELECT 1 FROM searches x
				WHERE x.q = m.q AND NOT x.found AND x.at > now() - interval '30 days'
				  AND NOT `+subjectSeesApp("x.subject")+`)
			ORDER BY lower(a.name), a.id LIMIT 1
		) h ON true
		ORDER BY m.people DESC, m.last_at DESC, m.q
		LIMIT $1`, maxMissed)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (MissedSearch, error) {
		var m MissedSearch
		var id, name, vis *string
		if err := r.Scan(&m.Query, &m.People, &m.LastAt, &id, &name, &vis); err != nil {
			return m, err
		}
		if id != nil {
			m.Hidden = &AppRef{ID: *id, Name: *name, Visibility: *vis}
		}
		return m, nil
	})
}

// DismissSearch hides a missed search until someone searches for it again.
func (s *Store) DismissSearch(ctx context.Context, by Actor, q string) error {
	q = strings.ToLower(strings.TrimSpace(q))
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO search_dismissals (q, at) VALUES ($1, now())
			ON CONFLICT (q) DO UPDATE SET at = now()`, q)
		if err != nil {
			return err
		}
		return logChange(ctx, tx, by, `dismissed the search "`+q+`"`)
	})
}
