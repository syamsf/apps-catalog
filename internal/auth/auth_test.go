package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
	"golang.org/x/crypto/bcrypt"

	"apps-catalog/internal/store"
)

// fake OIDC provider, behaves like Authelia: groups only in userinfo
type fakeOP struct {
	t         *testing.T
	srv       *httptest.Server
	key       *rsa.PrivateKey
	mu        sync.Mutex
	challenge string
	nonce     string
	groups    []string
}

func newFakeOP(t *testing.T) *fakeOP {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	op := &fakeOP{t: t, key: key, groups: []string{"dev", "platform"}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                op.srv.URL,
			"authorization_endpoint":                op.srv.URL + "/authorize",
			"token_endpoint":                        op.srv.URL + "/token",
			"userinfo_endpoint":                     op.srv.URL + "/userinfo",
			"jwks_uri":                              op.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"},
		}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		op.mu.Lock()
		challenge, nonce := op.challenge, op.nonce
		op.mu.Unlock()
		if r.Form.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at-1",
			"token_type":   "Bearer",
			"expires_in":   300,
			"id_token":     op.idToken(nonce),
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at-1" {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"sub": "u-1", "preferred_username": "budi", "name": "Budi", "email": "budi@example.id", "groups": op.groups,
		})
	})
	op.srv = httptest.NewServer(mux)
	t.Cleanup(op.srv.Close)
	return op
}

func (op *fakeOP) idToken(nonce string) string {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: op.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		op.t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{
		"iss": op.srv.URL, "sub": "u-1", "aud": "portal", "nonce": nonce,
		"iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(),
	})
	obj, err := signer.Sign(payload)
	if err != nil {
		op.t.Fatal(err)
	}
	s, _ := obj.CompactSerialize()
	return s
}

type memStore struct {
	mu       sync.Mutex
	logins   map[string]store.Login
	users    map[string]store.User
	sessions map[string]string // token hash -> subject
	hashes   map[string]string // local username -> hash
}

func (m *memStore) LocalCredentials(_ context.Context, username string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.hashes[strings.ToLower(username)]
	if !ok {
		return "", "", store.ErrNotFound
	}
	return store.LocalSubject(username), h, nil
}

func (m *memStore) TouchLogin(context.Context, string) error { return nil }

func (m *memStore) SaveLoginUser(_ context.Context, u store.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// portal groups live in the portal, keep them
	if old, ok := m.users[u.Subject]; ok {
		u.Groups = old.Groups
	}
	m.users[u.Subject] = u
	return nil
}

func (m *memStore) SaveLogin(_ context.Context, l store.Login) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logins[l.State] = l
	return nil
}

func (m *memStore) TakeLogin(_ context.Context, state string) (store.Login, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.logins[state]
	if !ok {
		return store.Login{}, store.ErrNotFound
	}
	delete(m.logins, state)
	return l, nil
}

func (m *memStore) CreateSession(_ context.Context, h []byte, subject string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[string(h)] = subject
	return nil
}

func (m *memStore) Session(_ context.Context, h []byte) (store.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sub, ok := m.sessions[string(h)]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return m.users[sub], nil
}

func (m *memStore) DeleteSession(_ context.Context, h []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, string(h))
	return nil
}

type setup struct {
	op     *fakeOP
	appURL string
	client *http.Client
	st     *memStore
}

func newSetup(t *testing.T, adminGroup string) *setup {
	t.Helper()
	return newSetupWith(t, func(c *Config) { c.AdminGroup = adminGroup })
}

func newSetupWith(t *testing.T, mod func(*Config)) *setup {
	t.Helper()
	op := newFakeOP(t)
	st := &memStore{logins: map[string]store.Login{}, users: map[string]store.User{}, sessions: map[string]string{},
		hashes: map[string]string{}}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	appURL := "http://" + ln.Addr().String()

	cfg := Config{Issuer: op.srv.URL, ClientID: "portal", ClientSecret: "secret", PublicURL: appURL}
	mod(&cfg)
	a, err := New(context.Background(), cfg, st, zerolog.New(zerolog.NewTestWriter(t)))
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	a.Register(app)
	app.Get("/api/me", a.Require, func(c fiber.Ctx) error {
		return c.JSON(map[string]any{"user": UserFrom(c), "viewer": ViewerFrom(c)})
	})
	go app.Listener(ln, fiber.ListenConfig{DisableStartupMessage: true})
	t.Cleanup(func() { app.Shutdown() })

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	return &setup{op: op, appURL: appURL, client: client, st: st}
}

