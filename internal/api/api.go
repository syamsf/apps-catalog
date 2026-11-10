// Package api has the http handlers.
//
//	@title			App Catalog API
//	@version		1.0
//	@description	Internal app catalog. When login is on, every endpoint needs the session cookie set by /auth/login.
//	@BasePath		/api
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"mime"
	"net/url"
	"path"
	"strings"
	"time"

	swaggo "github.com/gofiber/contrib/v3/swaggo"
	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"

	_ "apps-catalog/internal/api/docs"
	"apps-catalog/internal/auth"
	"apps-catalog/internal/store"
)

//go:generate go run github.com/swaggo/swag/cmd/swag@v1.16.6 init --generalInfo api.go --dir ./,../store,../catalog --output ./docs --outputTypes go,json --parseInternal

// Store is *store.Store.
type Store interface {
	Ping(ctx context.Context) error

	List(ctx context.Context, v store.Viewer, f store.Filter) ([]store.App, error)
	Get(ctx context.Context, v store.Viewer, id string) (store.App, error)
	Options(ctx context.Context, v store.Viewer, kind string) ([]store.Option, error)
	Icon(ctx context.Context, v store.Viewer, id string) (string, []byte, error)
	SetFavorite(ctx context.Context, v store.Viewer, id string, on bool) error

	CreateApp(ctx context.Context, by store.Actor, in store.AppInput) (store.App, error)
	UpdateApp(ctx context.Context, by store.Actor, id string, in store.AppInput) (store.App, error)
	DeleteApp(ctx context.Context, by store.Actor, id string) error
	SetIcon(ctx context.Context, by store.Actor, id, contentType string, data []byte) (store.App, error)
	DeleteIcon(ctx context.Context, by store.Actor, id string) (store.App, error)
	UpdateDetails(ctx context.Context, by store.Actor, id string, in store.DetailsInput) (store.App, error)
	Team(ctx context.Context, v store.Viewer, name string) (store.Team, error)

	Groups(ctx context.Context) ([]store.Group, error)
	Group(ctx context.Context, name string) (store.Group, error)
	CreateGroup(ctx context.Context, by store.Actor, in store.GroupInput) (store.Group, error)
	UpdateGroup(ctx context.Context, by store.Actor, name string, in store.GroupInput) (store.Group, error)
	DeleteGroup(ctx context.Context, by store.Actor, name string) error

	Users(ctx context.Context) ([]store.User, error)
	SetUserGroups(ctx context.Context, by store.Actor, subject string, groups []string) (store.User, error)
	CreateLocalUser(ctx context.Context, by store.Actor, in store.LocalUserInput, hash string) (store.User, error)
	UpdateLocalUser(ctx context.Context, by store.Actor, subject, name, email string, isAdmin bool) (store.User, error)
	SetPassword(ctx context.Context, by *store.Actor, subject, hash string) error
	PasswordHash(ctx context.Context, subject string) (string, error)
	DeleteUser(ctx context.Context, by store.Actor, subject string) error

	Settings(ctx context.Context) (store.Settings, error)
	UpdateSettings(ctx context.Context, by store.Actor, in store.SettingsInput) error
	AddOption(ctx context.Context, by store.Actor, kind, key, label string) error
	DeleteOption(ctx context.Context, by store.Actor, kind, key string) error
	RenameOption(ctx context.Context, by store.Actor, kind, key, label string) error
	ReorderOptions(ctx context.Context, by store.Actor, kind string, keys []string) error
	Dashboard(ctx context.Context) (store.Dashboard, error)

	LogSearch(ctx context.Context, subject, q string, found bool) error
	LogOpen(ctx context.Context, v store.Viewer, id string) error
	RecentOpens(ctx context.Context, v store.Viewer, limit int) ([]string, error)
	DismissSearch(ctx context.Context, by store.Actor, q string) error

	RequestAccess(ctx context.Context, v store.Viewer, appID, note string) (store.AccessRequest, error)
	LatestAccessRequest(ctx context.Context, subject, appID string) (*store.AccessRequest, error)
	AccessRequests(ctx context.Context) ([]store.AccessRequest, error)
	ApproveAccess(ctx context.Context, by store.Actor, id int64, group string) (store.AccessRequest, error)
	DeclineAccess(ctx context.Context, by store.Actor, id int64) (store.AccessRequest, error)

	PlanImport(ctx context.Context, data []byte) (store.ImportPlan, error)
	ApplyImport(ctx context.Context, by store.Actor, data []byte, update bool) (store.ImportResult, store.ImportPlan, error)
}

