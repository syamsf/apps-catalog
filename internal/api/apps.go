package api

import (
	"bytes"
	"errors"
	"image/png"
	"io"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3"

	"apps-catalog/internal/auth"
	"apps-catalog/internal/store"
)

type Portal struct {
	Name       string `json:"name"`
	AdminGroup string `json:"admin_group"`
	// sign-in methods that are on
	LoginSSO   bool `json:"login_sso"`
	LoginLocal bool `json:"login_local"`
}

type Me struct {
	User         *store.User `json:"user"`
	LoginEnabled bool        `json:"login_enabled"`
}

type AppList struct {
	Apps  []store.App `json:"apps"`
	Total int         `json:"total"`
	// Recent is the viewer's last opened app ids, newest first. Empty when searching.
	Recent []string `json:"recent"`
}

type Facets struct {
	Categories []store.Option `json:"categories"`
	RunsOn     []store.Option `json:"runs_on"`
}

type AppName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// NoAccess is the 403 body for an app the user can't see.
type NoAccess struct {
	Error      string   `json:"error"`
	App        AppName  `json:"app"`
	Groups     []string `json:"groups"`
	AdminEmail string   `json:"admin_email"`
	// Request is the viewer's latest access request for this app, if any.
	Request *store.AccessRequest `json:"request"`
}

// portal godoc
//
//	@Summary		Portal name, admin group and sign-in methods
//	@Description	Works without a session, for the sign-in page.
//	@Tags			portal
//	@Produce		json
//	@Success		200	{object}	Portal
//	@Router			/portal [get]
func (h *handler) portal(c fiber.Ctx) error {
	st, err := h.st.Settings(c.Context())
	if err != nil {
		return err
	}
	sso, local := h.authn.Methods()
	return c.JSON(Portal{Name: st.Name, AdminGroup: h.adminGroup, LoginSSO: sso, LoginLocal: local})
}

// me godoc
//
//	@Summary	Current user
//	@Tags		portal
//	@Produce	json
//	@Success	200	{object}	Me
//	@Failure	401	{object}	Error
//	@Router		/me [get]
func (h *handler) me(c fiber.Ctx) error {
	return c.JSON(Me{User: auth.UserFrom(c), LoginEnabled: h.authn.LoginEnabled()})
}

// settings godoc
//
//	@Summary		Portal settings
//	@Description	Options for categories, runs on and environments, icon colors. Read only here.
//	@Tags			portal
//	@Produce		json
//	@Success		200	{object}	store.Settings
//	@Failure		401	{object}	Error
//	@Router			/settings [get]
func (h *handler) settings(c fiber.Ctx) error {
	st, err := h.st.Settings(c.Context())
	if err != nil {
		return err
	}
	st.AdminGroup = h.adminGroup
	return c.JSON(st)
}

// listApps godoc
//
//	@Summary		List apps
//	@Description	Apps the current user can see, sorted by name. Admins see all apps.
//	@Tags			apps
//	@Produce		json
//	@Param			q			query		string	false	"search in name, id, description, owner and urls"
//	@Param			category	query		string	false	"category keys, comma separated, matches any"
//	@Param			runs_on		query		string	false	"runs on keys, comma separated, matches any"
//	@Param			group		query		string	false	"apps shared with this group, or everyone"
//	@Success		200			{object}	AppList
//	@Failure		401			{object}	Error
//	@Router			/apps [get]
func (h *handler) listApps(c fiber.Ctx) error {
	apps, err := h.st.List(c.Context(), auth.ViewerFrom(c), store.Filter{
		Query:    c.Query("q"),
		Category: splitKeys(c.Query("category")),
		RunsOn:   splitKeys(c.Query("runs_on")),
		Group:    c.Query("group"),
	})
	if err != nil {
		return err
	}
	recent := []string{}
	if strings.TrimSpace(c.Query("q")) == "" {
		if recent, err = h.st.RecentOpens(c.Context(), auth.ViewerFrom(c), 5); err != nil {
			return err
		}
		if recent == nil {
			recent = []string{}
		}
	}
	return c.JSON(AppList{Apps: apps, Total: len(apps), Recent: recent})
}

