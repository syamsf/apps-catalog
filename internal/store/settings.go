package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	KindCategory    = "category"
	KindRunsOn      = "runs_on"
	KindEnvironment = "environment"
)

var kindNames = map[string]string{
	KindCategory:    "category",
	KindRunsOn:      "runs-on option",
	KindEnvironment: "environment",
}

func ValidKind(kind string) bool { _, ok := kindNames[kind]; return ok }

// IconPalette is the set of colors for default icons.
var IconPalette = []string{"#2251C7", "#0F6E6E", "#5B3FB8", "#9A3D06", "#17663A", "#474C55"}

type Option struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// apps using it
	Count int `json:"count"`
}

type Settings struct {
	Name             string   `json:"name"`
	AdminEmail       string   `json:"admin_email"`
	AdminGroup       string   `json:"admin_group"`
	IconPalette      []string `json:"icon_palette"`
	IconColorMode    string   `json:"icon_color_mode"`
	NewAppVisibility string   `json:"new_app_visibility"`
	Categories       []Option `json:"categories"`
	RunsOn           []Option `json:"runs_on"`
	Environments     []Option `json:"environments"`
}

type SettingsInput struct {
	Name             string `json:"name"`
	AdminEmail       string `json:"admin_email"`
	IconColorMode    string `json:"icon_color_mode"`
	NewAppVisibility string `json:"new_app_visibility"`
}

// countSQL counts the apps using option o.key.
var countSQL = map[string]string{
	KindCategory:    `a.category = o.key`,
	KindRunsOn:      `a.runs_on = o.key`,
	KindEnvironment: `a.urls ? o.key`,
}

// Options lists one kind in order, counting the apps v can see.
func (s *Store) Options(ctx context.Context, v Viewer, kind string) ([]Option, error) {
	vis, args := visibleSQL(v, []any{kind})
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT o.key, o.label, (SELECT count(*) FROM apps a WHERE %s AND %s)
		FROM options o WHERE o.kind = $1 ORDER BY o.position, o.key`, countSQL[kind], vis), args...)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Option, error) {
		var o Option
		return o, r.Scan(&o.Key, &o.Label, &o.Count)
	})
	if out == nil {
		out = []Option{}
	}
	return out, err
}

// Settings returns the portal settings with option counts over all apps.
func (s *Store) Settings(ctx context.Context) (Settings, error) {
	st := Settings{IconPalette: IconPalette}
	err := s.pool.QueryRow(ctx, `SELECT name, admin_email, icon_color_mode, new_app_visibility FROM settings`).
		Scan(&st.Name, &st.AdminEmail, &st.IconColorMode, &st.NewAppVisibility)
	if err != nil {
		return st, err
	}
	all := Viewer{All: true}
	if st.Categories, err = s.Options(ctx, all, KindCategory); err != nil {
		return st, err
	}
	if st.RunsOn, err = s.Options(ctx, all, KindRunsOn); err != nil {
		return st, err
	}
	st.Environments, err = s.Options(ctx, all, KindEnvironment)
	return st, err
}

func (s *Store) UpdateSettings(ctx context.Context, by Actor, in SettingsInput) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE settings SET name = $1, admin_email = $2, icon_color_mode = $3, new_app_visibility = $4`,
			in.Name, in.AdminEmail, in.IconColorMode, in.NewAppVisibility)
		if err != nil {
			return err
		}
		return logChange(ctx, tx, by, "changed the portal settings")
	})
}

// AddOption appends an option. ErrConflict when the key exists.
func (s *Store) AddOption(ctx context.Context, by Actor, kind, key, label string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO options (kind, key, label, position)
			SELECT $1, $2, $3, coalesce(max(position), 0) + 1 FROM options WHERE kind = $1`, kind, key, label)
		if err != nil {
			return dbErr(err)
		}
		return logChange(ctx, tx, by, "added the "+kindNames[kind]+" "+label)
	})
}

// RenameOption changes the label. The key stays, apps keep pointing at it.
func (s *Store) RenameOption(ctx context.Context, by Actor, kind, key, label string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var old string
		err := tx.QueryRow(ctx, `SELECT label FROM options WHERE kind = $1 AND key = $2`, kind, key).Scan(&old)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE options SET label = $3 WHERE kind = $1 AND key = $2`, kind, key, label); err != nil {
			return err
		}
		return logChange(ctx, tx, by, "renamed the "+kindNames[kind]+" "+old+" to "+label)
	})
}

// ReorderOptions sets the order of all options of a kind. ErrConflict when
// keys is not exactly the current set, e.g. someone added one meanwhile.
func (s *Store) ReorderOptions(ctx context.Context, by Actor, kind string, keys []string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var have []string
		rows, err := tx.Query(ctx, `SELECT key FROM options WHERE kind = $1 FOR UPDATE`, kind)
		if err != nil {
			return err
		}
		if have, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return err
		}
		want := slices.Clone(keys)
		slices.Sort(have)
		slices.Sort(want)
		want = slices.Compact(want)
		if len(want) != len(keys) || !slices.Equal(have, want) {
			return ErrConflict
		}
		if _, err := tx.Exec(ctx, `
			UPDATE options o SET position = k.n
			FROM unnest($2::text[]) WITH ORDINALITY AS k(key, n)
			WHERE o.kind = $1 AND o.key = k.key`, kind, keys); err != nil {
			return err
		}
		return logChange(ctx, tx, by, "reordered the "+kindNames[kind]+" list")
	})
}

