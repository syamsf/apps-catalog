package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"

	"apps-catalog/internal/auth"
	"apps-catalog/internal/catalog"
	"apps-catalog/internal/store"
)

// fakeStore panics on methods a test does not set up.
type fakeStore struct {
	Store
	apps      []store.App
	gotList   store.Filter
	gotViewer store.Viewer
	created   *store.AppInput
	icon      []byte
	renamed   string
	reordered []string
	recent    []string
	logged    []string
	opened    string
	dismissed string
	details   *store.DetailsInput
}

func (f *fakeStore) Ping(context.Context) error { return nil }

func (f *fakeStore) List(_ context.Context, v store.Viewer, flt store.Filter) ([]store.App, error) {
	f.gotList, f.gotViewer = flt, v
	return f.apps, nil
}

func (f *fakeStore) Get(_ context.Context, _ store.Viewer, id string) (store.App, error) {
	if id == "hr" {
		return store.App{}, &store.NoAccessError{ID: "hr", Name: "HR portal", Groups: []string{"hr-team"}}
	}
	if id == "kibana" {
		return store.App{ID: "kibana", Name: "Kibana"}, nil
	}
	for _, a := range f.apps {
		if a.ID == id {
			return a, nil
		}
	}
	return store.App{}, store.ErrNotFound
}

func (f *fakeStore) Group(_ context.Context, name string) (store.Group, error) {
	if name == "ghost" {
		return store.Group{}, store.ErrNotFound
	}
	return store.Group{Name: name}, nil
}

func (f *fakeStore) UpdateDetails(_ context.Context, _ store.Actor, id string, in store.DetailsInput) (store.App, error) {
	f.details = &in
	return store.App{ID: id, Description: in.Description}, nil
}

func (f *fakeStore) DeleteIcon(_ context.Context, _ store.Actor, id string) (store.App, error) {
	return store.App{ID: id}, nil
}

func (f *fakeStore) Team(_ context.Context, _ store.Viewer, name string) (store.Team, error) {
	if name != "platform" {
		return store.Team{}, store.ErrNotFound
	}
	return store.Team{Name: name, Members: []string{"Ann"}, Apps: []store.App{{ID: "grafana"}}}, nil
}

func (f *fakeStore) Settings(context.Context) (store.Settings, error) {
	return store.Settings{
		Name: "Dev Portal", AdminEmail: "admins@example.id",
		Categories:   []store.Option{{Key: "monitoring"}},
		RunsOn:       []store.Option{{Key: "docker"}},
		Environments: []store.Option{{Key: "prod"}, {Key: "staging"}},
	}, nil
}

func (f *fakeStore) CreateApp(_ context.Context, _ store.Actor, in store.AppInput) (store.App, error) {
	if in.ID == "grafana" {
		return store.App{}, store.ErrConflict
	}
	f.created = &in
	return store.App{ID: in.ID, Name: in.Name}, nil
}

func (f *fakeStore) SetIcon(_ context.Context, _ store.Actor, id, ct string, data []byte) (store.App, error) {
	f.icon = data
	return store.App{ID: id, HasIcon: true}, nil
}

var testSPA = fstest.MapFS{
	"index.html":      {Data: []byte("<html>spa</html>")},
	"assets/app-1.js": {Data: []byte("console.log(1)")},
}

func newTestApp(t *testing.T) (*fakeStore, *fiber.App) {
	t.Helper()
	fs := &fakeStore{apps: []store.App{{ID: "grafana", Name: "Grafana", CanEdit: true}}}
	return fs, New(fs, auth.Disabled{}, testSPA, zerolog.Nop(), "portal-admins")
}

type result struct {
	code   int
	body   string
	header http.Header
}

func do(t *testing.T, app *fiber.App, req *http.Request) result {
	t.Helper()
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return result{code: resp.StatusCode, body: string(b), header: resp.Header}
}

func get(t *testing.T, app *fiber.App, path string) result {
	t.Helper()
	return do(t, app, httptest.NewRequest(http.MethodGet, path, nil))
}

func sendJSON(t *testing.T, app *fiber.App, method, path string, body any) result {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return do(t, app, req)
}

