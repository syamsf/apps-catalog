// Package store is the postgres storage for the catalog.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

var (
	ErrNotFound = errors.New("not found")
	// ErrConflict is a duplicate id or name.
	ErrConflict = errors.New("already exists")
	// ErrInUse is an option that apps still use.
	ErrInUse = errors.New("still in use")
	// ErrUnknownRef is a group, user or app that does not exist.
	ErrUnknownRef = errors.New("unknown group, user or app")
	// ErrDecided is an access request that is no longer pending.
	ErrDecided = errors.New("already decided")
	// ErrNotShared is a group the app is not shared with.
	ErrNotShared = errors.New("app not shared with that group")
)

const (
	VisibilityEveryone = "everyone"
	VisibilityGroups   = "groups"
)

type App struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Owner       string            `json:"owner"`
	Contact     string            `json:"contact"`
	Category    string            `json:"category"`
	RunsOn      string            `json:"runs_on"`
	Version     string            `json:"version"`
	StartHere   bool              `json:"start_here"`
	Access      string            `json:"access"`
	HealthURL   string            `json:"health_url"`
	URLs        map[string]string `json:"urls"`
	Links       map[string]string `json:"links"`
	// everyone or groups
	Visibility string   `json:"visibility"`
	Groups     []string `json:"groups"`
	IconColor  string   `json:"icon_color"`
	// Tabler icon name, used when no icon is uploaded
	Icon        string `json:"icon"`
	HasIcon     bool   `json:"has_icon"`
	IconVersion int    `json:"icon_version"`
	Favorite    bool   `json:"favorite"`
	// the viewer is an admin or in the owner group
	CanEdit bool `json:"can_edit"`
	// opens by everyone in the last 30 days
	Opens30d  int       `json:"opens_30d"`
	CreatedBy string    `json:"created_by"`
	UpdatedBy string    `json:"updated_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Status    *Status   `json:"status"`
}

// AppInput is what admins can set on an app.
type AppInput struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Owner       string            `json:"owner"`
	Contact     string            `json:"contact"`
	Category    string            `json:"category"`
	RunsOn      string            `json:"runs_on"`
	Version     string            `json:"version"`
	StartHere   bool              `json:"start_here"`
	Access      string            `json:"access"`
	HealthURL   string            `json:"health_url"`
	URLs        map[string]string `json:"urls"`
	Links       map[string]string `json:"links"`
	Visibility  string            `json:"visibility"`
	Groups      []string          `json:"groups"`
	IconColor   string            `json:"icon_color"`
	Icon        string            `json:"icon"`
}

type Status struct {
	Up         bool      `json:"up"`
	HTTPStatus int       `json:"http_status"`
	LatencyMS  int       `json:"latency_ms"`
	Error      string    `json:"error"`
	Target     string    `json:"target"`
	CheckedAt  time.Time `json:"checked_at"`
	Since      time.Time `json:"since"`
}

type Filter struct {
	Query string
	// match any of these keys, empty means no filter
	Category []string
	RunsOn   []string
	// group name, or "everyone" for apps shared with everyone
	Group string
	// exact owner group
	Owner string
}

// Viewer decides which apps a request can see. The zero value only sees
// apps shared with everyone.
type Viewer struct {
	Subject string
	All     bool
	// portal groups
	Groups []string
}

// Actor is who made a change, for the change log.
type Actor struct {
	Subject string
	Name    string
}

// NoAccessError is returned by Get for an app that exists but is hidden
// from the viewer.
type NoAccessError struct {
	ID     string
	Name   string
	Groups []string
}

func (e *NoAccessError) Error() string { return "no access to " + e.ID }

type Store struct{ pool *pgxpool.Pool }

// Open connects and runs migrations.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	if err := migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, mustSub(migrations, "migrations"))
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func (s *Store) Close()                         { s.pool.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// $1 is always the viewer subject, for the favorite flag
const selectApp = `
	SELECT a.id, a.name, a.description, coalesce(a.owner, ''), a.contact, a.category, a.runs_on, a.version, a.start_here,
	       a.access, a.health_url, a.urls, a.links, a.visibility,
	       coalesce((SELECT array_agg(g.group_name ORDER BY g.group_name) FROM app_groups g WHERE g.app_id = a.id), '{}'),
	       a.icon_color, a.icon, i.version,
	       EXISTS (SELECT 1 FROM favorites f WHERE f.app_id = a.id AND f.subject = $1),
	       (SELECT count(*) FROM opens o WHERE o.app_id = a.id AND o.at > now() - interval '30 days'),
	       a.created_by, a.updated_by, a.created_at, a.updated_at,
	       s.up, s.http_status, s.latency_ms, s.error, s.target, s.checked_at, s.since
	FROM apps a
	LEFT JOIN app_status s ON s.app_id = a.id
	LEFT JOIN app_icons i ON i.app_id = a.id`

// visibleSQL is the check that the viewer can see app a. Members of the
// owner group always see it.
func visibleSQL(v Viewer, args []any) (string, []any) {
	if v.All {
		return "true", args
	}
	args = append(args, nonNilSlice(v.Groups))
	return fmt.Sprintf(`(a.visibility = 'everyone' OR a.owner = ANY($%[1]d) OR EXISTS (
		SELECT 1 FROM app_groups g WHERE g.app_id = a.id AND g.group_name = ANY($%[1]d)))`, len(args)), args
}

func canEdit(v Viewer, app App) bool {
	return v.All || (app.Owner != "" && slices.Contains(v.Groups, app.Owner))
}

func (s *Store) List(ctx context.Context, v Viewer, f Filter) ([]App, error) {
	args := []any{v.Subject}
	vis, args := visibleSQL(v, args)
	where := []string{vis}
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+escapeLike(q)+"%")
		where = append(where, fmt.Sprintf(
			"(a.id ILIKE $%[1]d OR a.name ILIKE $%[1]d OR a.description ILIKE $%[1]d OR a.owner ILIKE $%[1]d OR a.urls::text ILIKE $%[1]d)",
			len(args)))
	}
	if len(f.Category) > 0 {
		args = append(args, f.Category)
		where = append(where, fmt.Sprintf("a.category = ANY($%d)", len(args)))
	}
	if len(f.RunsOn) > 0 {
		args = append(args, f.RunsOn)
		where = append(where, fmt.Sprintf("a.runs_on = ANY($%d)", len(args)))
	}
	switch f.Group {
	case "":
	case VisibilityEveryone:
		where = append(where, "a.visibility = 'everyone'")
	default:
		args = append(args, f.Group)
		where = append(where, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM app_groups g WHERE g.app_id = a.id AND g.group_name = $%d)", len(args)))
	}
	if f.Owner != "" {
		args = append(args, f.Owner)
		where = append(where, fmt.Sprintf("a.owner = $%d", len(args)))
	}

	rows, err := s.pool.Query(ctx, selectApp+" WHERE "+strings.Join(where, " AND ")+" ORDER BY lower(a.name), a.id", args...)
	if err != nil {
		return nil, err
	}
	apps, err := pgx.CollectRows(rows, scanApp)
	if apps == nil {
		apps = []App{}
	}
	for i := range apps {
		apps[i].CanEdit = canEdit(v, apps[i])
	}
	return apps, err
}

// Get returns ErrNotFound for missing apps and *NoAccessError for apps
// the viewer can't see.
func (s *Store) Get(ctx context.Context, v Viewer, id string) (App, error) {
	app, err := s.get(ctx, s.pool, v.Subject, id)
	if err != nil {
		return App{}, err
	}
	app.CanEdit = canEdit(v, app)
	if app.CanEdit || app.Visibility == VisibilityEveryone || overlaps(app.Groups, v.Groups) {
		return app, nil
	}
	return App{}, &NoAccessError{ID: app.ID, Name: app.Name, Groups: app.Groups}
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (s *Store) get(ctx context.Context, q querier, subject, id string) (App, error) {
	rows, err := q.Query(ctx, selectApp+" WHERE a.id = $2", subject, id)
	if err != nil {
		return App{}, err
	}
	app, err := pgx.CollectExactlyOneRow(rows, scanApp)
	if errors.Is(err, pgx.ErrNoRows) {
		return App{}, ErrNotFound
	}
	return app, err
}

// CreateApp adds an app. ErrConflict when the id is taken.
func (s *Store) CreateApp(ctx context.Context, by Actor, in AppInput) (App, error) {
	var app App
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO apps (id, name, description, owner, contact, category, runs_on, access, health_url,
			                  urls, links, visibility, icon_color, created_by, updated_by, version, start_here, icon)
			VALUES ($1, $2, $3, nullif($4, ''), $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14, $15, $16, $17)`,
			in.ID, in.Name, in.Description, in.Owner, in.Contact, in.Category, in.RunsOn, in.Access, in.HealthURL,
			nonNil(in.URLs), nonNil(in.Links), in.Visibility, in.IconColor, by.Name, in.Version, in.StartHere, in.Icon)
		if err != nil {
			return dbErr(err)
		}
		if err := setAppGroups(ctx, tx, in.ID, in.Groups); err != nil {
			return err
		}
		if err := logChange(ctx, tx, by, "added the app "+in.Name+sharedWith(in)); err != nil {
			return err
		}
		app, err = s.get(ctx, tx, by.Subject, in.ID)
		return err
	})
	return app, err
}

