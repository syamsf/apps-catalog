package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"apps-catalog/internal/catalog"
)

// AllApps returns every app, ignoring visibility. Used by the health checker.
func (s *Store) AllApps(ctx context.Context) ([]catalog.App, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, health_url, urls FROM apps ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (catalog.App, error) {
		var a catalog.App
		return a, r.Scan(&a.ID, &a.Name, &a.HealthURL, &a.URLs)
	})
}

// SaveStatus stores a check result. Results for apps deleted while the
// check was running are dropped.
func (s *Store) SaveStatus(ctx context.Context, appID string, st Status) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO app_status (app_id, up, http_status, latency_ms, error, target, checked_at, since)
		SELECT $1, $2, $3, $4, $5, $6, $7, $7
		WHERE EXISTS (SELECT 1 FROM apps WHERE id = $1)
		ON CONFLICT (app_id) DO UPDATE SET
			up = EXCLUDED.up, http_status = EXCLUDED.http_status, latency_ms = EXCLUDED.latency_ms,
			error = EXCLUDED.error, target = EXCLUDED.target, checked_at = EXCLUDED.checked_at,
			since = CASE WHEN app_status.up = EXCLUDED.up THEN app_status.since ELSE EXCLUDED.checked_at END`,
		appID, st.Up, st.HTTPStatus, st.LatencyMS, st.Error, st.Target, st.CheckedAt)
	return err
}

// ClearStatus removes the status of an app that has no url to check anymore.
func (s *Store) ClearStatus(ctx context.Context, appID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM app_status WHERE app_id = $1`, appID)
	return err
}
