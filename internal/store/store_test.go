package store

import (
	"context"
	"errors"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"apps-catalog/internal/catalog"
)

var (
	all   = Viewer{All: true}
	admin = Actor{Subject: "admin", Name: "Alex"}
)

// needs TEST_DATABASE_URL, empties every table
func openTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(s.Close)
	if _, err := s.pool.Exec(ctx, `TRUNCATE apps, app_status, app_groups, app_icons, favorites, sessions,
		oidc_logins, users, groups, group_members, changes, searches, search_dismissals, opens, access_requests CASCADE`); err != nil {
		t.Fatal(err)
	}
	// options the tests add
	if _, err := s.pool.Exec(ctx, `DELETE FROM options WHERE key LIKE 'test-%'`); err != nil {
		t.Fatal(err)
	}
	return s
}

func appIn(id string, mod ...func(*AppInput)) AppInput {
	in := AppInput{
		ID: id, Name: strings.ToUpper(id[:1]) + id[1:], Category: "monitoring", RunsOn: "docker",
		URLs:       map[string]string{"prod": "https://" + id + ".internal.example"},
		Visibility: VisibilityEveryone,
	}
	for _, m := range mod {
		m(&in)
	}
	return in
}

func onlyGroups(groups ...string) func(*AppInput) {
	return func(in *AppInput) { in.Visibility, in.Groups = VisibilityGroups, groups }
}

