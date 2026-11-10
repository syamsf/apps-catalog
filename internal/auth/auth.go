// Package auth does the sign-in, with OIDC (Authelia), with a local
// username and password, or both, and keeps sessions in the database.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
	"golang.org/x/oauth2"

	"apps-catalog/internal/store"
)

const (
	sessionCookie = "portal_session"
	loginCookie   = "portal_login"
)

type Config struct {
	// OIDC is off when empty
	Issuer       string
	ClientID     string
	ClientSecret string
	// base url of the portal, the callback is PublicURL + /auth/callback
	PublicURL string
	// members of this sso group are portal admins
	AdminGroup string
	SessionTTL time.Duration
	// where to send the browser after an sso logout, optional
	LogoutURL string
	// sign in with a username and password stored in the portal
	LocalLogin bool
}

type Store interface {
	SaveLogin(ctx context.Context, l store.Login) error
	TakeLogin(ctx context.Context, state string) (store.Login, error)
	SaveLoginUser(ctx context.Context, u store.User) error
	LocalCredentials(ctx context.Context, username string) (subject, hash string, err error)
	TouchLogin(ctx context.Context, subject string) error
	CreateSession(ctx context.Context, tokenHash []byte, subject string, expires time.Time) error
	Session(ctx context.Context, tokenHash []byte) (store.User, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
}

type Auth struct {
	cfg      Config
	st       Store
	log      zerolog.Logger
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	// nil when OIDC is off
	oauth  *oauth2.Config
	secure bool
	limit  *limiter
}

func New(ctx context.Context, cfg Config, st Store, log zerolog.Logger) (*Auth, error) {
	if cfg.Issuer == "" && !cfg.LocalLogin {
		return nil, errors.New("auth: turn on oidc, local login or both")
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 12 * time.Hour
	}
	public := strings.TrimRight(cfg.PublicURL, "/")
	a := &Auth{
		cfg:    cfg,
		st:     st,
		log:    log,
		secure: strings.HasPrefix(public, "https://"),
		limit:  newLimiter(maxTries, lockFor),
	}
	if cfg.Issuer == "" {
		return a, nil
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.PublicURL == "" {
		return nil, errors.New("oidc: client id, client secret and public url are required")
	}
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery for %s: %w", cfg.Issuer, err)
	}
	a.provider = provider
	a.verifier = provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})
	a.oauth = &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  public + "/auth/callback",
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}
	return a, nil
}

func (a *Auth) Register(r fiber.Router) {
	r.Get("/auth/login", a.login)
	if a.oauth != nil {
		r.Get("/auth/callback", a.callback)
	}
	if a.cfg.LocalLogin {
		r.Post("/auth/local", a.localLogin)
	}
	r.Post("/auth/logout", a.logout)
}

// Methods says which sign-in methods are on.
func (a *Auth) Methods() (sso, local bool) { return a.oauth != nil, a.cfg.LocalLogin }

func (a *Auth) login(c fiber.Ctx) error {
	if a.oauth == nil {
		// no sso, send them to the sign-in form
		return c.Redirect().Status(fiber.StatusFound).
			To("/login?redirect=" + url.QueryEscape(safeRedirect(c.Query("redirect"))))
	}
	l := store.Login{
		State:    randomToken(),
		Nonce:    randomToken(),
		Verifier: oauth2.GenerateVerifier(),
		// fiber reuses request buffers, so copy anything we keep
		Redirect: safeRedirect(strings.Clone(c.Query("redirect"))),
	}
	if err := a.st.SaveLogin(c.Context(), l); err != nil {
		return fmt.Errorf("save login: %w", err)
	}
	// ties the callback to this browser
	c.Cookie(&fiber.Cookie{
		Name: loginCookie, Value: l.State, Path: "/auth", MaxAge: 600,
		HTTPOnly: true, Secure: a.secure, SameSite: fiber.CookieSameSiteLaxMode,
	})
	return c.Redirect().Status(fiber.StatusFound).
		To(a.oauth.AuthCodeURL(l.State, oidc.Nonce(l.Nonce), oauth2.S256ChallengeOption(l.Verifier)))
}