func TestListApps_PassesFilters(t *testing.T) {
	fs, app := newTestApp(t)
	res := get(t, app, "/api/apps?q=graf&category=monitoring,data&runs_on=docker&group=data-team")
	if res.code != http.StatusOK {
		t.Fatalf("status %d", res.code)
	}
	want := store.Filter{Query: "graf", Category: []string{"monitoring", "data"}, RunsOn: []string{"docker"}, Group: "data-team"}
	if !reflect.DeepEqual(fs.gotList, want) {
		t.Fatalf("filter = %+v, want %+v", fs.gotList, want)
	}
	if !fs.gotViewer.All || fs.gotViewer.Subject != "local" {
		t.Fatalf("auth disabled should be the local admin, got %+v", fs.gotViewer)
	}
	var body AppList
	if err := json.Unmarshal([]byte(res.body), &body); err != nil || body.Total != 1 || body.Apps[0].ID != "grafana" {
		t.Fatalf("body: %+v, %v", body, err)
	}
}

func TestGetApp(t *testing.T) {
	_, app := newTestApp(t)
	if res := get(t, app, "/api/apps/grafana"); res.code != http.StatusOK {
		t.Fatalf("existing app: %d", res.code)
	}
	res := get(t, app, "/api/apps/nope")
	if res.code != http.StatusNotFound || !strings.Contains(res.body, "app not found") {
		t.Fatalf("missing app: %d %s", res.code, res.body)
	}
	res = get(t, app, "/api/apps/hr")
	var na NoAccess
	json.Unmarshal([]byte(res.body), &na)
	if res.code != http.StatusForbidden || na.App.Name != "HR portal" || na.Groups[0] != "hr-team" ||
		na.AdminEmail != "admins@example.id" || na.Request == nil || na.Request.Status != "pending" {
		t.Fatalf("hidden app: %d %s", res.code, res.body)
	}
}

func TestCreateApp_Validates(t *testing.T) {
	fs, app := newTestApp(t)
	res := sendJSON(t, app, http.MethodPost, "/api/admin/apps", store.AppInput{
		ID: "Bad ID", Category: "nope", Visibility: "groups", Icon: "not-a-tabler-icon",
		URLs:  map[string]string{"staging": "not a url"},
		Links: map[string]string{"wiki": "https://x.example"},
	})
	var bad Invalid
	json.Unmarshal([]byte(res.body), &bad)
	for _, f := range []string{"id", "name", "owner", "category", "urls.prod", "urls.staging", "links.wiki", "icon"} {
		if bad.Fields[f] == "" {
			t.Errorf("no error for %s: %s", f, res.body)
		}
	}
	if res.code != http.StatusBadRequest || fs.created != nil {
		t.Fatalf("got %d", res.code)
	}

	res = sendJSON(t, app, http.MethodPost, "/api/admin/apps", store.AppInput{
		ID: "x1", Name: "X", Owner: "ghost", Visibility: "everyone", URLs: map[string]string{"prod": "https://x.example"},
	})
	bad = Invalid{}
	json.Unmarshal([]byte(res.body), &bad)
	if res.code != http.StatusBadRequest || bad.Fields["owner"] != "Pick a team from the list" || fs.created != nil {
		t.Fatalf("unknown owner: %d %s", res.code, res.body)
	}

	res = sendJSON(t, app, http.MethodPost, "/api/admin/apps", store.AppInput{
		ID: "kafka-ui", Name: " Kafka UI ", Owner: "data", Visibility: "everyone", Groups: []string{"x", "x"}, Icon: "brand-docker",
		URLs: map[string]string{"prod": "kafka.internal.example", "staging": ""},
	})
	if res.code != http.StatusCreated {
		t.Fatalf("valid app: %d %s", res.code, res.body)
	}
	if fs.created.Name != "Kafka UI" || fs.created.URLs["prod"] != "https://kafka.internal.example" ||
		len(fs.created.URLs) != 1 || len(fs.created.Groups) != 1 {
		t.Fatalf("not cleaned up: %+v", fs.created)
	}

	res = sendJSON(t, app, http.MethodPost, "/api/admin/apps", store.AppInput{
		ID: "grafana", Name: "Grafana", Owner: "p", Visibility: "everyone", URLs: map[string]string{"prod": "https://g.example"},
	})
	if res.code != http.StatusConflict || !strings.Contains(res.body, `"id"`) {
		t.Fatalf("duplicate: %d %s", res.code, res.body)
	}
}