func sharedWith(in AppInput) string {
	switch {
	case in.Visibility == VisibilityEveryone:
		return " and shared it with everyone"
	case len(in.Groups) == 0:
		return ", not shared with any group yet"
	default:
		return " and shared it with " + joinAnd(in.Groups)
	}
}

// UpdateApp changes everything but the id.
func (s *Store) UpdateApp(ctx context.Context, by Actor, id string, in AppInput) (App, error) {
	var app App
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE apps SET name = $2, description = $3, owner = nullif($4, ''), contact = $5, category = $6, runs_on = $7,
			       access = $8, health_url = $9, urls = $10, links = $11, visibility = $12, icon_color = $13,
			       updated_by = $14, updated_at = now(), version = $15, start_here = $16, icon = $17
			WHERE id = $1`,
			id, in.Name, in.Description, in.Owner, in.Contact, in.Category, in.RunsOn, in.Access, in.HealthURL,
			nonNil(in.URLs), nonNil(in.Links), in.Visibility, in.IconColor, by.Name, in.Version, in.StartHere, in.Icon)
		if err != nil {
			return dbErr(err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM app_groups WHERE app_id = $1`, id); err != nil {
			return err
		}
		if err := setAppGroups(ctx, tx, id, in.Groups); err != nil {
			return err
		}
		if err := logChange(ctx, tx, by, "edited the app "+in.Name); err != nil {
			return err
		}
		app, err = s.get(ctx, tx, by.Subject, id)
		return err
	})
	return app, err
}