// splitKeys turns "a,b" into [a b], dropping blanks.
func splitKeys(s string) []string {
	var out []string
	for k := range strings.SplitSeq(s, ",") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// getApp godoc
//
//	@Summary		Get one app
//	@Description	An app that exists but is not shared with the user returns 403 with the groups that can see it.
//	@Tags			apps
//	@Produce		json
//	@Param			id	path		string	true	"app id"
//	@Success		200	{object}	store.App
//	@Failure		401	{object}	Error
//	@Failure		403	{object}	NoAccess
//	@Failure		404	{object}	Error
//	@Router			/apps/{id} [get]
func (h *handler) getApp(c fiber.Ctx) error {
	app, err := h.st.Get(c.Context(), auth.ViewerFrom(c), param(c, "id"))
	var na *store.NoAccessError
	if errors.As(err, &na) {
		st, err := h.st.Settings(c.Context())
		if err != nil {
			return err
		}
		req, err := h.st.LatestAccessRequest(c.Context(), auth.ViewerFrom(c).Subject, na.ID)
		if err != nil {
			return err
		}
		return c.Status(fiber.StatusForbidden).JSON(NoAccess{
			Error: "no access", App: AppName{ID: na.ID, Name: na.Name}, Groups: na.Groups, AdminEmail: st.AdminEmail,
			Request: req,
		})
	}
	if err != nil {
		return storeErr(err, "app not found")
	}
	return c.JSON(app)
}

// icon godoc
//
//	@Summary	Uploaded icon of an app
//	@Tags		apps
//	@Produce	png
//	@Produce	image/svg+xml
//	@Param		id	path	string	true	"app id"
//	@Success	200
//	@Failure	404	{object}	Error
//	@Router		/apps/{id}/icon [get]
func (h *handler) icon(c fiber.Ctx) error {
	ct, data, err := h.st.Icon(c.Context(), auth.ViewerFrom(c), param(c, "id"))
	if err != nil {
		return storeErr(err, "no icon")
	}
	c.Set(fiber.HeaderContentType, ct)
	c.Set(fiber.HeaderCacheControl, "private, max-age=86400")
	c.Set("X-Content-Type-Options", "nosniff")
	// svg can carry scripts, never run them
	c.Set(fiber.HeaderContentSecurityPolicy, "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	return c.Send(data)
}

// facets godoc
//
//	@Summary		Categories and runs on options with counts
//	@Description	Every configured option in order, counting the apps the user can see.
//	@Tags			apps
//	@Produce		json
//	@Success		200	{object}	Facets
//	@Failure		401	{object}	Error
//	@Router			/facets [get]
func (h *handler) facets(c fiber.Ctx) error {
	v := auth.ViewerFrom(c)
	cats, err := h.st.Options(c.Context(), v, store.KindCategory)
	if err != nil {
		return err
	}
	runsOn, err := h.st.Options(c.Context(), v, store.KindRunsOn)
	if err != nil {
		return err
	}
	return c.JSON(Facets{Categories: cats, RunsOn: runsOn})
}

// addFavorite godoc
//
//	@Summary	Add an app to my favorites
//	@Tags		apps
//	@Param		id	path	string	true	"app id"
//	@Success	204
//	@Failure	404	{object}	Error
//	@Router		/favorites/{id} [put]
func (h *handler) addFavorite(c fiber.Ctx) error {
	if err := h.st.SetFavorite(c.Context(), auth.ViewerFrom(c), param(c, "id"), true); err != nil {
		return storeErr(err, "app not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// removeFavorite godoc
//
//	@Summary	Remove an app from my favorites
//	@Tags		apps
//	@Param		id	path	string	true	"app id"
//	@Success	204
//	@Failure	404	{object}	Error
//	@Router		/favorites/{id} [delete]
func (h *handler) removeFavorite(c fiber.Ctx) error {
	if err := h.st.SetFavorite(c.Context(), auth.ViewerFrom(c), param(c, "id"), false); err != nil {
		return storeErr(err, "app not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// createApp godoc
//
//	@Summary	Add an app
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		app	body		store.AppInput	true	"the app"
//	@Success	201	{object}	store.App
//	@Failure	400	{object}	Invalid
//	@Failure	403	{object}	Error
//	@Failure	409	{object}	Error
//	@Router		/admin/apps [post]
func (h *handler) createApp(c fiber.Ctx) error {
	var in store.AppInput
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	if bad, err := h.checkApp(c, &in, true); bad != nil || err != nil {
		return err
	}
	app, err := h.st.CreateApp(c.Context(), auth.ActorFrom(c), in)
	if errors.Is(err, store.ErrConflict) {
		return c.Status(fiber.StatusConflict).JSON(Invalid{
			Error: "An app with this ID already exists", Fields: map[string]string{"id": "This ID is already used"},
		})
	}
	if err != nil {
		return storeErr(err, "app not found")
	}
	app.CanEdit = true
	return c.Status(fiber.StatusCreated).JSON(app)
}

// updateApp godoc
//
//	@Summary		Edit an app
//	@Description	The id can't change, the one in the body is ignored.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string			true	"app id"
//	@Param			app	body		store.AppInput	true	"the app"
//	@Success		200	{object}	store.App
//	@Failure		400	{object}	Invalid
//	@Failure		403	{object}	Error
//	@Failure		404	{object}	Error
//	@Router			/admin/apps/{id} [put]
func (h *handler) updateApp(c fiber.Ctx) error {
	var in store.AppInput
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	in.ID = param(c, "id")
	if bad, err := h.checkApp(c, &in, false); bad != nil || err != nil {
		return err
	}
	app, err := h.st.UpdateApp(c.Context(), auth.ActorFrom(c), in.ID, in)
	if err != nil {
		return storeErr(err, "app not found")
	}
	app.CanEdit = true
	return c.JSON(app)
}

// deleteApp godoc
//
//	@Summary	Delete an app
//	@Tags		admin
//	@Param		id	path	string	true	"app id"
//	@Success	204
//	@Failure	403	{object}	Error
//	@Failure	404	{object}	Error
//	@Router		/admin/apps/{id} [delete]
func (h *handler) deleteApp(c fiber.Ctx) error {
	if err := h.st.DeleteApp(c.Context(), auth.ActorFrom(c), param(c, "id")); err != nil {
		return storeErr(err, "app not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

const (
	maxIconBytes = 256 << 10
	minIconPx    = 128
)

// uploadIcon godoc
//
//	@Summary	Upload an app icon
//	@Tags		apps
//	@Accept		multipart/form-data
//	@Produce	json
//	@Param		id		path		string	true	"app id"
//	@Param		icon	formData	file	true	"png or svg, up to 256 KB, png at least 128 x 128"
//	@Success	200		{object}	store.App
//	@Failure	400		{object}	Error
//	@Failure	403		{object}	Error
//	@Failure	404		{object}	Error
//	@Router		/apps/{id}/icon [put]
func (h *handler) uploadIcon(c fiber.Ctx) error {
	if err := h.requireEdit(c); err != nil {
		return err
	}
	fh, err := c.FormFile("icon")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "send the icon as the multipart field icon")
	}
	if fh.Size > maxIconBytes {
		return fiber.NewError(fiber.StatusBadRequest, "the icon is over 256 KB")
	}
	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxIconBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxIconBytes {
		return fiber.NewError(fiber.StatusBadRequest, "the icon is over 256 KB")
	}
	ct, msg := iconType(data)
	if msg != "" {
		return fiber.NewError(fiber.StatusBadRequest, msg)
	}
	app, err := h.st.SetIcon(c.Context(), auth.ActorFrom(c), param(c, "id"), ct, data)
	if err != nil {
		return storeErr(err, "app not found")
	}
	app.CanEdit = true
	return c.JSON(app)
}

// iconType returns the content type of a png or svg icon, or a message
// saying what is wrong with it.
func iconType(data []byte) (string, string) {
	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return "", "the png can't be read"
		}
		if cfg.Width < minIconPx || cfg.Height < minIconPx {
			return "", "the icon must be at least 128 x 128 px"
		}
		return "image/png", ""
	}
	head := bytes.TrimSpace(data)
	if bytes.HasPrefix(head, []byte("<")) && bytes.Contains(data, []byte("<svg")) {
		return "image/svg+xml", ""
	}
	return "", "use a png or svg file"
}

// deleteIcon godoc
//
//	@Summary	Remove the uploaded icon, back to the default icon
//	@Tags		apps
//	@Produce	json
//	@Param		id	path		string	true	"app id"
//	@Success	200	{object}	store.App
//	@Failure	403	{object}	Error
//	@Failure	404	{object}	Error
//	@Router		/apps/{id}/icon [delete]
func (h *handler) deleteIcon(c fiber.Ctx) error {
	if err := h.requireEdit(c); err != nil {
		return err
	}
	app, err := h.st.DeleteIcon(c.Context(), auth.ActorFrom(c), param(c, "id"))
	if err != nil {
		return storeErr(err, "app not found")
	}
	app.CanEdit = true
	return c.JSON(app)
}

// checkApp cleans up in and writes a 400 when it is invalid. bad is
// non-nil when the response was sent.
func (h *handler) checkApp(c fiber.Ctx, in *store.AppInput, isNew bool) (bad *Invalid, err error) {
	st, err := h.st.Settings(c.Context())
	if err != nil {
		return nil, err
	}
	fields := store.ValidateApp(st, in, isNew)
	if in.Owner != "" && fields["owner"] == "" {
		if _, err := h.st.Group(c.Context(), in.Owner); errors.Is(err, store.ErrNotFound) {
			if fields == nil {
				fields = map[string]string{}
			}
			fields["owner"] = "Pick a team from the list"
		} else if err != nil {
			return nil, err
		}
	}
	if fields == nil {
		return nil, nil
	}
	bad = &Invalid{Error: "Some fields need a fix", Fields: fields}
	return bad, c.Status(fiber.StatusBadRequest).JSON(bad)
}

func dedupe(items []string) []string {
	out := []string{}
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}