func mustGroup(t *testing.T, s *Store, name string, members ...string) {
	t.Helper()
	for _, m := range members {
		if err := s.SaveLoginUser(context.Background(), User{Subject: m, Username: m, Name: m}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateGroup(context.Background(), admin, GroupInput{Name: name, Members: members}); err != nil {
		t.Fatal(err)
	}
}

func mustApp(t *testing.T, s *Store, in AppInput) App {
	t.Helper()
	app, err := s.CreateApp(context.Background(), admin, in)
	if err != nil {
		t.Fatalf("create %s: %v", in.ID, err)
	}
	return app
}

func ids(apps []App) []string {
	out := []string{}
	for _, a := range apps {
		out = append(out, a.ID)
	}
	return out
}

func TestCreateUpdateDeleteApp(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustGroup(t, s, "data-team")

	app := mustApp(t, s, appIn("kafka-ui", onlyGroups("data-team")))
	if app.CreatedBy != "Alex" || app.Visibility != VisibilityGroups || !slices.Equal(app.Groups, []string{"data-team"}) {
		t.Fatalf("created: %+v", app)
	}
	if _, err := s.CreateApp(ctx, admin, appIn("kafka-ui")); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate id: %v", err)
	}
	if _, err := s.CreateApp(ctx, admin, appIn("other", onlyGroups("nope"))); !errors.Is(err, ErrUnknownRef) {
		t.Fatalf("unknown group: %v", err)
	}

	in := appIn("kafka-ui", func(in *AppInput) {
		in.Name, in.Contact, in.Version, in.StartHere = "Kafka UI", "data@example.id", "v0.7.2", true
	})
	app, err := s.UpdateApp(ctx, Actor{Subject: "f", Name: "Fajar N."}, "kafka-ui", in)
	if err != nil || app.Name != "Kafka UI" || app.UpdatedBy != "Fajar N." || len(app.Groups) != 0 ||
		app.Version != "v0.7.2" || !app.StartHere {
		t.Fatalf("update: %+v %v", app, err)
	}
	if _, err := s.UpdateApp(ctx, admin, "missing", in); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}

	if err := s.DeleteApp(ctx, admin, "kafka-ui"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteApp(ctx, admin, "kafka-ui"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}

	changes, err := s.Changes(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range changes {
		got = append(got, c.Actor+" "+c.Summary)
	}
	want := []string{
		"Alex deleted the app Kafka UI",
		"Fajar N. edited the app Kafka UI",
		"Alex added the app Kafka-ui and shared it with data-team",
		"Alex created the group data-team",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("changes:\n%s", strings.Join(got, "\n"))
	}
}

func TestVisibility(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustGroup(t, s, "data-team")
	mustGroup(t, s, "hr-team")
	mustApp(t, s, appIn("grafana"))
	mustApp(t, s, appIn("kafka", onlyGroups("data-team")))
	mustApp(t, s, appIn("hr", onlyGroups("hr-team", "data-team")))
	mustApp(t, s, appIn("secret", onlyGroups()))

	cases := []struct {
		v    Viewer
		want []string
	}{
		{Viewer{}, []string{"grafana"}},
		{Viewer{Groups: []string{"data-team"}}, []string{"grafana", "hr", "kafka"}},
		{Viewer{Groups: []string{"hr-team"}}, []string{"grafana", "hr"}},
		{all, []string{"grafana", "hr", "kafka", "secret"}},
	}
	for _, c := range cases {
		apps, err := s.List(ctx, c.v, Filter{})
		if err != nil || !slices.Equal(ids(apps), c.want) {
			t.Errorf("%+v: %v %v", c.v, ids(apps), err)
		}
	}

	_, err := s.Get(ctx, Viewer{}, "hr")
	var na *NoAccessError
	if !errors.As(err, &na) || na.Name != "Hr" || !slices.Equal(na.Groups, []string{"data-team", "hr-team"}) {
		t.Fatalf("hidden app: %v", err)
	}
	if _, err := s.Get(ctx, Viewer{}, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing app: %v", err)
	}
}

func TestListFilters(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustGroup(t, s, "data-team")
	mustGroup(t, s, "platform")
	mustApp(t, s, appIn("grafana", func(in *AppInput) { in.Owner = "platform" }))
	mustApp(t, s, appIn("metabase", onlyGroups("data-team"), func(in *AppInput) {
		in.Owner, in.Category, in.RunsOn = "platform", "data", "kubernetes"
		in.URLs = map[string]string{"prod": "https://reports.example"}
	}))

	cases := map[string]struct {
		f    Filter
		want []string
	}{
		"by url":        {Filter{Query: "reports"}, []string{"metabase"}},
		"by owner":      {Filter{Query: "PLATFORM"}, []string{"grafana", "metabase"}},
		"like escaped":  {Filter{Query: "%"}, []string{}},
		"category":      {Filter{Category: []string{"data"}}, []string{"metabase"}},
		"categories":    {Filter{Category: []string{"data", "monitoring"}}, []string{"grafana", "metabase"}},
		"runs on":       {Filter{RunsOn: []string{"docker"}}, []string{"grafana"}},
		"runs on any":   {Filter{RunsOn: []string{"docker", "kubernetes"}}, []string{"grafana", "metabase"}},
		"both":          {Filter{Category: []string{"data", "monitoring"}, RunsOn: []string{"docker"}}, []string{"grafana"}},
		"group":         {Filter{Group: "data-team"}, []string{"metabase"}},
		"everyone only": {Filter{Group: "everyone"}, []string{"grafana"}},
		"owner":         {Filter{Owner: "platform"}, []string{"grafana", "metabase"}},
		"other owner":   {Filter{Owner: "data-team"}, []string{}},
	}
	for name, c := range cases {
		apps, err := s.List(ctx, all, c.f)
		if err != nil || !slices.Equal(ids(apps), c.want) {
			t.Errorf("%s: %v %v", name, ids(apps), err)
		}
	}

	cats, err := s.Options(ctx, Viewer{}, KindCategory)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, o := range cats {
		counts[o.Key] = o.Count
	}
	// metabase is hidden from this viewer, unused options are listed with 0
	if counts["monitoring"] != 1 || counts["data"] != 0 || len(cats) < 5 {
		t.Fatalf("categories: %+v", cats)
	}
}

func TestFavoritesAndIcons(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustGroup(t, s, "hr-team")
	mustApp(t, s, appIn("grafana"))
	mustApp(t, s, appIn("hr", onlyGroups("hr-team")))
	v := Viewer{Subject: "u1"}

	if err := s.SetFavorite(ctx, v, "grafana", true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFavorite(ctx, v, "hr", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("favorite of a hidden app: %v", err)
	}
	app, _ := s.Get(ctx, v, "grafana")
	other, _ := s.Get(ctx, Viewer{Subject: "u2"}, "grafana")
	if !app.Favorite || other.Favorite {
		t.Fatalf("favorite flags: %v %v", app.Favorite, other.Favorite)
	}
	if err := s.SetFavorite(ctx, v, "grafana", false); err != nil {
		t.Fatal(err)
	}
	if app, _ := s.Get(ctx, v, "grafana"); app.Favorite {
		t.Fatal("still a favorite")
	}

	app, err := s.SetIcon(ctx, admin, "grafana", "image/svg+xml", []byte("<svg/>"))
	if err != nil || !app.HasIcon || app.IconVersion != 1 {
		t.Fatalf("set icon: %+v %v", app, err)
	}
	app, _ = s.SetIcon(ctx, admin, "grafana", "image/svg+xml", []byte("<svg></svg>"))
	if app.IconVersion != 2 {
		t.Fatalf("version not bumped: %d", app.IconVersion)
	}
	ct, data, err := s.Icon(ctx, v, "grafana")
	if err != nil || ct != "image/svg+xml" || string(data) != "<svg></svg>" {
		t.Fatalf("icon: %q %q %v", ct, data, err)
	}
	if _, err := s.SetIcon(ctx, admin, "hr", "image/png", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Icon(ctx, v, "hr"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("icon of a hidden app: %v", err)
	}
	if app, err := s.DeleteIcon(ctx, admin, "grafana"); err != nil || app.HasIcon {
		t.Fatalf("delete icon: %+v %v", app, err)
	}
}

func TestGroups(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustGroup(t, s, "data", "andi", "bunga")
	mustApp(t, s, appIn("kafka", onlyGroups("data")))
	mustApp(t, s, appIn("metabase"))

	if _, err := s.CreateGroup(ctx, admin, GroupInput{Name: "data"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := s.CreateGroup(ctx, admin, GroupInput{Name: "x", Members: []string{"ghost"}}); !errors.Is(err, ErrUnknownRef) {
		t.Fatalf("unknown member: %v", err)
	}

	g, err := s.UpdateGroup(ctx, admin, "data", GroupInput{
		Name: "data-team", Description: "Data", Members: []string{"bunga"}, Apps: []string{"kafka", "metabase"},
	})
	if err != nil || g.Name != "data-team" || len(g.Members) != 1 || g.Members[0].Subject != "bunga" || len(g.Apps) != 2 {
		t.Fatalf("update: %+v %v", g, err)
	}
	app, _ := s.Get(ctx, all, "kafka")
	if !slices.Equal(app.Groups, []string{"data-team"}) {
		t.Fatalf("rename did not follow to apps: %v", app.Groups)
	}
	u, _ := s.user(ctx, s.pool, "bunga")
	if !slices.Equal(u.Groups, []string{"data-team"}) {
		t.Fatalf("rename did not follow to members: %v", u.Groups)
	}

	if err := s.DeleteGroup(ctx, admin, "data-team"); err != nil {
		t.Fatal(err)
	}
	if groups, _ := s.Groups(ctx); len(groups) != 0 {
		t.Fatalf("groups left: %+v", groups)
	}
	if app, _ := s.Get(ctx, all, "kafka"); len(app.Groups) != 0 || app.Visibility != VisibilityGroups {
		t.Fatalf("app after group delete: %+v", app)
	}
}

func TestUsersAndSessions(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustGroup(t, s, "dev")
	mustGroup(t, s, "data")
	if err := s.SaveLoginUser(ctx, User{Subject: "u1", Username: "budi", Name: "Budi", IsAdmin: true}); err != nil {
		t.Fatal(err)
	}

	u, err := s.SetUserGroups(ctx, admin, "u1", []string{"dev", "data"})
	if err != nil || !slices.Equal(u.Groups, []string{"data", "dev"}) {
		t.Fatalf("set groups: %+v %v", u, err)
	}
	if _, err := s.SetUserGroups(ctx, admin, "u1", []string{"dev"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetUserGroups(ctx, admin, "nobody", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
	changes, _ := s.Changes(ctx, 2)
	if changes[0].Summary != "removed Budi from data" || changes[1].Summary != "added Budi to data and dev" {
		t.Fatalf("changes: %+v", changes)
	}

	// a new login updates the profile but keeps portal groups
	if err := s.SaveLoginUser(ctx, User{Subject: "u1", Username: "budi", Name: "Budi S."}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, []byte("live"), "u1", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, []byte("old"), "u1", time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Session(ctx, []byte("live"))
	if err != nil || got.Name != "Budi S." || got.IsAdmin || !slices.Equal(got.Groups, []string{"dev"}) {
		t.Fatalf("session: %+v %v", got, err)
	}
	if _, err := s.Session(ctx, []byte("old")); err != ErrNotFound {
		t.Fatalf("expired session: %v", err)
	}
	if err := s.DeleteSession(ctx, []byte("live")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(ctx, []byte("live")); err != ErrNotFound {
		t.Fatalf("deleted session: %v", err)
	}

	if err := s.SaveLogin(ctx, Login{State: "st", Nonce: "n", Verifier: "v", Redirect: "/apps/a"}); err != nil {
		t.Fatal(err)
	}
	l, err := s.TakeLogin(ctx, "st")
	if err != nil || l.Redirect != "/apps/a" || l.Nonce != "n" {
		t.Fatalf("take: %+v %v", l, err)
	}
	if _, err := s.TakeLogin(ctx, "st"); err != ErrNotFound {
		t.Fatalf("state reused: %v", err)
	}
	if err := s.DeleteExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if users, err := s.Users(ctx); err != nil || len(users) != 1 {
		t.Fatalf("users: %+v %v", users, err)
	}
}

func TestSettingsAndOptions(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustApp(t, s, appIn("grafana"))

	if err := s.AddOption(ctx, admin, KindEnvironment, "test-uat", "Test UAT"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddOption(ctx, admin, KindEnvironment, "test-uat", "Again"); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate option: %v", err)
	}
	st, err := s.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	envs := st.Environments
	if envs[0].Key != "prod" || envs[0].Count != 1 || envs[len(envs)-1].Key != "test-uat" {
		t.Fatalf("environments: %+v", envs)
	}
	if err := s.DeleteOption(ctx, admin, KindEnvironment, "prod"); !errors.Is(err, ErrInUse) {
		t.Fatalf("delete used option: %v", err)
	}
	if err := s.DeleteOption(ctx, admin, KindCategory, "monitoring"); !errors.Is(err, ErrInUse) {
		t.Fatalf("delete used category: %v", err)
	}
	if err := s.RenameOption(ctx, admin, KindEnvironment, "test-uat", "UAT"); err != nil {
		t.Fatal(err)
	}
	if err := s.RenameOption(ctx, admin, KindEnvironment, "nope", "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rename missing option: %v", err)
	}
	keys := []string{"test-uat"}
	for _, e := range envs[:len(envs)-1] {
		keys = append(keys, e.Key)
	}
	if err := s.ReorderOptions(ctx, admin, KindEnvironment, keys); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.Settings(ctx); st.Environments[0].Key != "test-uat" || st.Environments[0].Label != "UAT" || st.Environments[1].Key != "prod" {
		t.Fatalf("after reorder: %+v", st.Environments)
	}
	if err := s.ReorderOptions(ctx, admin, KindEnvironment, keys[1:]); !errors.Is(err, ErrConflict) {
		t.Fatalf("reorder with a missing key: %v", err)
	}
	if err := s.ReorderOptions(ctx, admin, KindEnvironment, append(slices.Clone(keys), keys[0])); !errors.Is(err, ErrConflict) {
		t.Fatalf("reorder with a duplicate key: %v", err)
	}
	if err := s.DeleteOption(ctx, admin, KindEnvironment, "test-uat"); err != nil {
		t.Fatal(err)
	}

	defer s.pool.Exec(ctx, `UPDATE settings SET name = 'Dev Portal', admin_email = '', icon_color_mode = 'auto', new_app_visibility = 'groups'`)
	in := SettingsInput{Name: "Portal", AdminEmail: "a@example.id", IconColorMode: "accent", NewAppVisibility: "everyone"}
	if err := s.UpdateSettings(ctx, admin, in); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.Settings(ctx); st.Name != "Portal" || st.IconColorMode != "accent" || st.NewAppVisibility != "everyone" {
		t.Fatalf("settings: %+v", st)
	}

	for label, want := range map[string]string{"Cloud Run": "cloud-run", " UAT 2 ": "uat-2", "--": ""} {
		if got := OptionKey(label); got != want {
			t.Errorf("OptionKey(%q) = %q, want %q", label, got, want)
		}
	}
}

func TestDashboard(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustGroup(t, s, "empty")
	mustApp(t, s, appIn("grafana", func(in *AppInput) { in.Contact = "x@example.id" }))
	mustApp(t, s, appIn("hr", onlyGroups()))
	if err := s.SaveLoginUser(ctx, User{Subject: "gita", Name: "Gita M."}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveLoginUser(ctx, User{Subject: "boss", Name: "Boss", IsAdmin: true}); err != nil {
		t.Fatal(err)
	}

	d, err := s.Dashboard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d.Apps != 2 || d.AppsEveryone != 1 || d.Groups != 1 || d.Users != 2 || d.Admins != 1 || d.DefaultIcons != 2 {
		t.Fatalf("counts: %+v", d)
	}
	var got []string
	for _, a := range d.Attention {
		got = append(got, a.Kind+":"+a.Target)
	}
	want := []string{"user_no_group:gita", "app_no_groups:hr", "app_no_owner:grafana", "app_no_owner:hr",
		"app_no_contact:hr", "group_no_members:empty"}
	if !slices.Equal(got, want) {
		t.Fatalf("attention: %v", got)
	}
	if d.Attention[0].Detail != "They only see the 1 app shared with everyone." || len(d.Changes) != 3 {
		t.Fatalf("dashboard: %+v", d)
	}
}

func TestImport(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustApp(t, s, appIn("grafana", func(in *AppInput) { in.Name = "Edited in portal" }))

	res, err := s.Import(ctx, Actor{Name: "Import"}, []catalog.App{
		{ID: "grafana", Name: "Grafana"},
		{ID: "billing", Name: "Billing", Owner: " Billing-Team ", Category: "test-api",
			AllowedGroups: []string{"billing-team", "Bad Name"}, URLs: map[string]string{"test-uat": "https://b.example"}},
		{ID: "ghosted", Name: "Ghosted", Owner: "ghost"},
	})
	if err != nil || !slices.Equal(res.Added, []string{"billing", "ghosted"}) || !slices.Equal(res.Skipped, []string{"grafana"}) {
		t.Fatalf("import: %+v %v", res, err)
	}
	if app, _ := s.Get(ctx, all, "grafana"); app.Name != "Edited in portal" {
		t.Fatal("import overwrote an existing app")
	}
	app, _ := s.Get(ctx, all, "billing")
	if app.Visibility != VisibilityGroups || !slices.Equal(app.Groups, []string{"billing-team"}) || app.Owner != "billing-team" {
		t.Fatalf("imported app: %+v", app)
	}
	if app, err := s.Get(ctx, all, "ghosted"); err != nil || app.Owner != "" {
		t.Fatalf("unknown owner: %+v %v", app, err)
	}
	st, _ := s.Settings(ctx)
	if !slices.ContainsFunc(st.Categories, func(o Option) bool { return o.Key == "test-api" }) ||
		!slices.ContainsFunc(st.Environments, func(o Option) bool { return o.Key == "test-uat" }) {
		t.Fatal("import did not add the new category and environment")
	}
}

func TestSaveStatus_SinceOnlyMovesOnChange(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustApp(t, s, appIn("a"))
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	save := func(up bool, at time.Time) Status {
		t.Helper()
		if err := s.SaveStatus(ctx, "a", Status{Up: up, HTTPStatus: 200, LatencyMS: 5, Target: "https://a", CheckedAt: at}); err != nil {
			t.Fatal(err)
		}
		app, err := s.Get(ctx, all, "a")
		if err != nil || app.Status == nil {
			t.Fatalf("get: %+v %v", app, err)
		}
		return *app.Status
	}

	if st := save(true, t0); !st.Since.Equal(t0) {
		t.Fatalf("first check since = %v", st.Since)
	}
	if st := save(true, t0.Add(time.Minute)); !st.Since.Equal(t0) {
		t.Fatalf("still up, since moved to %v", st.Since)
	}
	if st := save(false, t0.Add(2*time.Minute)); !st.Since.Equal(t0.Add(2*time.Minute)) || st.Up {
		t.Fatalf("went down: %+v", st)
	}

	if err := s.SaveStatus(ctx, "gone", Status{Up: true, CheckedAt: t0}); err != nil {
		t.Fatalf("status for a deleted app should be ignored, got %v", err)
	}
}

func TestLocalUsers(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustGroup(t, s, "dev")

	u, err := s.CreateLocalUser(ctx, admin, LocalUserInput{Username: "Budi", Name: "Budi S.", Groups: []string{"dev"}}, "hash1")
	if err != nil || u.Subject != "local:budi" || u.Source != SourceLocal || u.LastLogin != nil ||
		!slices.Equal(u.Groups, []string{"dev"}) {
		t.Fatalf("create: %+v %v", u, err)
	}
	if _, err := s.CreateLocalUser(ctx, admin, LocalUserInput{Username: "budi"}, "x"); !errors.Is(err, ErrConflict) {
		t.Fatalf("same username: %v", err)
	}

	sub, hash, err := s.LocalCredentials(ctx, "BUDI")
	if err != nil || sub != "local:budi" || hash != "hash1" {
		t.Fatalf("credentials: %q %q %v", sub, hash, err)
	}
	if err := s.SetPassword(ctx, nil, sub, "hash2"); err != nil {
		t.Fatal(err)
	}
	if h, _ := s.PasswordHash(ctx, sub); h != "hash2" {
		t.Fatalf("hash after change: %q", h)
	}

	u, err = s.UpdateLocalUser(ctx, admin, sub, "Budi", "budi@example.id", true)
	if err != nil || !u.IsAdmin || u.Email != "budi@example.id" {
		t.Fatalf("update: %+v %v", u, err)
	}
	if n, _ := s.LocalAdmins(ctx); n != 1 {
		t.Fatalf("local admins: %d", n)
	}

	// sso users can't be edited or get a password, and an sso login never
	// overwrites a local user
	if err := s.SaveLoginUser(ctx, User{Subject: "sso-1", Username: "budi"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateLocalUser(ctx, admin, "sso-1", "x", "", true); !errors.Is(err, ErrNotLocal) {
		t.Fatalf("edit sso user: %v", err)
	}
	if err := s.SetPassword(ctx, &admin, "sso-1", "h"); !errors.Is(err, ErrNotLocal) {
		t.Fatalf("password for sso user: %v", err)
	}
	if _, _, err := s.LocalCredentials(ctx, "sso-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("sso user has no local credentials: %v", err)
	}

	if err := s.TouchLogin(ctx, sub); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, []byte("tok"), sub, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Session(ctx, []byte("tok")); err != nil || got.LastLogin == nil || !got.IsAdmin {
		t.Fatalf("session: %+v %v", got, err)
	}
	if err := s.SetFavorite(ctx, Viewer{Subject: sub}, "missing", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("favorite: %v", err)
	}
	if err := s.DeleteUser(ctx, admin, sub); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(ctx, []byte("tok")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("session after delete: %v", err)
	}
	changes, _ := s.Changes(ctx, 3)
	if changes[0].Summary != "removed the user Budi" || changes[1].Summary != "made Budi an admin" {
		t.Fatalf("changes: %+v", changes)
	}
}

func TestImportFile(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustApp(t, s, appIn("grafana"))
	mustGroup(t, s, "platform")

	file := []byte(`version: 1
apps:
  - id: grafana
    name: Grafana from file
    owner: obs
    category: Monitoring
    visible_to: [platform]
    urls:
      Production: grafana.example.com
  - id: test-new
    name: New one
    owner: data
    category: Test Cloud Stuff
    runs_on: test-metal
    version: "1.10"
    start_here: true
    icon: Brand-Docker
    visible_to: [test-team, platform]
    icon_color: teal
    urls:
      prod: https://new.example.com
      test-qa: https://qa.new.example.com
    links:
      repo: https://git.example.com/new
`)
	plan, err := s.PlanImport(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Problems) > 0 {
		t.Fatalf("problems: %+v", plan.Problems)
	}
	if len(plan.Apps) != 2 || !plan.Apps[0].Exists || plan.Apps[1].Exists {
		t.Fatalf("apps: %+v", plan.Apps)
	}
	c := plan.Create
	if !slices.Equal(c.Categories, []ImportOption{{"test-cloud-stuff", "Test Cloud Stuff"}}) ||
		!slices.Equal(c.RunsOn, []ImportOption{{"test-metal", "Test metal"}}) ||
		!slices.Equal(c.Environments, []ImportOption{{"test-qa", "Test qa"}}) ||
		!slices.Equal(c.Groups, []string{"test-team"}) {
		t.Fatalf("create: %+v", c)
	}

	// skip keeps the existing app as it is
	res, _, err := s.ApplyImport(ctx, admin, file, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Added, []string{"test-new"}) || !slices.Equal(res.Skipped, []string{"grafana"}) || len(res.Updated) != 0 {
		t.Fatalf("result: %+v", res)
	}
	g, _ := s.Get(ctx, all, "grafana")
	if g.Name != "Grafana" {
		t.Fatalf("grafana changed on skip: %q", g.Name)
	}
	n, err := s.Get(ctx, all, "test-new")
	if err != nil {
		t.Fatal(err)
	}
	if n.Category != "test-cloud-stuff" || n.RunsOn != "test-metal" || n.IconColor != IconPalette[1] ||
		n.Visibility != VisibilityGroups || !slices.Equal(n.Groups, []string{"platform", "test-team"}) ||
		n.URLs["test-qa"] == "" || n.Links["repo"] == "" || n.Version != "1.10" || !n.StartHere || n.Icon != "brand-docker" {
		t.Fatalf("new app: %+v", n)
	}

	// update replaces every field
	res, _, err = s.ApplyImport(ctx, admin, file, true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Updated, []string{"grafana", "test-new"}) || len(res.Added) != 0 {
		t.Fatalf("result: %+v", res)
	}
	g, _ = s.Get(ctx, all, "grafana")
	// obs is not a group, so the owner stays empty
	if g.Name != "Grafana from file" || g.Owner != "" || g.RunsOn != "" || g.URLs["prod"] != "https://grafana.example.com" ||
		!slices.Equal(g.Groups, []string{"platform"}) {
		t.Fatalf("grafana after update: %+v", g)
	}
	st, _ := s.Settings(ctx)
	if !hasOption(st.Categories, "test-cloud-stuff") || !hasOption(st.Environments, "test-qa") {
		t.Fatal("options were not created")
	}
}

func TestImportFile_ProblemsSaveNothing(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	file := []byte(`version: 1
apps:
  - id: test-ok
    name: Fine
    owner: data
    category: test-should-not-exist
    urls:
      prod: https://ok.example.com
  - id: Bad_ID
    name: Bad
    urls:
      staging: https://bad.example.com
    links:
      wiki: https://wiki.example.com
    icon_color: pink
    visible_to: [everyone, platform]
    icon: not-a-real-icon
`)
	res, plan, err := s.ApplyImport(ctx, admin, file, false)
	if !errors.Is(err, ErrImportInvalid) {
		t.Fatalf("err = %v", err)
	}
	if len(res.Added) != 0 {
		t.Fatalf("added: %v", res.Added)
	}
	want := map[string]int{"id": 9, "owner": 9, "urls.prod": 11, "links.wiki": 14, "icon_color": 15, "visible_to": 16, "icon": 17}
	got := map[string]int{}
	for _, p := range plan.Problems {
		got[p.Field] = p.Line
		if p.App != "Bad_ID" {
			t.Fatalf("problem on the wrong app: %+v", p)
		}
	}
	for f, line := range want {
		if got[f] != line {
			t.Fatalf("%s on line %d, want %d (all: %+v)", f, got[f], line, plan.Problems)
		}
	}
	if plan.Apps[1].Problems == 0 || plan.Apps[0].Problems != 0 {
		t.Fatalf("apps: %+v", plan.Apps)
	}
	apps, _ := s.List(ctx, all, Filter{})
	st, _ := s.Settings(ctx)
	if len(apps) != 0 || hasOption(st.Categories, "test-should-not-exist") {
		t.Fatal("something was saved")
	}
}

func TestUsageLogging(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.LogSearch(ctx, "u1", "  Sonar  ", false); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM searches WHERE q = 'sonar' AND subject = 'u1' AND NOT found`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("sonar rows = %d, err %v", n, err)
	}
	if err := s.LogSearch(ctx, "u1", strings.Repeat("a", 150), true); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM searches WHERE length(q) = 100`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("cut rows = %d, err %v", n, err)
	}
	if err := s.LogSearch(ctx, "u1", "  ", true); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM searches`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("search rows = %d, err %v", n, err)
	}

	mustApp(t, s, appIn("grafana"))
	mustGroup(t, s, "hr-team")
	mustApp(t, s, appIn("hr", onlyGroups("hr-team")))
	mustApp(t, s, appIn("kibana"))
	u1 := Viewer{Subject: "u1"}
	for _, id := range []string{"grafana", "grafana"} {
		if err := s.LogOpen(ctx, u1, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"hr", "nope"} {
		if err := s.LogOpen(ctx, u1, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("open %s = %v, want ErrNotFound", id, err)
		}
	}
	for _, id := range []string{"kibana", "grafana"} {
		if err := s.LogOpen(ctx, u1, id); err != nil {
			t.Fatal(err)
		}
	}
	recent, err := s.RecentOpens(ctx, u1, 5)
	if err != nil || !slices.Equal(recent, []string{"grafana", "kibana"}) {
		t.Fatalf("recent = %v, err %v", recent, err)
	}
	none, err := s.RecentOpens(ctx, Viewer{Subject: "u2"}, 5)
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("none = %#v, err %v", none, err)
	}

	apps, err := s.List(ctx, all, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range apps {
		want := map[string]int{"grafana": 3, "kibana": 1}[a.ID]
		if a.Opens30d != want {
			t.Errorf("%s opens_30d = %d, want %d", a.ID, a.Opens30d, want)
		}
	}

	if _, err := s.pool.Exec(ctx, `INSERT INTO searches (subject, q, found, at) VALUES ('u1', 'old', true, now() - interval '200 days')`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO opens (subject, app_id, at) VALUES ('u1', 'grafana', now() - interval '200 days')`); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteOldUsage(ctx); err != nil {
		t.Fatal(err)
	}
	var old, fresh int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM searches WHERE q = 'old') + (SELECT count(*) FROM opens WHERE at < now() - interval '100 days'),
		(SELECT count(*) FROM searches) + (SELECT count(*) FROM opens)`).Scan(&old, &fresh); err != nil {
		t.Fatal(err)
	}
	if old != 0 || fresh != 6 {
		t.Fatalf("old = %d, fresh = %d, want 0 and 6", old, fresh)
	}
}

func TestDashboardUsage(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	for _, u := range []string{"ann", "bob"} {
		if err := s.SaveLoginUser(ctx, User{Subject: u, Name: u}); err != nil {
			t.Fatal(err)
		}
	}
	mustGroup(t, s, "dev", "ann", "bob")
	mustGroup(t, s, "qa")
	mustApp(t, s, appIn("grafana"))
	mustApp(t, s, appIn("sonarqube", onlyGroups("qa")))
	for _, x := range []struct {
		who, q string
		found  bool
	}{{"ann", "sonar", false}, {"bob", "sonar", false}, {"ann", "jenkins", false}, {"ann", "graf", true}} {
		if err := s.LogSearch(ctx, x.who, x.q, x.found); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.LogOpen(ctx, Viewer{Subject: "ann", All: true}, "grafana"); err != nil {
		t.Fatal(err)
	}

	d, err := s.Dashboard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d.Usage != (Usage{People: 2, Opens: 1, Searches: 4, Missed: 3}) {
		t.Fatalf("usage: %+v", d.Usage)
	}
	if len(d.Missed) != 2 {
		t.Fatalf("missed: %+v", d.Missed)
	}
	m := d.Missed[0]
	if m.Query != "sonar" || m.People != 2 || m.Hidden == nil || m.Hidden.ID != "sonarqube" {
		t.Fatalf("missed[0]: %+v", m)
	}
	if d.Missed[1].Query != "jenkins" || d.Missed[1].Hidden != nil {
		t.Fatalf("missed[1]: %+v", d.Missed[1])
	}

	if err := s.DismissSearch(ctx, admin, "sonar"); err != nil {
		t.Fatal(err)
	}
	d, _ = s.Dashboard(ctx)
	for _, m := range d.Missed {
		if m.Query == "sonar" {
			t.Fatalf("sonar still listed: %+v", d.Missed)
		}
	}
	if err := s.LogSearch(ctx, "bob", "sonar", false); err != nil {
		t.Fatal(err)
	}
	d, _ = s.Dashboard(ctx)
	if len(d.Missed) == 0 || d.Missed[0].Query != "sonar" || d.Missed[0].People != 2 {
		t.Fatalf("after re-search: %+v", d.Missed)
	}
}

func TestOwnerGroup(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustGroup(t, s, "platform", "ann")
	mustGroup(t, s, "qa")
	mustApp(t, s, appIn("grafana", onlyGroups("qa"), func(in *AppInput) { in.Owner = "platform" }))

	ann := Viewer{Subject: "ann", Groups: []string{"platform"}}
	app, err := s.Get(ctx, ann, "grafana")
	if err != nil || !app.CanEdit || app.Owner != "platform" {
		t.Fatalf("owner get: %+v %v", app, err)
	}
	if apps, _ := s.List(ctx, ann, Filter{}); len(apps) != 1 || !apps[0].CanEdit {
		t.Fatalf("owner list: %+v", apps)
	}
	qa := Viewer{Subject: "q", Groups: []string{"qa"}}
	if app, err := s.Get(ctx, qa, "grafana"); err != nil || app.CanEdit {
		t.Fatalf("qa get: %+v %v", app, err)
	}
	if apps, _ := s.List(ctx, qa, Filter{}); len(apps) != 1 || apps[0].CanEdit {
		t.Fatalf("qa list: %+v", apps)
	}
	if apps, _ := s.List(ctx, all, Filter{}); len(apps) != 1 || !apps[0].CanEdit {
		t.Fatalf("admin list: %+v", apps)
	}

	// an owner who searched for the app can see it, so it is not hidden
	if err := s.LogSearch(ctx, "ann", "grafa", false); err != nil {
		t.Fatal(err)
	}
	if d, err := s.Dashboard(ctx); err != nil || len(d.Missed) != 1 || d.Missed[0].Hidden != nil {
		t.Fatalf("missed: %+v %v", d.Missed, err)
	}

	app, err = s.UpdateDetails(ctx, Actor{Subject: "ann", Name: "Ann"}, "grafana", DetailsInput{
		Description: "Dashboards", URLs: map[string]string{"prod": "https://g2.example"},
	})
	if err != nil || app.Description != "Dashboards" || app.URLs["prod"] != "https://g2.example" || app.UpdatedBy != "Ann" {
		t.Fatalf("details: %+v %v", app, err)
	}
	app, _ = s.Get(ctx, all, "grafana")
	if app.Name != "Grafana" || app.Visibility != VisibilityGroups || !slices.Equal(app.Groups, []string{"qa"}) ||
		app.Owner != "platform" || app.Category != "monitoring" || app.Description != "Dashboards" {
		t.Fatalf("after details: %+v", app)
	}
	if changes, _ := s.Changes(ctx, 1); changes[0].Actor != "Ann" || changes[0].Summary != "edited the app Grafana" {
		t.Fatalf("change: %+v", changes)
	}
	if _, err := s.UpdateDetails(ctx, admin, "missing", DetailsInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("details of a missing app: %v", err)
	}

	if _, err := s.UpdateGroup(ctx, admin, "platform", GroupInput{Name: "platform-team", Members: []string{"ann"}}); err != nil {
		t.Fatal(err)
	}
	if app, _ := s.Get(ctx, all, "grafana"); app.Owner != "platform-team" {
		t.Fatalf("owner after rename: %q", app.Owner)
	}
	if err := s.DeleteGroup(ctx, admin, "platform-team"); err != nil {
		t.Fatal(err)
	}
	if app, _ := s.Get(ctx, all, "grafana"); app.Owner != "" {
		t.Fatalf("owner after delete: %q", app.Owner)
	}
	d, err := s.Dashboard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(d.Attention, func(a Attention) bool { return a.Kind == "app_no_owner" && a.Target == "grafana" })
	if i < 0 || d.Attention[i].Title != "Grafana has no owner team" || d.Attention[i].Detail != "Nobody outside the admins can edit it." {
		t.Fatalf("attention: %+v", d.Attention)
	}

	if _, err := s.CreateApp(ctx, admin, appIn("kibana", func(in *AppInput) { in.Owner = "ghost" })); !errors.Is(err, ErrUnknownRef) {
		t.Fatalf("create with unknown owner: %v", err)
	}
	if _, err := s.UpdateApp(ctx, admin, "grafana", appIn("grafana", func(in *AppInput) { in.Owner = "ghost" })); !errors.Is(err, ErrUnknownRef) {
		t.Fatalf("update with unknown owner: %v", err)
	}
}

func TestTeam(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustGroup(t, s, "platform", "bob", "ann")
	mustGroup(t, s, "qa")
	owned := func(in *AppInput) { in.Owner = "platform" }
	mustApp(t, s, appIn("grafana", owned))
	mustApp(t, s, appIn("kibana", onlyGroups("qa"), owned))
	mustApp(t, s, appIn("jira"))

	team, err := s.Team(ctx, Viewer{Subject: "x"}, "platform")
	if err != nil || team.Name != "platform" || len(team.Members) != 2 || team.Members[0] != "ann" ||
		!slices.Equal(ids(team.Apps), []string{"grafana"}) {
		t.Fatalf("team: %+v %v", team, err)
	}
	if team, _ := s.Team(ctx, all, "platform"); !slices.Equal(ids(team.Apps), []string{"grafana", "kibana"}) {
		t.Fatalf("team for admin: %v", ids(team.Apps))
	}
	if team, _ := s.Team(ctx, all, "qa"); team.Apps == nil || len(team.Apps) != 0 || team.Members == nil {
		t.Fatalf("empty team: %+v", team)
	}
	if _, err := s.Team(ctx, all, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing team: %v", err)
	}
}

func TestImportFile_Owner(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustGroup(t, s, "platform")
	mustApp(t, s, appIn("c", func(in *AppInput) { in.Owner = "platform" }))

	file := []byte(`version: 1
apps:
  - id: a
    name: A
    owner: Data
    urls:
      prod: https://a.example.com
  - id: b
    name: B
    owner: ghost
    urls:
      prod: https://b.example.com
  - id: c
    name: C from file
    owner: ghost
    urls:
      prod: https://c.example.com
  - id: d
    name: D
    owner: platform
    visible_to: [data]
    urls:
      prod: https://d.example.com
`)
	plan, err := s.PlanImport(ctx, file)
	if err != nil || len(plan.Problems) > 0 {
		t.Fatalf("plan: %+v %v", plan.Problems, err)
	}
	notes := []string{}
	for _, a := range plan.Apps {
		notes = append(notes, a.OwnerNote)
	}
	want := []string{"", `Team "ghost" does not exist, the app will have no owner`,
		`Team "ghost" does not exist, the owner stays platform`, ""}
	if !slices.Equal(notes, want) || !slices.Equal(plan.Create.Groups, []string{"data"}) {
		t.Fatalf("notes: %q create: %+v", notes, plan.Create)
	}

	if _, _, err := s.ApplyImport(ctx, admin, file, true); err != nil {
		t.Fatal(err)
	}
	for id, owner := range map[string]string{"a": "data", "b": "", "c": "platform", "d": "platform"} {
		app, err := s.Get(ctx, all, id)
		if err != nil || app.Owner != owner {
			t.Errorf("%s: owner %q, want %q (%v)", id, app.Owner, owner, err)
		}
	}
	if app, _ := s.Get(ctx, all, "c"); app.Name != "C from file" {
		t.Fatalf("c was not updated: %+v", app)
	}

	// an existing app without an owner says so
	if _, err := s.pool.Exec(ctx, `UPDATE apps SET owner = NULL WHERE id = 'c'`); err != nil {
		t.Fatal(err)
	}
	plan, _ = s.PlanImport(ctx, file)
	if plan.Apps[2].OwnerNote != `Team "ghost" does not exist, the owner stays empty` {
		t.Fatalf("note: %q", plan.Apps[2].OwnerNote)
	}
}

func TestOwnerMigration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	reset := func() error {
		_, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`)
		return err
	}
	// leave a fully migrated schema for the other tests
	t.Cleanup(func() {
		if err := reset(); err != nil {
			t.Error(err)
		}
		if err := migrate(ctx, pool); err != nil {
			t.Error(err)
		}
	})
	if err := reset(); err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { db.Close() })
	p, err := goose.NewProvider(goose.DialectPostgres, db, mustSub(migrations, "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO groups (name) VALUES ('platform');
		INSERT INTO apps (id, name, owner) VALUES ('a', 'A', 'Platform'), ('b', 'B', '  platform '), ('c', 'C', 'nobody'), ('d', 'D', ' ')`); err != nil {
		t.Fatal(err)
	}
	owners := func() map[string]string {
		t.Helper()
		rows, err := pool.Query(ctx, `SELECT id, coalesce(owner, '<null>') FROM apps`)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for rows.Next() {
			var id, owner string
			if err := rows.Scan(&id, &owner); err != nil {
				t.Fatal(err)
			}
			out[id] = owner
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}

	if _, err := p.UpTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	if got, want := owners(), map[string]string{"a": "platform", "b": "platform", "c": "<null>", "d": "<null>"}; !maps.Equal(got, want) {
		t.Fatalf("after up: %v", got)
	}
	// only the unmatched owner leaves a change row
	rows, err := pool.Query(ctx, `SELECT actor_subject, actor, summary FROM changes ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	var changes []string
	for rows.Next() {
		var subject, actor, summary string
		if err := rows.Scan(&subject, &actor, &summary); err != nil {
			t.Fatal(err)
		}
		changes = append(changes, subject+"|"+actor+"|"+summary)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{`|migration|cleared the owner "nobody" of C, no team with that name`}; !slices.Equal(changes, want) {
		t.Fatalf("changes after up: %q", changes)
	}
	if _, err := pool.Exec(ctx, `UPDATE apps SET owner = 'nobody' WHERE id = 'c'`); !errors.Is(dbErr(err), ErrUnknownRef) {
		t.Fatalf("owner is not a group reference: %v", err)
	}

	if _, err := p.DownTo(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if got, want := owners(), map[string]string{"a": "platform", "b": "platform", "c": "", "d": ""}; !maps.Equal(got, want) {
		t.Fatalf("after down: %v", got)
	}
	var nullable, def string
	if err := pool.QueryRow(ctx, `SELECT is_nullable, column_default FROM information_schema.columns
		WHERE table_name = 'apps' AND column_name = 'owner'`).Scan(&nullable, &def); err != nil {
		t.Fatal(err)
	}
	if nullable != "NO" || def != "''::text" {
		t.Fatalf("owner column after down: nullable %s, default %s", nullable, def)
	}
	if _, err := pool.Exec(ctx, `UPDATE apps SET owner = 'nobody' WHERE id = 'c'`); err != nil {
		t.Fatalf("free text owner after down: %v", err)
	}
}

func TestAccessRequests(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	mustGroup(t, s, "hr-team", "amy")
	mustGroup(t, s, "qa", "amy")
	if err := s.SaveLoginUser(ctx, User{Subject: "bob", Username: "bob", Name: "Bob"}); err != nil {
		t.Fatal(err)
	}
	mustApp(t, s, appIn("hr", onlyGroups("hr-team")))
	mustApp(t, s, appIn("grafana"))
	bob := Viewer{Subject: "bob"}

	if _, err := s.RequestAccess(ctx, bob, "grafana", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("visible app: %v", err)
	}
	if _, err := s.RequestAccess(ctx, bob, "missing", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing app: %v", err)
	}
	r1, err := s.RequestAccess(ctx, bob, "hr", "  need payroll  ")
	if err != nil {
		t.Fatal(err)
	}
	if r1.Status != "pending" || r1.Note != "need payroll" || r1.Person != "Bob" || r1.AppName != "Hr" {
		t.Fatalf("request: %+v", r1)
	}
	r2, err := s.RequestAccess(ctx, bob, "hr", "again")
	if err != nil || r2.ID != r1.ID {
		t.Fatalf("second request: %+v %v", r2, err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM access_requests`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("rows: %d %v", n, err)
	}

	got, err := s.LatestAccessRequest(ctx, "bob", "hr")
	if err != nil || got == nil || got.Status != "pending" {
		t.Fatalf("latest: %+v %v", got, err)
	}
	if got, err := s.LatestAccessRequest(ctx, "bob", "grafana"); err != nil || got != nil {
		t.Fatalf("latest grafana: %+v %v", got, err)
	}

	if _, err := s.ApproveAccess(ctx, admin, r1.ID, "qa"); !errors.Is(err, ErrNotShared) {
		t.Fatalf("not shared: %v", err)
	}
	if _, err := s.ApproveAccess(ctx, admin, 99999, "hr-team"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing request: %v", err)
	}
	if got, _ := s.LatestAccessRequest(ctx, "bob", "hr"); got.Status != "pending" {
		t.Fatalf("still pending: %+v", got)
	}
	done, err := s.ApproveAccess(ctx, admin, r1.ID, "hr-team")
	if err != nil || done.Status != "approved" || done.Group != "hr-team" || done.DecidedAt == nil || done.DecidedBy != "Alex" {
		t.Fatalf("approve: %+v %v", done, err)
	}
	g, err := s.Group(ctx, "hr-team")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range g.Members {
		found = found || m.Subject == "bob"
	}
	if !found {
		t.Fatalf("bob not a member: %+v", g.Members)
	}
	if _, err := s.ApproveAccess(ctx, admin, r1.ID, "hr-team"); !errors.Is(err, ErrDecided) {
		t.Fatalf("approve again: %v", err)
	}
	if _, err := s.DeclineAccess(ctx, admin, r1.ID); !errors.Is(err, ErrDecided) {
		t.Fatalf("decline decided: %v", err)
	}

	// access removed later, bob asks again
	if _, err := s.pool.Exec(ctx, `DELETE FROM group_members WHERE subject = 'bob'`); err != nil {
		t.Fatal(err)
	}
	r3, err := s.RequestAccess(ctx, bob, "hr", "")
	if err != nil || r3.ID == r1.ID || r3.Status != "pending" {
		t.Fatalf("new request: %+v %v", r3, err)
	}
	list, err := s.AccessRequests(ctx)
	if err != nil || len(list) != 1 || list[0].ID != r3.ID {
		t.Fatalf("list: %+v %v", list, err)
	}
	// added to the group by hand: the pending request is not listed
	if _, err := s.pool.Exec(ctx, `INSERT INTO group_members (group_name, subject) VALUES ('hr-team', 'bob')`); err != nil {
		t.Fatal(err)
	}
	if list, err := s.AccessRequests(ctx); err != nil || len(list) != 0 {
		t.Fatalf("list with access: %+v %v", list, err)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM group_members WHERE subject = 'bob'`); err != nil {
		t.Fatal(err)
	}
	dec, err := s.DeclineAccess(ctx, admin, r3.ID)
	if err != nil || dec.Status != "declined" {
		t.Fatalf("decline: %+v %v", dec, err)
	}
	if list, _ := s.AccessRequests(ctx); list == nil || len(list) != 0 {
		t.Fatalf("empty list: %#v", list)
	}
	r4, err := s.RequestAccess(ctx, bob, "hr", "")
	if err != nil || r4.ID == r3.ID {
		t.Fatalf("after decline: %+v %v", r4, err)
	}

	if err := s.DeleteUser(ctx, admin, "bob"); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM access_requests`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows after delete: %d %v", n, err)
	}
}