// Auth is *auth.Auth or auth.Disabled.
type Auth interface {
	Register(r fiber.Router)
	Require(c fiber.Ctx) error
	LoginEnabled() bool
	Methods() (sso, local bool)
}

type Error struct {
	Error string `json:"error"`
}

// Invalid is a 400 with a message per form field.
type Invalid struct {
	Error  string            `json:"error"`
	Fields map[string]string `json:"fields"`
}

type Health struct {
	Status string `json:"status"`
}

// Prober is *health.Checker, used for the admin "test now" button.
type Prober interface {
	Check(ctx context.Context, target string) store.Status
}

type Option func(*handler)

func WithProber(p Prober) Option {
	return func(h *handler) { h.prober = p }
}

func New(st Store, authn Auth, spa fs.FS, log zerolog.Logger, adminGroup string, opts ...Option) *fiber.App {
	h := &handler{st: st, authn: authn, log: log, adminGroup: adminGroup}
	for _, o := range opts {
		o(h)
	}
	app := fiber.New(fiber.Config{
		Immutable:    true,
		ErrorHandler: h.errorHandler,
		ReadTimeout:  10 * time.Second,
	})
	app.Use(requestLogger(log))

	app.Get("/healthz", h.healthz)
	authn.Register(app)
	// before the /api group so it works signed out
	app.Get("/api/portal", h.portal)

	api := app.Group("/api", authn.Require)
	api.Get("/me", h.me)
	api.Get("/settings", h.settings)
	api.Get("/apps", h.listApps)
	api.Get("/apps/:id", h.getApp)
	api.Get("/apps/:id/icon", h.icon)
	api.Put("/apps/:id/details", h.updateDetails)
	api.Put("/apps/:id/icon", h.uploadIcon)
	api.Delete("/apps/:id/icon", h.deleteIcon)
	api.Get("/teams/:name", h.getTeam)
	api.Post("/apps/:id/open", h.logOpen)
	api.Post("/apps/:id/access-requests", h.requestAccess)
	api.Post("/searches", h.logSearch)
	api.Get("/facets", h.facets)
	api.Get("/icons/tabler", h.tablerIndex)
	api.Get("/icons/tabler/:name", h.tablerIcon)
	api.Put("/me/password", h.changePassword)
	api.Put("/favorites/:id", h.addFavorite)
	api.Delete("/favorites/:id", h.removeFavorite)
	api.Get("/docs/*", swaggo.HandlerDefault)

	admin := api.Group("/admin", auth.RequireAdmin)
	admin.Get("/dashboard", h.dashboard)
	admin.Post("/apps", h.createApp)
	admin.Put("/apps/:id", h.updateApp)
	admin.Delete("/apps/:id", h.deleteApp)
	admin.Get("/groups", h.listGroups)
	admin.Get("/groups/:name", h.getGroup)
	admin.Post("/groups", h.createGroup)
	admin.Put("/groups/:name", h.updateGroup)
	admin.Delete("/groups/:name", h.deleteGroup)
	admin.Get("/users", h.listUsers)
	admin.Post("/users", h.createUser)
	admin.Put("/users/:subject", h.updateUser)
	admin.Put("/users/:subject/password", h.setUserPassword)
	admin.Delete("/users/:subject", h.deleteUser)
	admin.Put("/users/:subject/groups", h.setUserGroups)
	admin.Put("/settings", h.updateSettings)
	admin.Post("/options/:kind", h.addOption)
	admin.Delete("/options/:kind/:key", h.deleteOption)
	admin.Put("/options/:kind", h.reorderOptions)
	admin.Put("/options/:kind/:key", h.renameOption)
	admin.Post("/searches/dismiss", h.dismissSearch)
	admin.Get("/access-requests", h.listAccessRequests)
	admin.Post("/access-requests/:id/approve", h.approveAccess)
	admin.Post("/access-requests/:id/decline", h.declineAccess)
	admin.Post("/health-check", h.healthCheck)
	admin.Post("/import/preview", h.previewImport)
	admin.Post("/import", h.runImport)

	api.Use(func(c fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotFound, "no such endpoint")
	})

	if spa != nil {
		app.Use(spaHandler(spa))
	}
	return app
}