func TestTablerIcons(t *testing.T) {
	_, app := newTestApp(t)
	res := get(t, app, "/api/icons/tabler/brand-docker.svg")
	if res.code != http.StatusOK || res.header.Get("Content-Type") != "image/svg+xml" || !strings.HasPrefix(res.body, "<svg") {
		t.Fatalf("icon: %d %s %s", res.code, res.header.Get("Content-Type"), res.body)
	}
	if res := get(t, app, "/api/icons/tabler/nope-nope"); res.code != http.StatusNotFound {
		t.Fatalf("unknown icon: %d", res.code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/icons/tabler", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	res = do(t, app, req)
	if res.code != http.StatusOK || res.header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("gzip index: %d %v", res.code, res.header)
	}
	res = get(t, app, "/api/icons/tabler")
	if res.code != http.StatusOK || !strings.Contains(res.body, `"n":"brand-docker"`) {
		t.Fatalf("plain index: %d", res.code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/icons/tabler", nil)
	req.Header.Set("If-None-Match", res.header.Get("ETag"))
	if res := do(t, app, req); res.code != http.StatusNotModified {
		t.Fatalf("etag: %d", res.code)
	}
}

func uploadReq(t *testing.T, name string, data []byte) *http.Request {
	return uploadReqTo(t, "/api/apps/grafana/icon", name, data)
}

func uploadReqTo(t *testing.T, path, name string, data []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("icon", name)
	part.Write(data)
	w.Close()
	req := httptest.NewRequest(http.MethodPut, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func pngOf(size int) []byte {
	var buf bytes.Buffer
	png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, size, size)))
	return buf.Bytes()
}

func TestUploadIcon(t *testing.T) {
	fs, app := newTestApp(t)
	cases := []struct {
		name string
		data []byte
		code int
	}{
		{"small.png", pngOf(64), http.StatusBadRequest},
		{"ok.png", pngOf(128), http.StatusOK},
		{"ok.svg", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`), http.StatusOK},
		{"x.gif", []byte("GIF89a"), http.StatusBadRequest},
		{"big.svg", append([]byte("<svg>"), make([]byte, 300<<10)...), http.StatusBadRequest},
	}
	for _, c := range cases {
		fs.icon = nil
		res := do(t, app, uploadReq(t, c.name, c.data))
		if res.code != c.code {
			t.Errorf("%s: %d %s", c.name, res.code, res.body)
		}
		if c.code == http.StatusOK && !bytes.Equal(fs.icon, c.data) {
			t.Errorf("%s: icon not stored", c.name)
		}
	}
}

func TestUnknownAPIPathIsJSON404NotSPA(t *testing.T) {
	_, app := newTestApp(t)
	res := get(t, app, "/api/nope")
	if res.code != http.StatusNotFound || !strings.Contains(res.body, `"error"`) {
		t.Fatalf("got %d %q", res.code, res.body)
	}
}

func TestSPAFallbackAndAssets(t *testing.T) {
	_, app := newTestApp(t)
	for _, p := range []string{"/", "/apps/grafana", "/admin/apps"} {
		res := get(t, app, p)
		if res.code != http.StatusOK || !strings.Contains(res.body, "spa") {
			t.Fatalf("%s: %d %q", p, res.code, res.body)
		}
	}
	res := get(t, app, "/assets/app-1.js")
	if res.code != http.StatusOK || !strings.Contains(res.header.Get("Cache-Control"), "immutable") ||
		!strings.Contains(res.header.Get("Content-Type"), "javascript") {
		t.Fatalf("asset: %d %v", res.code, res.header)
	}
}

func TestDocs(t *testing.T) {
	_, app := newTestApp(t)
	if res := get(t, app, "/api/docs/index.html"); res.code != http.StatusOK || !strings.Contains(res.body, "swagger") {
		t.Fatalf("docs ui: %d", res.code)
	}
	res := get(t, app, "/api/docs/doc.json")
	if res.code != http.StatusOK || !strings.Contains(res.body, `"/admin/apps/{id}"`) {
		t.Fatalf("doc.json: %d %.200s", res.code, res.body)
	}
}

type denyAll struct{}

func (denyAll) Register(fiber.Router) {}
func (denyAll) LoginEnabled() bool    { return true }
func (denyAll) Methods() (bool, bool) { return true, true }

func (denyAll) Require(fiber.Ctx) error {
	return fiber.NewError(fiber.StatusUnauthorized, "login required")
}

func TestAPIIsBehindAuthButHealthzPortalAndSPAAreNot(t *testing.T) {
	app := New(&fakeStore{}, denyAll{}, testSPA, zerolog.Nop(), "")
	for _, p := range []string{"/api/apps", "/api/apps/x", "/api/me", "/api/nope", "/api/docs/index.html", "/api/admin/users"} {
		if res := get(t, app, p); res.code != http.StatusUnauthorized {
			t.Errorf("%s: %d", p, res.code)
		}
	}
	for _, p := range []string{"/healthz", "/", "/apps/x", "/api/portal"} {
		if res := get(t, app, p); res.code != http.StatusOK {
			t.Errorf("%s: %d", p, res.code)
		}
	}
	var p Portal
	json.Unmarshal([]byte(get(t, app, "/api/portal").body), &p)
	if !p.LoginSSO || !p.LoginLocal || p.Name != "Dev Portal" {
		t.Errorf("portal: %+v", p)
	}
}

type nonAdmin struct{ denyAll }

func (nonAdmin) Require(c fiber.Ctx) error {
	auth.SetUser(c, &store.User{Subject: "u1", Groups: []string{}})
	return c.Next()
}

func TestAdminRoutesNeedAdmin(t *testing.T) {
	app := New(&fakeStore{apps: []store.App{}}, nonAdmin{}, testSPA, zerolog.Nop(), "")
	if res := get(t, app, "/api/apps"); res.code != http.StatusOK {
		t.Fatalf("user api: %d", res.code)
	}
	for _, p := range []string{"/api/admin/users", "/api/admin/dashboard", "/api/admin/groups", "/api/admin/access-requests"} {
		if res := get(t, app, p); res.code != http.StatusForbidden {
			t.Errorf("%s: %d", p, res.code)
		}
	}
	if res := sendJSON(t, app, http.MethodPost, "/api/admin/apps", store.AppInput{}); res.code != http.StatusForbidden {
		t.Errorf("create app: %d", res.code)
	}
	if res := sendJSON(t, app, http.MethodPost, "/api/admin/searches/dismiss", map[string]string{"q": "x"}); res.code != http.StatusForbidden {
		t.Errorf("dismiss: %d", res.code)
	}
	for _, p := range []string{"/api/admin/access-requests/1/approve", "/api/admin/access-requests/1/decline"} {
		if res := sendJSON(t, app, http.MethodPost, p, map[string]string{"group": "x"}); res.code != http.StatusForbidden {
			t.Errorf("%s: %d", p, res.code)
		}
	}
}

func TestMeWithAuthDisabled(t *testing.T) {
	_, app := newTestApp(t)
	res := get(t, app, "/api/me")
	var me Me
	json.Unmarshal([]byte(res.body), &me)
	if res.code != http.StatusOK || me.LoginEnabled || me.User == nil || !me.User.IsAdmin {
		t.Fatalf("got %d %s", res.code, res.body)
	}
}

func TestPostToSPAIsRejected(t *testing.T) {
	_, app := newTestApp(t)
	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/apps/grafana", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func (f *fakeStore) CreateLocalUser(_ context.Context, _ store.Actor, in store.LocalUserInput, hash string) (store.User, error) {
	if !strings.HasPrefix(hash, "$2") {
		panic("password not hashed")
	}
	return store.User{Subject: store.LocalSubject(in.Username), Username: in.Username, Source: store.SourceLocal}, nil
}

func TestUserRules(t *testing.T) {
	_, app := newTestApp(t)
	res := sendJSON(t, app, http.MethodPost, "/api/admin/users", store.LocalUserInput{
		Username: "bad name", Email: "nope", Password: "short",
	})
	var bad Invalid
	json.Unmarshal([]byte(res.body), &bad)
	if res.code != http.StatusBadRequest || bad.Fields["username"] == "" || bad.Fields["email"] == "" ||
		bad.Fields["password"] != "Use at least 8 characters" {
		t.Fatalf("invalid user: %d %s", res.code, res.body)
	}
	res = sendJSON(t, app, http.MethodPost, "/api/admin/users", store.LocalUserInput{Username: "Budi", Password: "long enough"})
	if res.code != http.StatusCreated || !strings.Contains(res.body, `"local:budi"`) {
		t.Fatalf("create: %d %s", res.code, res.body)
	}

	// the local admin of a dev run is "local"
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/users/local", nil)
	if res := do(t, app, req); res.code != http.StatusBadRequest || !strings.Contains(res.body, "yourself") {
		t.Fatalf("delete self: %d %s", res.code, res.body)
	}
	res = sendJSON(t, app, http.MethodPut, "/api/admin/users/local", UserInput{Name: "Me", IsAdmin: false})
	if res.code != http.StatusBadRequest || !strings.Contains(res.body, "is_admin") {
		t.Fatalf("drop own admin: %d %s", res.code, res.body)
	}
	res = sendJSON(t, app, http.MethodPut, "/api/me/password", PasswordChange{Current: "x", Password: "long enough"})
	if res.code != http.StatusBadRequest || !strings.Contains(res.body, "sso") {
		t.Fatalf("change password as non-local user: %d %s", res.code, res.body)
	}
}

func (f *fakeStore) RenameOption(_ context.Context, _ store.Actor, kind, key, label string) error {
	if key != "prod" {
		return store.ErrNotFound
	}
	f.renamed = kind + "/" + key + "=" + label
	return nil
}

func (f *fakeStore) ReorderOptions(_ context.Context, _ store.Actor, kind string, keys []string) error {
	if len(keys) != 2 {
		return store.ErrConflict
	}
	f.reordered = keys
	return nil
}

func TestOptionsRenameAndReorder(t *testing.T) {
	fs, app := newTestApp(t)
	if res := sendJSON(t, app, http.MethodPut, "/api/admin/options/environment/prod", OptionInput{Label: " Live "}); res.code != http.StatusOK {
		t.Fatalf("rename: %d %s", res.code, res.body)
	}
	if fs.renamed != "environment/prod=Live" {
		t.Fatalf("renamed %q", fs.renamed)
	}
	if res := sendJSON(t, app, http.MethodPut, "/api/admin/options/environment/prod", OptionInput{Label: "  "}); res.code != http.StatusBadRequest {
		t.Fatalf("blank label: %d", res.code)
	}
	if res := sendJSON(t, app, http.MethodPut, "/api/admin/options/environment/nope", OptionInput{Label: "x"}); res.code != http.StatusNotFound {
		t.Fatalf("missing option: %d", res.code)
	}
	if res := sendJSON(t, app, http.MethodPut, "/api/admin/options/environment", OptionOrder{Keys: []string{"staging", "prod"}}); res.code != http.StatusOK {
		t.Fatalf("reorder: %d %s", res.code, res.body)
	}
	if !slices.Equal(fs.reordered, []string{"staging", "prod"}) {
		t.Fatalf("reordered %v", fs.reordered)
	}
	if res := sendJSON(t, app, http.MethodPut, "/api/admin/options/environment", OptionOrder{Keys: []string{"prod"}}); res.code != http.StatusConflict {
		t.Fatalf("stale order: %d", res.code)
	}
	if res := sendJSON(t, app, http.MethodPut, "/api/admin/options/colors", OptionOrder{}); res.code != http.StatusNotFound {
		t.Fatalf("unknown kind: %d", res.code)
	}
}

type fakeProber struct{ got string }

func (p *fakeProber) Check(_ context.Context, target string) store.Status {
	p.got = target
	return store.Status{Up: true, HTTPStatus: 200, LatencyMS: 12, Target: target}
}

func TestHealthCheckNow(t *testing.T) {
	fs := &fakeStore{}
	p := &fakeProber{}
	app := New(fs, auth.Disabled{}, testSPA, zerolog.Nop(), "", WithProber(p))
	res := sendJSON(t, app, http.MethodPost, "/api/admin/health-check", HealthCheckInput{
		HealthURL: "/healthz", URLs: map[string]string{"prod": "https://grafana.example.id/"},
	})
	if res.code != http.StatusOK || p.got != "https://grafana.example.id/healthz" {
		t.Fatalf("check: %d %s, target %q", res.code, res.body, p.got)
	}
	if res := sendJSON(t, app, http.MethodPost, "/api/admin/health-check", HealthCheckInput{}); res.code != http.StatusBadRequest {
		t.Fatalf("nothing to check: %d", res.code)
	}
	_, off := newTestApp(t)
	if res := sendJSON(t, off, http.MethodPost, "/api/admin/health-check", HealthCheckInput{HealthURL: "https://x.example.id"}); res.code != http.StatusServiceUnavailable {
		t.Fatalf("no prober: %d", res.code)
	}
}

type importStore struct {
	fakeStore
	gotData   string
	gotUpdate bool
}

func (f *importStore) PlanImport(_ context.Context, data []byte) (store.ImportPlan, error) {
	f.gotData = string(data)
	return store.ImportPlan{Apps: []store.ImportApp{{ID: "grafana"}}}, nil
}

func (f *importStore) ApplyImport(_ context.Context, _ store.Actor, data []byte, update bool) (store.ImportResult, store.ImportPlan, error) {
	f.gotData, f.gotUpdate = string(data), update
	if strings.Contains(f.gotData, "bad") {
		return store.ImportResult{}, store.ImportPlan{Problems: []catalog.Problem{{Line: 3, Message: "Name is required"}}}, store.ErrImportInvalid
	}
	return store.ImportResult{Added: []string{"grafana"}}, store.ImportPlan{}, nil
}

func TestImportEndpoints(t *testing.T) {
	fs := &importStore{}
	app := New(fs, auth.Disabled{}, testSPA, zerolog.Nop(), "")

	res := sendJSON(t, app, http.MethodPost, "/api/admin/import/preview", ImportInput{YAML: "version: 1"})
	if res.code != http.StatusOK || fs.gotData != "version: 1" || !strings.Contains(res.body, `"grafana"`) {
		t.Fatalf("preview: %d %s", res.code, res.body)
	}
	if res := sendJSON(t, app, http.MethodPost, "/api/admin/import", ImportInput{YAML: "x"}); res.code != http.StatusBadRequest {
		t.Fatalf("no existing choice: %d", res.code)
	}
	res = sendJSON(t, app, http.MethodPost, "/api/admin/import", ImportInput{YAML: "ok", Existing: "update"})
	if res.code != http.StatusOK || !fs.gotUpdate || !strings.Contains(res.body, `"added":["grafana"]`) {
		t.Fatalf("import: %d %s", res.code, res.body)
	}
	res = sendJSON(t, app, http.MethodPost, "/api/admin/import", ImportInput{YAML: "bad", Existing: "skip"})
	if res.code != http.StatusBadRequest || fs.gotUpdate || !strings.Contains(res.body, `"line":3`) {
		t.Fatalf("bad file: %d %s", res.code, res.body)
	}
	big := ImportInput{YAML: strings.Repeat("a", maxImportBytes+1), Existing: "skip"}
	if res := sendJSON(t, app, http.MethodPost, "/api/admin/import/preview", big); res.code != http.StatusRequestEntityTooLarge {
		t.Fatalf("big file: %d", res.code)
	}
}

func (f *fakeStore) LogSearch(_ context.Context, subject, q string, found bool) error {
	f.logged = []string{subject, q, strconv.FormatBool(found)}
	return nil
}

func (f *fakeStore) LogOpen(_ context.Context, _ store.Viewer, id string) error {
	if id == "hr" {
		return store.ErrNotFound
	}
	f.opened = id
	return nil
}

func (f *fakeStore) RecentOpens(_ context.Context, _ store.Viewer, _ int) ([]string, error) {
	return f.recent, nil
}

func (f *fakeStore) DismissSearch(_ context.Context, _ store.Actor, q string) error {
	f.dismissed = q
	return nil
}

func TestLogSearch(t *testing.T) {
	fs, app := newTestApp(t)
	res := sendJSON(t, app, http.MethodPost, "/api/searches", SearchLog{Q: "sonar", Found: false})
	if res.code != http.StatusNoContent {
		t.Fatalf("status %d %s", res.code, res.body)
	}
	if want := []string{"local", "sonar", "false"}; !reflect.DeepEqual(fs.logged, want) {
		t.Fatalf("logged %v, want %v", fs.logged, want)
	}
	for _, q := range []string{"", "  ", "\u0000"} {
		if res := sendJSON(t, app, http.MethodPost, "/api/searches", SearchLog{Q: q}); res.code != http.StatusBadRequest {
			t.Errorf("q %q: %d", q, res.code)
		}
	}
}

func TestLogOpen(t *testing.T) {
	fs, app := newTestApp(t)
	if res := sendJSON(t, app, http.MethodPost, "/api/apps/grafana/open", nil); res.code != http.StatusNoContent {
		t.Fatalf("status %d", res.code)
	}
	if fs.opened != "grafana" {
		t.Fatalf("opened %q", fs.opened)
	}
	if res := sendJSON(t, app, http.MethodPost, "/api/apps/hr/open", nil); res.code != http.StatusNotFound {
		t.Fatalf("hr: %d", res.code)
	}
}

func TestListAppsRecent(t *testing.T) {
	fs, app := newTestApp(t)
	fs.recent = []string{"grafana"}
	if res := get(t, app, "/api/apps"); !strings.Contains(res.body, `"recent":["grafana"]`) {
		t.Fatalf("body %s", res.body)
	}
	if res := get(t, app, "/api/apps?q=x"); !strings.Contains(res.body, `"recent":[]`) {
		t.Fatalf("body %s", res.body)
	}
	if res := get(t, app, "/api/apps?q=%20"); !strings.Contains(res.body, `"recent":["grafana"]`) {
		t.Fatalf("blank q: %s", res.body)
	}
	fs.recent = nil
	if res := get(t, app, "/api/apps"); !strings.Contains(res.body, `"recent":[]`) {
		t.Fatalf("nil recent: %s", res.body)
	}
}

func TestDismissSearch(t *testing.T) {
	fs, app := newTestApp(t)
	if res := sendJSON(t, app, http.MethodPost, "/api/admin/searches/dismiss", map[string]string{"q": "sonar"}); res.code != http.StatusNoContent {
		t.Fatalf("status %d", res.code)
	}
	if fs.dismissed != "sonar" {
		t.Fatalf("dismissed %q", fs.dismissed)
	}
	if res := sendJSON(t, app, http.MethodPost, "/api/admin/searches/dismiss", map[string]string{"q": " "}); res.code != http.StatusBadRequest {
		t.Fatalf("empty: %d", res.code)
	}
}

func TestUpdateDetails(t *testing.T) {
	fs := &fakeStore{apps: []store.App{
		{ID: "grafana", Name: "Grafana", Owner: "platform", Category: "monitoring", Visibility: "everyone", CanEdit: true},
		{ID: "kibana", Name: "Kibana"},
	}}
	app := New(fs, nonAdmin{}, testSPA, zerolog.Nop(), "")
	body := store.DetailsInput{Description: "Dashboards", URLs: map[string]string{"prod": "https://g.example"}}
	if res := sendJSON(t, app, http.MethodPut, "/api/apps/kibana/details", body); res.code != http.StatusForbidden || fs.details != nil {
		t.Fatalf("kibana: %d %s", res.code, res.body)
	}
	if res := sendJSON(t, app, http.MethodPut, "/api/apps/hr/details", body); res.code != http.StatusNotFound || fs.details != nil {
		t.Fatalf("hr: %d %s", res.code, res.body)
	}
	if res := sendJSON(t, app, http.MethodPut, "/api/apps/nope/details", body); res.code != http.StatusNotFound {
		t.Fatalf("missing: %d", res.code)
	}
	res := sendJSON(t, app, http.MethodPut, "/api/apps/grafana/details", body)
	var got store.App
	json.Unmarshal([]byte(res.body), &got)
	if res.code != http.StatusOK || fs.details == nil || fs.details.Description != "Dashboards" || !got.CanEdit {
		t.Fatalf("grafana: %d %s %+v", res.code, res.body, fs.details)
	}
	fs.details = nil
	body.URLs = map[string]string{"prod": "ftp://x"}
	res = sendJSON(t, app, http.MethodPut, "/api/apps/grafana/details", body)
	var bad Invalid
	json.Unmarshal([]byte(res.body), &bad)
	if res.code != http.StatusBadRequest || bad.Fields["urls.prod"] == "" || fs.details != nil {
		t.Fatalf("bad url: %d %s", res.code, res.body)
	}
}

func TestIconRoutesMoved(t *testing.T) {
	fs, app := newTestApp(t)
	if res := do(t, app, uploadReq(t, "ok.png", pngOf(128))); res.code != http.StatusOK || fs.icon == nil {
		t.Fatalf("upload: %d %s", res.code, res.body)
	}
	if res := do(t, app, uploadReqTo(t, "/api/admin/apps/grafana/icon", "ok.png", pngOf(128))); res.code != http.StatusNotFound {
		t.Fatalf("old route: %d", res.code)
	}
	if res := do(t, app, httptest.NewRequest(http.MethodDelete, "/api/apps/grafana/icon", nil)); res.code != http.StatusOK {
		t.Fatalf("delete: %d", res.code)
	}

	fs.icon = nil
	user := New(fs, nonAdmin{}, testSPA, zerolog.Nop(), "")
	if res := do(t, user, uploadReqTo(t, "/api/apps/kibana/icon", "ok.png", pngOf(128))); res.code != http.StatusForbidden || fs.icon != nil {
		t.Fatalf("kibana upload: %d", res.code)
	}
	if res := do(t, user, httptest.NewRequest(http.MethodDelete, "/api/apps/kibana/icon", nil)); res.code != http.StatusForbidden {
		t.Fatalf("kibana delete: %d", res.code)
	}
	if res := do(t, user, uploadReqTo(t, "/api/apps/hr/icon", "ok.png", pngOf(128))); res.code != http.StatusNotFound {
		t.Fatalf("hr upload: %d", res.code)
	}
	if res := do(t, user, uploadReq(t, "ok.png", pngOf(128))); res.code != http.StatusOK || fs.icon == nil {
		t.Fatalf("grafana upload: %d", res.code)
	}
}

func TestTeamPage(t *testing.T) {
	_, app := newTestApp(t)
	res := get(t, app, "/api/teams/platform")
	var team store.Team
	json.Unmarshal([]byte(res.body), &team)
	if res.code != http.StatusOK || len(team.Members) != 1 || team.Members[0] != "Ann" || len(team.Apps) != 1 {
		t.Fatalf("team: %d %s", res.code, res.body)
	}
	if res := get(t, app, "/api/teams/nope"); res.code != http.StatusNotFound {
		t.Fatalf("unknown team: %d", res.code)
	}
}

func (f *fakeStore) LatestAccessRequest(_ context.Context, _, appID string) (*store.AccessRequest, error) {
	if appID == "hr" {
		return &store.AccessRequest{ID: 1, AppID: "hr", Status: "pending"}, nil
	}
	return nil, nil
}

func (f *fakeStore) RequestAccess(_ context.Context, _ store.Viewer, appID, note string) (store.AccessRequest, error) {
	if appID != "hr" {
		return store.AccessRequest{}, store.ErrNotFound
	}
	return store.AccessRequest{ID: 1, AppID: "hr", Note: note, Status: "pending"}, nil
}

func (f *fakeStore) AccessRequests(context.Context) ([]store.AccessRequest, error) {
	return []store.AccessRequest{{ID: 1, AppID: "hr", Status: "pending"}}, nil
}

func (f *fakeStore) ApproveAccess(_ context.Context, _ store.Actor, id int64, group string) (store.AccessRequest, error) {
	switch {
	case id == 2:
		return store.AccessRequest{}, store.ErrDecided
	case group == "qa":
		return store.AccessRequest{}, store.ErrNotShared
	case id == 404:
		return store.AccessRequest{}, store.ErrNotFound
	}
	return store.AccessRequest{ID: id, Status: "approved", Group: group}, nil
}

func (f *fakeStore) DeclineAccess(_ context.Context, _ store.Actor, id int64) (store.AccessRequest, error) {
	if id == 2 {
		return store.AccessRequest{}, store.ErrDecided
	}
	return store.AccessRequest{ID: id, Status: "declined"}, nil
}

func TestAccessRequests(t *testing.T) {
	_, app := newTestApp(t)
	res := sendJSON(t, app, http.MethodPost, "/api/apps/hr/access-requests", map[string]string{"note": "need it"})
	var r store.AccessRequest
	json.Unmarshal([]byte(res.body), &r)
	if res.code != http.StatusCreated || r.Note != "need it" {
		t.Fatalf("request: %d %s", res.code, res.body)
	}
	if res := sendJSON(t, app, http.MethodPost, "/api/apps/hr/access-requests", nil); res.code != http.StatusCreated {
		t.Fatalf("empty body: %d %s", res.code, res.body)
	}
	if res := sendJSON(t, app, http.MethodPost, "/api/apps/grafana/access-requests", nil); res.code != http.StatusNotFound {
		t.Fatalf("visible app: %d", res.code)
	}
	if res := get(t, app, "/api/admin/access-requests"); res.code != http.StatusOK || !strings.Contains(res.body, `"requests"`) {
		t.Fatalf("list: %d %s", res.code, res.body)
	}
	if res := sendJSON(t, app, http.MethodPost, "/api/admin/access-requests/1/approve", map[string]string{"group": "hr-team"}); res.code != http.StatusOK {
		t.Fatalf("approve: %d %s", res.code, res.body)
	}
	if res := sendJSON(t, app, http.MethodPost, "/api/admin/access-requests/2/approve", map[string]string{"group": "hr-team"}); res.code != http.StatusConflict ||
		!strings.Contains(res.body, "this request was already handled") {
		t.Fatalf("decided: %d %s", res.code, res.body)
	}
	if res := sendJSON(t, app, http.MethodPost, "/api/admin/access-requests/1/approve", map[string]string{"group": "qa"}); res.code != http.StatusBadRequest ||
		!strings.Contains(res.body, "the app is not shared with that group") {
		t.Fatalf("not shared: %d %s", res.code, res.body)
	}
	if res := sendJSON(t, app, http.MethodPost, "/api/admin/access-requests/1/decline", nil); res.code != http.StatusOK {
		t.Fatalf("decline: %d %s", res.code, res.body)
	}
	if res := sendJSON(t, app, http.MethodPost, "/api/admin/access-requests/2/decline", nil); res.code != http.StatusConflict {
		t.Fatalf("decline decided: %d", res.code)
	}
	for _, id := range []string{"abc", "0", "-1"} {
		if res := sendJSON(t, app, http.MethodPost, "/api/admin/access-requests/"+id+"/decline", nil); res.code != http.StatusBadRequest {
			t.Errorf("id %s: %d", id, res.code)
		}
	}
}