func setAppGroups(ctx context.Context, tx pgx.Tx, id string, groups []string) error {
	if len(groups) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO app_groups (app_id, group_name) SELECT $1, unnest($2::text[]) ON CONFLICT DO NOTHING`,
		id, groups)
	return dbErr(err)
}

func (s *Store) DeleteApp(ctx context.Context, by Actor, id string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `DELETE FROM apps WHERE id = $1 RETURNING name`, id).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		return logChange(ctx, tx, by, "deleted the app "+name)
	})
}

// SetIcon stores an uploaded icon and bumps its version.
func (s *Store) SetIcon(ctx context.Context, by Actor, id, contentType string, data []byte) (App, error) {
	var app App
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `UPDATE apps SET updated_by = $2, updated_at = now() WHERE id = $1 RETURNING name`,
			id, by.Name).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO app_icons (app_id, content_type, data) VALUES ($1, $2, $3)
			ON CONFLICT (app_id) DO UPDATE SET content_type = EXCLUDED.content_type, data = EXCLUDED.data,
				version = app_icons.version + 1, updated_at = now()`, id, contentType, data)
		if err != nil {
			return err
		}
		if err := logChange(ctx, tx, by, "uploaded a new icon for "+name); err != nil {
			return err
		}
		app, err = s.get(ctx, tx, by.Subject, id)
		return err
	})
	return app, err
}