type handler struct {
	st         Store
	authn      Auth
	log        zerolog.Logger
	adminGroup string
	prober     Prober
}

// param returns a path param, url decoded.
func param(c fiber.Ctx, name string) string {
	v := c.Params(name)
	if u, err := url.PathUnescape(v); err == nil {
		return u
	}
	return v
}

// storeErr maps store errors to http errors.
func storeErr(err error, notFound string) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, notFound)
	case errors.Is(err, store.ErrConflict):
		return fiber.NewError(fiber.StatusConflict, "that name is already taken")
	case errors.Is(err, store.ErrInUse):
		return fiber.NewError(fiber.StatusConflict, "still used by apps, move those apps first")
	case errors.Is(err, store.ErrUnknownRef):
		return fiber.NewError(fiber.StatusBadRequest, "unknown group, person or app")
	case errors.Is(err, store.ErrDecided):
		return fiber.NewError(fiber.StatusConflict, "this request was already handled")
	case errors.Is(err, store.ErrNotShared):
		return fiber.NewError(fiber.StatusBadRequest, "the app is not shared with that group")
	case errors.Is(err, store.ErrNotLocal):
		return fiber.NewError(fiber.StatusBadRequest, "this person signs in with sso, change it there")
	}
	return err
}

func bindJSON(c fiber.Ctx, v any) error {
	if err := json.Unmarshal(c.Body(), v); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid json body")
	}
	return nil
}

func (h *handler) healthz(c fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
	defer cancel()
	if err := h.st.Ping(ctx); err != nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "database unreachable")
	}
	return c.JSON(Health{Status: "ok"})
}

func (h *handler) errorHandler(c fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	msg := "internal error"
	var fe *fiber.Error
	if errors.As(err, &fe) {
		code, msg = fe.Code, fe.Message
	} else {
		h.log.Error().Err(err).Str("path", c.Path()).Msg("request failed")
	}
	return c.Status(code).JSON(Error{Error: msg})
}

// serve the file if it exists, otherwise index.html (vue router handles it)
func spaHandler(spa fs.FS) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
			return fiber.NewError(fiber.StatusMethodNotAllowed, "method not allowed")
		}
		name := strings.TrimPrefix(path.Clean(c.Path()), "/")
		if name != "" {
			if data, err := fs.ReadFile(spa, name); err == nil {
				if strings.HasPrefix(name, "assets/") {
					c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
				}
				if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
					c.Set(fiber.HeaderContentType, ct)
				}
				return c.Send(data)
			}
		}
		index, err := fs.ReadFile(spa, "index.html")
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("frontend not built: run `make web`")
		}
		c.Set(fiber.HeaderCacheControl, "no-cache")
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.Send(index)
	}
}

func requestLogger(log zerolog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		if err != nil {
			// let the error handler set the status before we log it
			if herr := c.App().Config().ErrorHandler(c, err); herr != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
		}
		if c.Path() == "/healthz" {
			return nil
		}
		log.Info().
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", c.Response().StatusCode()).
			Dur("duration_ms", time.Since(start)).
			Msg("http")
		return nil
	}
}
