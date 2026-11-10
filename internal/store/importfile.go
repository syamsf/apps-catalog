package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"apps-catalog/internal/catalog"
)

// ErrImportInvalid means the file has problems, nothing was imported.
var ErrImportInvalid = errors.New("the file has problems, nothing was imported")

type ImportOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// ImportCreate is what an import adds besides apps.
type ImportCreate struct {
	Categories   []ImportOption `json:"categories"`
	RunsOn       []ImportOption `json:"runs_on"`
	Environments []ImportOption `json:"environments"`
	Groups       []string       `json:"groups"`
}

type ImportApp struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Line       int      `json:"line"`
	Exists     bool     `json:"exists"`
	Problems   int      `json:"problems"`
	Visibility string   `json:"visibility"`
	Groups     []string `json:"groups"`
	IconColor  string   `json:"icon_color"`
	Icon       string   `json:"icon"`
	// set when the owner is not a group
	OwnerNote string `json:"owner_note"`
}

// ImportPlan is what an import file would do. Nothing is saved.
type ImportPlan struct {
	Apps     []ImportApp       `json:"apps"`
	Create   ImportCreate      `json:"create"`
	Problems []catalog.Problem `json:"problems"`

	inputs []AppInput
}

// IconColors are the names an import file can use for icon_color.
var IconColors = map[string]string{
	"blue": IconPalette[0], "teal": IconPalette[1], "purple": IconPalette[2],
	"orange": IconPalette[3], "green": IconPalette[4], "gray": IconPalette[5], "grey": IconPalette[5],
}

// PlanImport checks an import file against the portal. Categories, runs on
// options, environments and groups it doesn't know are listed to be created.
func (s *Store) PlanImport(ctx context.Context, data []byte) (ImportPlan, error) {
	entries, probs := catalog.ParseFile(data)
	plan := ImportPlan{
		Apps:     []ImportApp{},
		Problems: probs,
		Create:   ImportCreate{Categories: []ImportOption{}, RunsOn: []ImportOption{}, Environments: []ImportOption{}, Groups: []string{}},
	}
	if plan.Problems == nil {
		plan.Problems = []catalog.Problem{}
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return plan, err
	}
	rows, err := s.pool.Query(ctx, `SELECT name FROM groups`)
	if err != nil {
		return plan, err
	}
	groups, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return plan, err
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, strings.TrimSpace(e.ID))
	}
	// id to current owner
	existing := map[string]string{}
	rows, err = s.pool.Query(ctx, `SELECT id, coalesce(owner, '') FROM apps WHERE id = ANY($1)`, ids)
	if err != nil {
		return plan, err
	}
	var id, owner string
	if _, err := pgx.ForEachRow(rows, []any{&id, &owner}, func() error {
		existing[id] = owner
		return nil
	}); err != nil {
		return plan, err
	}

	for _, e := range entries {
		in, p := plan.resolve(&st, groups, e)
		if fields := ValidateApp(st, &in, true); fields != nil {
			for _, f := range sortedFields(fields) {
				p = append(p, catalog.Problem{Line: fieldLine(e, f), App: in.ID, Field: f, Message: importMessage(f, fields[f])})
			}
		}
		plan.Problems = append(plan.Problems, p...)
		_, exists := existing[in.ID]
		plan.inputs = append(plan.inputs, in)
		plan.Apps = append(plan.Apps, ImportApp{
			ID: in.ID, Name: in.Name, Line: e.Line, Exists: exists, Problems: len(p),
			Visibility: in.Visibility, Groups: nonNilSlice(in.Groups), IconColor: in.IconColor,
			Icon: in.Icon,
		})
	}
	// owners are matched last, a group can be created further down the file
	for i := range plan.inputs {
		in := &plan.inputs[i]
		if in.Owner == "" {
			continue
		}
		if g := strings.ToLower(in.Owner); slices.Contains(groups, g) || slices.Contains(plan.Create.Groups, g) {
			in.Owner = g
			continue
		}
		if cur, ok := existing[in.ID]; ok {
			plan.Apps[i].OwnerNote = fmt.Sprintf("Team %q does not exist, the owner stays %s", in.Owner, cmp.Or(cur, "empty"))
		} else {
			plan.Apps[i].OwnerNote = fmt.Sprintf("Team %q does not exist, the app will have no owner", in.Owner)
		}
		in.Owner = ""
	}
	slices.SortStableFunc(plan.Problems, func(a, b catalog.Problem) int { return a.Line - b.Line })
	return plan, nil
}

