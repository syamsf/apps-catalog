package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type AccessRequest struct {
	ID        int64      `json:"id"`
	Subject   string     `json:"subject"`
	Person    string     `json:"person"`
	AppID     string     `json:"app_id"`
	AppName   string     `json:"app_name"`
	Note      string     `json:"note"`
	Status    string     `json:"status"`
	Group     string     `json:"group"`
	DecidedBy string     `json:"decided_by"`
	CreatedAt time.Time  `json:"created_at"`
	DecidedAt *time.Time `json:"decided_at"`
}

const selectAccess = `
	SELECT r.id, r.subject, coalesce(nullif(u.name, ''), u.username), r.app_id, a.name, r.note, r.status,
	       r.group_name, r.decided_by, r.created_at, r.decided_at
	FROM access_requests r
	JOIN users u ON u.subject = r.subject
	JOIN apps a ON a.id = r.app_id`

func scanAccess(r pgx.CollectableRow) (AccessRequest, error) {
	var a AccessRequest
	err := r.Scan(&a.ID, &a.Subject, &a.Person, &a.AppID, &a.AppName, &a.Note, &a.Status,
		&a.Group, &a.DecidedBy, &a.CreatedAt, &a.DecidedAt)
	return a, err
}

func (s *Store) accessRequest(ctx context.Context, q querier, where string, args ...any) (AccessRequest, error) {
	rows, err := q.Query(ctx, selectAccess+where, args...)
	if err != nil {
		return AccessRequest{}, err
	}
	a, err := pgx.CollectExactlyOneRow(rows, scanAccess)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccessRequest{}, ErrNotFound
	}
	return a, err
}

// RequestAccess asks for access to an app the viewer cannot see. An open
// request for the same app is returned as is.
func (s *Store) RequestAccess(ctx context.Context, v Viewer, appID, note string) (AccessRequest, error) {
	_, err := s.Get(ctx, v, appID)
	var noAccess *NoAccessError
	if !errors.As(err, &noAccess) {
		if err == nil {
			err = ErrNotFound
		}
		return AccessRequest{}, err
	}
	note = strings.TrimSpace(note)
	if r := []rune(note); len(r) > 500 {
		note = strings.TrimSpace(string(r[:500]))
	}
	var out AccessRequest
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO access_requests (subject, app_id, note) VALUES ($1, $2, $3)
			ON CONFLICT (subject, app_id) WHERE status = 'pending' DO NOTHING`, v.Subject, appID, note)
		if err != nil {
			return dbErr(err)
		}
		out, err = s.accessRequest(ctx, tx, ` WHERE r.subject = $1 AND r.app_id = $2 AND r.status = 'pending'`,
			v.Subject, appID)
		return err
	})
	return out, err
}

// LatestAccessRequest is nil when the person never asked for the app.
func (s *Store) LatestAccessRequest(ctx context.Context, subject, appID string) (*AccessRequest, error) {
	a, err := s.accessRequest(ctx, s.pool, ` WHERE r.subject = $1 AND r.app_id = $2
		ORDER BY r.created_at DESC, r.id DESC LIMIT 1`, subject, appID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// AccessRequests lists the pending requests, oldest first. Requests from
// people who can already see the app are left out.
func (s *Store) AccessRequests(ctx context.Context) ([]AccessRequest, error) {
	rows, err := s.pool.Query(ctx, selectAccess+` WHERE r.status = 'pending' AND NOT `+subjectSeesApp("r.subject")+`
		ORDER BY r.created_at, r.id`)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, scanAccess)
	if out == nil {
		out = []AccessRequest{}
	}
	return out, err
}

// pendingAccess locks a request and checks that it is still open.
func (s *Store) pendingAccess(ctx context.Context, tx pgx.Tx, id int64) (AccessRequest, error) {
	a, err := s.accessRequest(ctx, tx, ` WHERE r.id = $1 FOR UPDATE OF r`, id)
	if err != nil {
		return AccessRequest{}, err
	}
	if a.Status != "pending" {
		return AccessRequest{}, ErrDecided
	}
	return a, nil
}

// ApproveAccess adds the person to a group the app is shared with.
func (s *Store) ApproveAccess(ctx context.Context, by Actor, id int64, group string) (AccessRequest, error) {
	var out AccessRequest
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		a, err := s.pendingAccess(ctx, tx, id)
		if err != nil {
			return err
		}
		var shared bool
		err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_groups WHERE app_id = $1 AND group_name = $2)`,
			a.AppID, group).Scan(&shared)
		if err != nil {
			return err
		}
		if !shared {
			return ErrNotShared
		}
		if _, err := tx.Exec(ctx, `INSERT INTO group_members (group_name, subject) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, group, a.Subject); err != nil {
			return dbErr(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE access_requests
			SET status = 'approved', group_name = $2, decided_at = now(), decided_by = $3 WHERE id = $1`,
			id, group, by.Name); err != nil {
			return err
		}
		if err := logChange(ctx, tx, by, "gave "+a.Person+" access to "+a.AppName+" through "+group); err != nil {
			return err
		}
		out, err = s.accessRequest(ctx, tx, ` WHERE r.id = $1`, id)
		return err
	})
	return out, err
}

// DeclineAccess closes a pending request without giving access.
func (s *Store) DeclineAccess(ctx context.Context, by Actor, id int64) (AccessRequest, error) {
	var out AccessRequest
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		a, err := s.pendingAccess(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE access_requests
			SET status = 'declined', decided_at = now(), decided_by = $2 WHERE id = $1`, id, by.Name); err != nil {
			return err
		}
		if err := logChange(ctx, tx, by, "declined "+a.Person+"'s request for "+a.AppName); err != nil {
			return err
		}
		out, err = s.accessRequest(ctx, tx, ` WHERE r.id = $1`, id)
		return err
	})
	return out, err
}