// DeleteOption removes an option. ErrInUse when apps still use it.
func (s *Store) DeleteOption(ctx context.Context, by Actor, kind, key string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var label string
		var used int
		err := tx.QueryRow(ctx, fmt.Sprintf(`
			SELECT o.label, (SELECT count(*) FROM apps a WHERE %s) FROM options o WHERE o.kind = $1 AND o.key = $2`,
			countSQL[kind]), kind, key).Scan(&label, &used)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if used > 0 {
			return ErrInUse
		}
		if _, err := tx.Exec(ctx, `DELETE FROM options WHERE kind = $1 AND key = $2`, kind, key); err != nil {
			return err
		}
		return logChange(ctx, tx, by, "removed the "+kindNames[kind]+" "+label)
	})
}

type Change struct {
	ID      int64     `json:"id"`
	At      time.Time `json:"at"`
	Actor   string    `json:"actor"`
	Summary string    `json:"summary"`
}

func logChange(ctx context.Context, tx pgx.Tx, by Actor, summary string) error {
	_, err := tx.Exec(ctx, `INSERT INTO changes (actor_subject, actor, summary) VALUES ($1, $2, $3)`,
		by.Subject, by.Name, summary)
	return err
}

func (s *Store) Changes(ctx context.Context, limit int) ([]Change, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, at, actor, summary FROM changes ORDER BY at DESC, id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Change])
	if out == nil {
		out = []Change{}
	}
	return out, err
}

type Attention struct {
	// user_no_group, app_no_groups, app_no_owner, app_no_contact, group_no_members
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	// user subject, app id or group name
	Target string `json:"target"`
}

type Dashboard struct {
	Apps         int            `json:"apps"`
	AppsEveryone int            `json:"apps_everyone"`
	Groups       int            `json:"groups"`
	Users        int            `json:"users"`
	Admins       int            `json:"admins"`
	DefaultIcons int            `json:"default_icons"`
	Attention    []Attention    `json:"attention"`
	Changes      []Change       `json:"changes"`
	Usage        Usage          `json:"usage"`
	Missed       []MissedSearch `json:"missed"`
}

const maxAttention = 10

func (s *Store) Dashboard(ctx context.Context) (Dashboard, error) {
	var d Dashboard
	err := s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM apps),
		       (SELECT count(*) FROM apps WHERE visibility = 'everyone'),
		       (SELECT count(*) FROM groups),
		       (SELECT count(*) FROM users),
		       (SELECT count(*) FROM users WHERE is_admin),
		       (SELECT count(*) FROM apps a WHERE NOT EXISTS (SELECT 1 FROM app_icons i WHERE i.app_id = a.id))`).
		Scan(&d.Apps, &d.AppsEveryone, &d.Groups, &d.Users, &d.Admins, &d.DefaultIcons)
	if err != nil {
		return d, err
	}

	rows, err := s.pool.Query(ctx, `
		(SELECT 'user_no_group', coalesce(nullif(u.name, ''), u.username), u.subject FROM users u
		 WHERE NOT u.is_admin AND NOT EXISTS (SELECT 1 FROM group_members m WHERE m.subject = u.subject)
		 ORDER BY u.last_login DESC NULLS LAST LIMIT $1)
		UNION ALL
		(SELECT 'app_no_groups', a.name, a.id FROM apps a
		 WHERE a.visibility = 'groups' AND NOT EXISTS (SELECT 1 FROM app_groups g WHERE g.app_id = a.id)
		 ORDER BY lower(a.name) LIMIT $1)
		UNION ALL
		(SELECT 'app_no_owner', a.name, a.id FROM apps a WHERE a.owner IS NULL
		 ORDER BY lower(a.name) LIMIT $1)
		UNION ALL
		(SELECT 'app_no_contact', a.name, a.id FROM apps a WHERE trim(a.contact) = ''
		 ORDER BY lower(a.name) LIMIT $1)
		UNION ALL
		(SELECT 'group_no_members', g.name, g.name FROM groups g
		 WHERE NOT EXISTS (SELECT 1 FROM group_members m WHERE m.group_name = g.name)
		 ORDER BY g.name LIMIT $1)`, maxAttention)
	if err != nil {
		return d, err
	}
	d.Attention, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Attention, error) {
		var a Attention
		var name string
		if err := r.Scan(&a.Kind, &name, &a.Target); err != nil {
			return a, err
		}
		switch a.Kind {
		case "user_no_group":
			a.Title = name + " is not in any group"
			a.Detail = fmt.Sprintf("They only see the %s shared with everyone.", plural(d.AppsEveryone, "app"))
		case "app_no_groups":
			a.Title = name + " is not shared with any group"
			a.Detail = "Only admins can see it."
		case "app_no_owner":
			a.Title = name + " has no owner team"
			a.Detail = "Nobody outside the admins can edit it."
		case "app_no_contact":
			a.Title = name + " has no contact"
			a.Detail = "Users who lose access have nobody to ask."
		case "group_no_members":
			a.Title = name + " has no members"
			a.Detail = "Add people so its apps show up for them."
		}
		return a, nil
	})
	if err != nil {
		return d, err
	}
	if len(d.Attention) > maxAttention {
		d.Attention = d.Attention[:maxAttention]
	}
	if d.Attention == nil {
		d.Attention = []Attention{}
	}
	if d.Changes, err = s.Changes(ctx, 10); err != nil {
		return d, err
	}
	if d.Usage, err = s.usage(ctx); err != nil {
		return d, err
	}
	d.Missed, err = s.missedSearches(ctx)
	return d, err
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// OptionKey makes an option key from a label: "Cloud Run" -> "cloud-run".
func OptionKey(label string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(label) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		} else {
			dash = true
		}
	}
	return b.String()
}