// resolve turns an entry into an AppInput, matching option names by key or
// label and adding the unknown ones to st and the create lists.
func (p *ImportPlan) resolve(st *Settings, groups []string, e catalog.Entry) (AppInput, []catalog.Problem) {
	in := AppInput{
		ID: strings.TrimSpace(e.ID), Name: e.Name, Description: e.Description, Owner: e.Owner, Contact: e.Contact,
		Version: e.Version, StartHere: e.StartHere, Icon: strings.ToLower(e.Icon), Access: e.Access, HealthURL: e.HealthURL,
		URLs: map[string]string{}, Links: map[string]string{},
	}
	var probs []catalog.Problem
	bad := func(field, msg string) {
		probs = append(probs, catalog.Problem{Line: fieldLine(e, field), App: in.ID, Field: field, Message: msg})
	}

	if e.Category != "" {
		if key, ok := resolveOption(&st.Categories, &p.Create.Categories, e.Category); ok {
			in.Category = key
		} else {
			bad("category", fmt.Sprintf("Category %q needs at least one letter or number", e.Category))
		}
	}
	if e.RunsOn != "" {
		if key, ok := resolveOption(&st.RunsOn, &p.Create.RunsOn, e.RunsOn); ok {
			in.RunsOn = key
		} else {
			bad("runs_on", fmt.Sprintf("Runs on %q needs at least one letter or number", e.RunsOn))
		}
	}
	for _, name := range sortedFields(e.URLs) {
		key, ok := resolveOption(&st.Environments, &p.Create.Environments, name)
		switch {
		case !ok:
			bad("urls."+name, fmt.Sprintf("Environment %q needs at least one letter or number", name))
		case in.URLs[key] != "":
			bad("urls."+name, fmt.Sprintf("Environment %q is listed twice", key))
		default:
			in.URLs[key] = e.URLs[name]
		}
	}
	for name, u := range e.Links {
		in.Links[strings.ToLower(strings.TrimSpace(name))] = u
	}

	switch {
	case e.VisibleTo == nil:
		in.Visibility = st.NewAppVisibility
	case len(e.VisibleTo) == 1 && strings.EqualFold(strings.TrimSpace(e.VisibleTo[0]), VisibilityEveryone):
		in.Visibility = VisibilityEveryone
	default:
		in.Visibility = VisibilityGroups
		for _, g := range e.VisibleTo {
			g = strings.ToLower(strings.TrimSpace(g))
			switch {
			case g == VisibilityEveryone:
				bad("visible_to", "Use everyone on its own, not in a list with groups")
			case !catalog.ValidID(g):
				bad("visible_to", fmt.Sprintf("Group %q must be lowercase letters, numbers and single dashes", g))
			default:
				in.Groups = append(in.Groups, g)
				if !slices.Contains(groups, g) && !slices.Contains(p.Create.Groups, g) {
					p.Create.Groups = append(p.Create.Groups, g)
				}
			}
		}
	}

	if c := strings.ToLower(strings.TrimSpace(e.IconColor)); c != "" && c != "auto" {
		if hex, ok := IconColors[c]; ok {
			in.IconColor = hex
		} else if i := slices.IndexFunc(IconPalette, func(h string) bool { return strings.EqualFold(h, c) }); i >= 0 {
			in.IconColor = IconPalette[i]
		} else {
			bad("icon_color", "Use auto, blue, teal, purple, orange, green or gray")
		}
	}
	return in, probs
}

var notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// resolveOption finds value by key or label. Unknown values get a key made
// from the value and are added to opts and created.
func resolveOption(opts *[]Option, created *[]ImportOption, value string) (string, bool) {
	v := strings.TrimSpace(value)
	for _, o := range *opts {
		if strings.EqualFold(o.Key, v) || strings.EqualFold(o.Label, v) {
			return o.Key, true
		}
	}
	key := strings.Trim(notSlug.ReplaceAllString(strings.ToLower(v), "-"), "-")
	if !catalog.ValidID(key) {
		return "", false
	}
	if hasOption(*opts, key) {
		return key, true
	}
	label := v
	if v == key {
		label = labelFor(key)
	}
	*opts = append(*opts, Option{Key: key, Label: label})
	*created = append(*created, ImportOption{Key: key, Label: label})
	return key, true
}