func (a *Auth) callback(c fiber.Ctx) error {
	ctx := c.Context()
	if e := c.Query("error"); e != "" {
		return c.Status(fiber.StatusForbidden).SendString("login failed: " + e + " " + c.Query("error_description"))
	}
	state := strings.Clone(c.Query("state"))
	if state == "" || c.Cookies(loginCookie) != state {
		return c.Status(fiber.StatusBadRequest).SendString("login expired or started in another browser, go back and try again")
	}
	c.Cookie(&fiber.Cookie{Name: loginCookie, Path: "/auth", MaxAge: -1, HTTPOnly: true, Secure: a.secure})

	l, err := a.st.TakeLogin(ctx, state)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("login expired, go back and try again")
	}

	user, ssoGroups, err := a.exchange(ctx, strings.Clone(c.Query("code")), l)
	if err != nil {
		a.log.Warn().Err(err).Msg("oidc callback")
		return c.Status(fiber.StatusBadGateway).SendString("login failed")
	}
	user.IsAdmin = a.cfg.AdminGroup != "" && slices.Contains(ssoGroups, a.cfg.AdminGroup)
	if err := a.st.SaveLoginUser(ctx, user); err != nil {
		return fmt.Errorf("save user: %w", err)
	}

	if err := a.startSession(c, user.Subject); err != nil {
		return err
	}
	a.log.Info().Str("user", user.Username).Strs("sso_groups", ssoGroups).Bool("admin", user.IsAdmin).Msg("login")
	return c.Redirect().Status(fiber.StatusFound).To(l.Redirect)
}

func (a *Auth) exchange(ctx context.Context, code string, l store.Login) (store.User, []string, error) {
	tok, err := a.oauth.Exchange(ctx, code, oauth2.VerifierOption(l.Verifier))
	if err != nil {
		return store.User{}, nil, fmt.Errorf("token exchange: %w", err)
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok {
		return store.User{}, nil, errors.New("no id_token in token response")
	}
	idToken, err := a.verifier.Verify(ctx, rawID)
	if err != nil {
		return store.User{}, nil, fmt.Errorf("verify id token: %w", err)
	}
	if idToken.Nonce != l.Nonce {
		return store.User{}, nil, errors.New("nonce mismatch")
	}
	claims := map[string]any{}
	if err := idToken.Claims(&claims); err != nil {
		return store.User{}, nil, err
	}

	// Authelia puts groups, email etc in userinfo, not in the id token
	info, err := a.provider.UserInfo(ctx, oauth2.StaticTokenSource(tok))
	if err != nil {
		return store.User{}, nil, fmt.Errorf("userinfo: %w", err)
	}
	if info.Subject != idToken.Subject {
		return store.User{}, nil, errors.New("userinfo subject does not match id token")
	}
	extra := map[string]any{}
	if err := info.Claims(&extra); err != nil {
		return store.User{}, nil, err
	}
	for k, v := range extra {
		claims[k] = v
	}
	u, groups := userFromClaims(claims)
	return u, groups, nil
}

func (a *Auth) startSession(c fiber.Ctx, subject string) error {
	token := randomToken()
	if err := a.st.CreateSession(c.Context(), hashToken(token), subject, time.Now().Add(a.cfg.SessionTTL)); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	c.Cookie(&fiber.Cookie{
		Name: sessionCookie, Value: token, Path: "/", MaxAge: int(a.cfg.SessionTTL.Seconds()),
		HTTPOnly: true, Secure: a.secure, SameSite: fiber.CookieSameSiteLaxMode,
	})
	return nil
}

func (a *Auth) logout(c fiber.Ctx) error {
	ssoUser := false
	if token := strings.Clone(c.Cookies(sessionCookie)); token != "" {
		if u, err := a.st.Session(c.Context(), hashToken(token)); err == nil {
			ssoUser = u.Source != store.SourceLocal
		}
		if err := a.st.DeleteSession(c.Context(), hashToken(token)); err != nil {
			a.log.Error().Err(err).Msg("delete session")
		}
	}
	c.Cookie(&fiber.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HTTPOnly: true, Secure: a.secure})
	to := "/?signed_out=1"
	// local users have no sso session to end
	if a.cfg.LogoutURL != "" && ssoUser {
		to = a.cfg.LogoutURL
	}
	return c.Redirect().Status(fiber.StatusSeeOther).To(to)
}