func (s *Store) DeleteIcon(ctx context.Context, by Actor, id string) (App, error) {
	var app App
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `SELECT name FROM apps WHERE id = $1`, id).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM app_icons WHERE app_id = $1`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			if err := logChange(ctx, tx, by, "removed the icon of "+name); err != nil {
				return err
			}
		}
		app, err = s.get(ctx, tx, by.Subject, id)
		return err
	})
	return app, err
}

// Icon returns the uploaded icon of an app the viewer can see.
func (s *Store) Icon(ctx context.Context, v Viewer, id string) (contentType string, data []byte, err error) {
	vis, args := visibleSQL(v, []any{id})
	err = s.pool.QueryRow(ctx, `
		SELECT i.content_type, i.data FROM app_icons i JOIN apps a ON a.id = i.app_id
		WHERE i.app_id = $1 AND `+vis, args...).Scan(&contentType, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, ErrNotFound
	}
	return contentType, data, err
}

// SetFavorite stars or unstars an app the viewer can see.
func (s *Store) SetFavorite(ctx context.Context, v Viewer, id string, on bool) error {
	if _, err := s.Get(ctx, v, id); err != nil {
		var na *NoAccessError
		if errors.As(err, &na) {
			return ErrNotFound
		}
		return err
	}
	var err error
	if on {
		_, err = s.pool.Exec(ctx, `INSERT INTO favorites (subject, app_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, v.Subject, id)
	} else {
		_, err = s.pool.Exec(ctx, `DELETE FROM favorites WHERE subject = $1 AND app_id = $2`, v.Subject, id)
	}
	return err
}

func scanApp(r pgx.CollectableRow) (App, error) {
	var (
		a       App
		version *int
		up      *bool
		st      Status
		// nullable because of the left join
		httpStatus, latency *int
		errText, target     *string
		checkedAt, since    *time.Time
	)
	err := r.Scan(&a.ID, &a.Name, &a.Description, &a.Owner, &a.Contact, &a.Category, &a.RunsOn, &a.Version,
		&a.StartHere, &a.Access, &a.HealthURL, &a.URLs, &a.Links, &a.Visibility, &a.Groups, &a.IconColor, &a.Icon, &version, &a.Favorite, &a.Opens30d,
		&a.CreatedBy, &a.UpdatedBy, &a.CreatedAt, &a.UpdatedAt,
		&up, &httpStatus, &latency, &errText, &target, &checkedAt, &since)
	if err != nil {
		return a, err
	}
	a.URLs, a.Links = nonNil(a.URLs), nonNil(a.Links)
	a.Groups = nonNilSlice(a.Groups)
	if version != nil {
		a.HasIcon, a.IconVersion = true, *version
	}
	if up != nil {
		st.Up, st.HTTPStatus, st.LatencyMS = *up, *httpStatus, *latency
		st.Error, st.Target, st.CheckedAt, st.Since = *errText, *target, *checkedAt, *since
		a.Status = &st
	}
	return a, nil
}

// dbErr maps constraint errors to ErrConflict and ErrUnknownRef.
func dbErr(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23505":
			return ErrConflict
		case "23503":
			return ErrUnknownRef
		}
	}
	return err
}

func overlaps(a, b []string) bool {
	return slices.ContainsFunc(a, func(x string) bool { return slices.Contains(b, x) })
}

// joinAnd gives "a", "a and b", "a, b and c".
func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func nonNilSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLike(s string) string { return likeEscaper.Replace(s) }