// startLogin hits /auth/login and plays the provider's authorize step,
// returning the state.
func (s *setup) startLogin(t *testing.T, redirect string) string {
	t.Helper()
	resp, err := s.client.Get(s.appURL + "/auth/login?redirect=" + url.QueryEscape(redirect))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || !strings.HasPrefix(loc.String(), s.op.srv.URL+"/authorize") {
		t.Fatalf("login redirect = %q", resp.Header.Get("Location"))
	}
	q := loc.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" || !strings.Contains(q.Get("scope"), "groups") {
		t.Fatalf("authorize params: %v", q)
	}
	s.op.mu.Lock()
	s.op.challenge, s.op.nonce = q.Get("code_challenge"), q.Get("nonce")
	s.op.mu.Unlock()
	return q.Get("state")
}

func (s *setup) me(t *testing.T) (int, map[string]any) {
	t.Helper()
	resp, err := s.client.Get(s.appURL + "/api/me")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func TestLoginFlow(t *testing.T) {
	s := newSetup(t, "")

	if code, _ := s.me(t); code != http.StatusUnauthorized {
		t.Fatalf("before login: %d", code)
	}

	state := s.startLogin(t, "/apps/grafana")
	resp, err := s.client.Get(s.appURL + "/auth/callback?code=good-code&state=" + state)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/apps/grafana" {
		t.Fatalf("callback: %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	code, body := s.me(t)
	if code != http.StatusOK {
		t.Fatalf("after login: %d", code)
	}
	user := body["user"].(map[string]any)
	if user["username"] != "budi" || user["email"] != "budi@example.id" {
		t.Fatalf("user = %v", user)
	}
	// sso groups only decide admin, apps follow portal groups
	viewer := body["viewer"].(map[string]any)
	if viewer["All"] != false || viewer["Subject"] != "u-1" || user["is_admin"] != false {
		t.Fatalf("viewer = %v, user = %v", viewer, user)
	}

	// state is single use
	resp, _ = s.client.Get(s.appURL + "/auth/callback?code=good-code&state=" + state)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("replayed state: %d", resp.StatusCode)
	}

	resp, err = s.client.Post(s.appURL+"/auth/logout", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	if code, _ := s.me(t); code != http.StatusUnauthorized {
		t.Fatalf("after logout: %d", code)
	}
	if len(s.st.sessions) != 0 {
		t.Fatal("session not deleted")
	}
}

func TestAdminGroupSeesAll(t *testing.T) {
	s := newSetup(t, "platform")
	s.st.users["u-1"] = store.User{Subject: "u-1", Groups: []string{"data-team"}}
	state := s.startLogin(t, "/")
	resp, _ := s.client.Get(s.appURL + "/auth/callback?code=good-code&state=" + state)
	resp.Body.Close()
	_, body := s.me(t)
	viewer := body["viewer"].(map[string]any)
	if viewer["All"] != true || body["user"].(map[string]any)["is_admin"] != true {
		t.Fatalf("viewer = %v", viewer)
	}
	if g := viewer["Groups"].([]any); len(g) != 1 || g[0] != "data-team" {
		t.Fatalf("portal groups lost on login: %v", g)
	}
}

func TestRequireAdmin(t *testing.T) {
	app := fiber.New()
	app.Get("/user", func(c fiber.Ctx) error { SetUser(c, &store.User{Subject: "u"}); return c.Next() }, RequireAdmin,
		func(c fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/local", Disabled{}.Require, RequireAdmin, func(c fiber.Ctx) error { return c.SendString("ok") })
	for path, want := range map[string]int{"/user": http.StatusForbidden, "/local": http.StatusOK} {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Errorf("%s: %d, want %d", path, resp.StatusCode, want)
		}
	}
}

func TestCallbackRejects(t *testing.T) {
	s := newSetup(t, "")

	t.Run("callback from another browser", func(t *testing.T) {
		state := s.startLogin(t, "/")
		other := &http.Client{CheckRedirect: s.client.CheckRedirect}
		resp, _ := other.Get(s.appURL + "/auth/callback?code=good-code&state=" + state)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("got %d", resp.StatusCode)
		}
	})

	t.Run("bad code", func(t *testing.T) {
		state := s.startLogin(t, "/")
		resp, _ := s.client.Get(s.appURL + "/auth/callback?code=wrong&state=" + state)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadGateway {
			t.Fatalf("got %d", resp.StatusCode)
		}
	})

	t.Run("wrong nonce", func(t *testing.T) {
		state := s.startLogin(t, "/")
		s.op.mu.Lock()
		s.op.nonce = "something-else"
		s.op.mu.Unlock()
		resp, _ := s.client.Get(s.appURL + "/auth/callback?code=good-code&state=" + state)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadGateway {
			t.Fatalf("got %d", resp.StatusCode)
		}
	})

	t.Run("provider error", func(t *testing.T) {
		resp, _ := s.client.Get(s.appURL + "/auth/callback?error=access_denied")
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("got %d", resp.StatusCode)
		}
	})

	if len(s.st.sessions) != 0 {
		t.Fatalf("sessions created: %d", len(s.st.sessions))
	}
}

func TestSafeRedirect(t *testing.T) {
	cases := map[string]string{
		"":                     "/",
		"/apps/grafana?q=x":    "/apps/grafana?q=x",
		"https://evil.example": "/",
		"//evil.example":       "/",
		"/\\evil.example":      "/",
		"apps":                 "/",
	}
	for in, want := range cases {
		if got := safeRedirect(in); got != want {
			t.Errorf("safeRedirect(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUserFromClaims(t *testing.T) {
	u, groups := userFromClaims(map[string]any{"sub": "x", "groups": "solo"})
	if u.Username != "x" || len(groups) != 1 || groups[0] != "solo" {
		t.Fatalf("%+v %v", u, groups)
	}
	u, groups = userFromClaims(map[string]any{"sub": "x"})
	if groups == nil || u.Groups == nil {
		t.Fatal("groups should be empty, not nil")
	}
}

func TestMain(m *testing.M) {
	bcryptCost = bcrypt.MinCost
	os.Exit(m.Run())
}

// addLocal adds a local user to the fake store.
func (s *setup) addLocal(t *testing.T, username, pw string, admin bool) {
	t.Helper()
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	sub := store.LocalSubject(username)
	s.st.hashes[username] = h
	s.st.users[sub] = store.User{Subject: sub, Username: username, Groups: []string{}, IsAdmin: admin, Source: store.SourceLocal}
}

func (s *setup) local(t *testing.T, username, pw string) int {
	t.Helper()
	b, _ := json.Marshal(LocalLogin{Username: username, Password: pw})
	resp, err := s.client.Post(s.appURL+"/auth/local", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestLocalLoginOnly(t *testing.T) {
	s := newSetupWith(t, func(c *Config) {
		*c = Config{LocalLogin: true, PublicURL: c.PublicURL, LogoutURL: "https://auth.example/logout"}
	})
	s.addLocal(t, "budi", "correct horse", true)

	if code := s.local(t, "budi", "wrong password"); code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", code)
	}
	if code := s.local(t, "nobody", "correct horse"); code != http.StatusUnauthorized {
		t.Fatalf("unknown user: %d", code)
	}
	if code := s.local(t, "BUDI", "correct horse"); code != http.StatusNoContent {
		t.Fatalf("login: %d", code)
	}
	code, body := s.me(t)
	if code != http.StatusOK || body["viewer"].(map[string]any)["All"] != true {
		t.Fatalf("me: %d %v", code, body)
	}

	// no sso: /auth/login points at the form, no callback route
	resp, _ := s.client.Get(s.appURL + "/auth/login?redirect=/apps/x")
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); resp.StatusCode != http.StatusFound || loc != "/login?redirect=%2Fapps%2Fx" {
		t.Fatalf("auth/login: %d %q", resp.StatusCode, loc)
	}
	resp, _ = s.client.Get(s.appURL + "/auth/callback?code=x&state=y")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("callback without sso: %d", resp.StatusCode)
	}

	// local users skip the sso logout url
	resp, _ = s.client.Post(s.appURL+"/auth/logout", "", nil)
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != "/?signed_out=1" {
		t.Fatalf("logout to %q", loc)
	}
}

func TestLocalLoginLockout(t *testing.T) {
	s := newSetupWith(t, func(c *Config) { c.LocalLogin = true })
	s.addLocal(t, "budi", "correct horse", false)
	for range maxTries {
		if code := s.local(t, "budi", "nope nope"); code != http.StatusUnauthorized {
			t.Fatalf("try: %d", code)
		}
	}
	if code := s.local(t, "budi", "correct horse"); code != http.StatusTooManyRequests {
		t.Fatalf("after %d wrong tries: %d", maxTries, code)
	}
	// other usernames are not blocked
	s.addLocal(t, "sari", "another one", false)
	if code := s.local(t, "sari", "another one"); code != http.StatusNoContent {
		t.Fatalf("other user: %d", code)
	}
}

func TestLimiterWindow(t *testing.T) {
	now := time.Unix(0, 0)
	l := newLimiter(2, time.Minute)
	l.now = func() time.Time { return now }
	l.fail("k")
	l.fail("k")
	if !l.blocked("k") {
		t.Fatal("should be blocked")
	}
	now = now.Add(61 * time.Second)
	if l.blocked("k") {
		t.Fatal("should be open after the window")
	}
}

func TestHashPassword(t *testing.T) {
	if _, err := HashPassword("short"); err != ErrPasswordShort {
		t.Fatalf("short: %v", err)
	}
	if _, err := HashPassword(strings.Repeat("x", 73)); err != ErrPasswordLong {
		t.Fatalf("long: %v", err)
	}
	h, err := HashPassword("long enough")
	if err != nil || !CheckPassword(h, "long enough") || CheckPassword(h, "long enougH") || CheckPassword("", "") {
		t.Fatalf("hash: %v", err)
	}
}

func TestNeedsAMethod(t *testing.T) {
	if _, err := New(context.Background(), Config{}, &memStore{}, zerolog.Nop()); err == nil {
		t.Fatal("no method should fail")
	}
}