func importMessage(field, msg string) string {
	switch {
	case msg == "Unknown link":
		return fmt.Sprintf("Unknown link %q, use repo, docs or ci", strings.TrimPrefix(field, "links."))
	case field == "id":
		return "ID must be lowercase letters, numbers and single dashes"
	case field == "icon":
		return "Unknown icon, use a name from tabler.io/icons like brand-docker"
	}
	return msg
}

func fieldLine(e catalog.Entry, field string) int {
	if l, ok := e.Lines[field]; ok {
		return l
	}
	if i := strings.IndexByte(field, '.'); i > 0 {
		if l, ok := e.Lines[field[:i]]; ok {
			return l
		}
	}
	return e.Line
}

func sortedFields(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// ApplyImport saves an import file in one go. Apps that already exist are
// skipped, or replaced by the file when update is set. Returns
// ErrImportInvalid with the plan when the file has problems.
func (s *Store) ApplyImport(ctx context.Context, by Actor, data []byte, update bool) (ImportResult, ImportPlan, error) {
	res := ImportResult{Added: []string{}, Updated: []string{}, Skipped: []string{}}
	plan, err := s.PlanImport(ctx, data)
	if err != nil {
		return res, plan, err
	}
	if len(plan.Problems) > 0 {
		return res, plan, ErrImportInvalid
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for kind, opts := range map[string][]ImportOption{
			KindCategory: plan.Create.Categories, KindRunsOn: plan.Create.RunsOn, KindEnvironment: plan.Create.Environments,
		} {
			for _, o := range opts {
				if err := ensureOptionLabel(ctx, tx, kind, o.Key, o.Label); err != nil {
					return err
				}
			}
		}
		if len(plan.Create.Groups) > 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO groups (name) SELECT unnest($1::text[]) ON CONFLICT DO NOTHING`,
				plan.Create.Groups); err != nil {
				return err
			}
		}
		for _, in := range plan.inputs {
			tag, err := tx.Exec(ctx, `
				INSERT INTO apps (id, name, description, owner, contact, category, runs_on, access, health_url,
				                  urls, links, visibility, icon_color, created_by, updated_by, version, start_here, icon)
				VALUES ($1, $2, $3, nullif($4, ''), $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14, $15, $16, $17)
				ON CONFLICT (id) DO NOTHING`,
				in.ID, in.Name, in.Description, in.Owner, in.Contact, in.Category, in.RunsOn, in.Access, in.HealthURL,
				nonNil(in.URLs), nonNil(in.Links), in.Visibility, in.IconColor, by.Name, in.Version, in.StartHere, in.Icon)
			if err != nil {
				return fmt.Errorf("%s: %w", in.ID, dbErr(err))
			}
			summary := "added the app " + in.Name + " from a YAML import"
			switch {
			case tag.RowsAffected() == 1:
				res.Added = append(res.Added, in.ID)
			case !update:
				res.Skipped = append(res.Skipped, in.ID)
				continue
			default:
				// the owner is only empty when it is not a group, then the current one stays
				if _, err := tx.Exec(ctx, `
					UPDATE apps SET name = $2, description = $3, owner = coalesce(nullif($4, ''), owner), contact = $5, category = $6, runs_on = $7,
					       access = $8, health_url = $9, urls = $10, links = $11, visibility = $12, icon_color = $13,
					       updated_by = $14, updated_at = now(), version = $15, start_here = $16, icon = $17
					WHERE id = $1`,
					in.ID, in.Name, in.Description, in.Owner, in.Contact, in.Category, in.RunsOn, in.Access, in.HealthURL,
					nonNil(in.URLs), nonNil(in.Links), in.Visibility, in.IconColor, by.Name, in.Version, in.StartHere, in.Icon); err != nil {
					return fmt.Errorf("%s: %w", in.ID, dbErr(err))
				}
				if _, err := tx.Exec(ctx, `DELETE FROM app_groups WHERE app_id = $1`, in.ID); err != nil {
					return err
				}
				res.Updated = append(res.Updated, in.ID)
				summary = "updated the app " + in.Name + " from a YAML import"
			}
			if err := setAppGroups(ctx, tx, in.ID, in.Groups); err != nil {
				return err
			}
			if err := logChange(ctx, tx, by, summary); err != nil {
				return err
			}
		}
		return nil
	})
	return res, plan, err
}