// Require is middleware that rejects requests without a valid session.
func (a *Auth) Require(c fiber.Ctx) error {
	token := strings.Clone(c.Cookies(sessionCookie))
	if token == "" {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	u, err := a.st.Session(c.Context(), hashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	SetUser(c, &u)
	return c.Next()
}

func (a *Auth) LoginEnabled() bool { return true }

// LocalAdmin is the user of every request when login is off.
var LocalAdmin = store.User{Subject: "local", Username: "local", Name: "Local admin", Groups: []string{}, IsAdmin: true}

// Disabled is used when no OIDC issuer is configured. Everyone is an
// admin.
type Disabled struct{}

func (Disabled) Register(fiber.Router) {}

func (Disabled) Require(c fiber.Ctx) error {
	u := LocalAdmin
	SetUser(c, &u)
	return c.Next()
}

func (Disabled) LoginEnabled() bool { return false }

func (Disabled) Methods() (sso, local bool) { return false, false }

// RequireAdmin is middleware for admin-only routes, used after Require.
func RequireAdmin(c fiber.Ctx) error {
	if u := UserFrom(c); u == nil || !u.IsAdmin {
		return fiber.NewError(fiber.StatusForbidden, "admins only")
	}
	return c.Next()
}

type localsKey struct{}

// SetUser puts the signed in user on the request.
func SetUser(c fiber.Ctx, u *store.User) {
	c.Locals(localsKey{}, u)
}

// UserFrom returns the signed in user, nil outside Require.
func UserFrom(c fiber.Ctx) *store.User {
	u, _ := c.Locals(localsKey{}).(*store.User)
	return u
}

// ViewerFrom decides which apps the request can see: admins see all,
// others the apps shared with everyone or with their portal groups.
func ViewerFrom(c fiber.Ctx) store.Viewer {
	u := UserFrom(c)
	if u == nil {
		return store.Viewer{}
	}
	return store.Viewer{Subject: u.Subject, All: u.IsAdmin, Groups: u.Groups}
}

// ActorFrom is the signed in user for the change log.
func ActorFrom(c fiber.Ctx) store.Actor {
	u := UserFrom(c)
	if u == nil {
		return store.Actor{}
	}
	return store.Actor{Subject: u.Subject, Name: u.DisplayName()}
}

// userFromClaims returns the user and its sso groups.
func userFromClaims(c map[string]any) (store.User, []string) {
	str := func(k string) string { s, _ := c[k].(string); return s }
	u := store.User{
		Subject:  str("sub"),
		Username: str("preferred_username"),
		Name:     str("name"),
		Email:    str("email"),
		Groups:   []string{},
	}
	groups := []string{}
	switch g := c["groups"].(type) {
	case []any:
		for _, v := range g {
			if s, ok := v.(string); ok && s != "" {
				groups = append(groups, s)
			}
		}
	case string:
		if g != "" {
			groups = append(groups, g)
		}
	}
	if u.Username == "" {
		u.Username = u.Subject
	}
	return u, groups
}

// safeRedirect only allows local paths, so the login can't be used to
// send people to another site.
func safeRedirect(s string) string {
	if s == "" || !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || strings.HasPrefix(s, "/\\") {
		return "/"
	}
	u, err := url.Parse(s)
	if err != nil || u.Host != "" || u.Scheme != "" {
		return "/"
	}
	return s
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(t string) []byte {
	h := sha256.Sum256([]byte(t))
	return h[:]
}
