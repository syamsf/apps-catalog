package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"apps-catalog/internal/catalog"
	"apps-catalog/internal/store"
)

func newChecker(t *testing.T) *Checker {
	t.Helper()
	c, err := NewChecker(200*time.Millisecond, "")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCheck(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(200)
		case "/login":
			http.Redirect(w, r, "https://sso.example/", http.StatusFound)
		case "/missing":
			w.WriteHeader(404)
		case "/broken":
			w.WriteHeader(503)
		case "/slow":
			<-block
		}
	}))
	// cleanups run in reverse, so this unblocks /slow before srv.Close
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(block) })

	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	cases := []struct {
		path   string
		url    string
		up     bool
		status int
	}{
		{path: "/ok", up: true, status: 200},
		{path: "/login", up: true, status: 302},
		{path: "/missing", up: true, status: 404},
		{path: "/broken", up: false, status: 503},
		{path: "/slow", up: false},
		{url: closedURL, up: false},
	}
	c := newChecker(t)
	for _, tc := range cases {
		target := tc.url
		if target == "" {
			target = srv.URL + tc.path
		}
		st := c.Check(context.Background(), target)
		if st.Up != tc.up || st.HTTPStatus != tc.status {
			t.Errorf("%s: up=%v status=%d err=%q, want up=%v status=%d", target, st.Up, st.HTTPStatus, st.Error, tc.up, tc.status)
		}
		if !st.Up && st.Error == "" {
			t.Errorf("%s: down without an error message", target)
		}
		if st.Target != target || st.CheckedAt.IsZero() {
			t.Errorf("%s: missing target or time: %+v", target, st)
		}
	}
}

type fakeStore struct {
	mu      sync.Mutex
	apps    []catalog.App
	saved   map[string]store.Status
	cleared []string
}

func (f *fakeStore) AllApps(context.Context) ([]catalog.App, error) { return f.apps, nil }

func (f *fakeStore) SaveStatus(_ context.Context, id string, st store.Status) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved[id] = st
	return nil
}

func (f *fakeStore) ClearStatus(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleared = append(f.cleared, id)
	return nil
}

func TestCheckAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(srv.Close)

	fs := &fakeStore{saved: map[string]store.Status{}, apps: []catalog.App{
		{ID: "web", URLs: map[string]string{"prod": srv.URL}},
		{ID: "api", HealthURL: "/healthz", URLs: map[string]string{"prod": srv.URL}},
		{ID: "no-url"},
	}}
	log := zerolog.Nop()
	if err := newChecker(t).CheckAll(context.Background(), fs, log); err != nil {
		t.Fatal(err)
	}
	if !fs.saved["web"].Up {
		t.Errorf("web: %+v", fs.saved["web"])
	}
	if fs.saved["api"].Up || fs.saved["api"].Target != srv.URL+"/healthz" {
		t.Errorf("api should be down on /healthz: %+v", fs.saved["api"])
	}
	if len(fs.cleared) != 1 || fs.cleared[0] != "no-url" {
		t.Errorf("cleared = %v", fs.cleared)
	}
}
