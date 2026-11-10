package api

import (
	"errors"

	"net/mail"
	"strings"

	"github.com/gofiber/fiber/v3"

	"apps-catalog/internal/auth"
	"apps-catalog/internal/catalog"
	"apps-catalog/internal/store"
)

type GroupList struct {
	Groups []store.Group `json:"groups"`
}

type UserList struct {
	Users []store.User `json:"users"`
}

type UserGroups struct {
	Groups []string `json:"groups"`
}

type OptionInput struct {
	Label string `json:"label"`
}

type OptionOrder struct {
	Keys []string `json:"keys"`
}

// HealthCheckInput is the app form state, so a check works before saving.
type HealthCheckInput struct {
	HealthURL string            `json:"health_url"`
	URLs      map[string]string `json:"urls"`
}

// dashboard godoc
//
//	@Summary	Counts, things to fix and recent changes
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	store.Dashboard
//	@Failure	403	{object}	Error
//	@Router		/admin/dashboard [get]
func (h *handler) dashboard(c fiber.Ctx) error {
	d, err := h.st.Dashboard(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(d)
}

// listGroups godoc
//
//	@Summary	List groups with their members and apps
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	GroupList
//	@Failure	403	{object}	Error
//	@Router		/admin/groups [get]
func (h *handler) listGroups(c fiber.Ctx) error {
	groups, err := h.st.Groups(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(GroupList{Groups: groups})
}

// getGroup godoc
//
//	@Summary	Get one group
//	@Tags		admin
//	@Produce	json
//	@Param		name	path		string	true	"group name"
//	@Success	200		{object}	store.Group
//	@Failure	403		{object}	Error
//	@Failure	404		{object}	Error
//	@Router		/admin/groups/{name} [get]
func (h *handler) getGroup(c fiber.Ctx) error {
	g, err := h.st.Group(c.Context(), param(c, "name"))
	if err != nil {
		return storeErr(err, "group not found")
	}
	return c.JSON(g)
}

// createGroup godoc
//
//	@Summary	Add a group
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		group	body		store.GroupInput	true	"the group"
//	@Success	201		{object}	store.Group
//	@Failure	400		{object}	Invalid
//	@Failure	403		{object}	Error
//	@Failure	409		{object}	Invalid
//	@Router		/admin/groups [post]
func (h *handler) createGroup(c fiber.Ctx) error {
	var in store.GroupInput
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	if bad := checkGroup(&in); bad != nil {
		return c.Status(fiber.StatusBadRequest).JSON(bad)
	}
	g, err := h.st.CreateGroup(c.Context(), auth.ActorFrom(c), in)
	if err != nil {
		return groupErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(g)
}

// updateGroup godoc
//
//	@Summary		Edit a group
//	@Description	Replaces the members and apps. A new name renames the group.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			name	path		string				true	"group name"
//	@Param			group	body		store.GroupInput	true	"the group"
//	@Success		200		{object}	store.Group
//	@Failure		400		{object}	Invalid
//	@Failure		403		{object}	Error
//	@Failure		404		{object}	Error
//	@Failure		409		{object}	Invalid
//	@Router			/admin/groups/{name} [put]
func (h *handler) updateGroup(c fiber.Ctx) error {
	var in store.GroupInput
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	if bad := checkGroup(&in); bad != nil {
		return c.Status(fiber.StatusBadRequest).JSON(bad)
	}
	g, err := h.st.UpdateGroup(c.Context(), auth.ActorFrom(c), param(c, "name"), in)
	if err != nil {
		return groupErr(c, err)
	}
	return c.JSON(g)
}

func checkGroup(in *store.GroupInput) *Invalid {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.Members = dedupe(in.Members)
	in.Apps = dedupe(in.Apps)
	if !catalog.ValidID(in.Name) {
		return &Invalid{Error: "Some fields need a fix", Fields: map[string]string{
			"name": "Use lowercase letters, numbers and single dashes",
		}}
	}
	return nil
}

func groupErr(c fiber.Ctx, err error) error {
	if err == store.ErrConflict {
		return c.Status(fiber.StatusConflict).JSON(Invalid{
			Error: "A group with this name already exists", Fields: map[string]string{"name": "This name is already used"},
		})
	}
	return storeErr(err, "group not found")
}

// deleteGroup godoc
//
//	@Summary		Delete a group
//	@Description	Members lose the apps they only had through this group.
//	@Tags			admin
//	@Param			name	path	string	true	"group name"
//	@Success		204
//	@Failure		403	{object}	Error
//	@Failure		404	{object}	Error
//	@Router			/admin/groups/{name} [delete]
func (h *handler) deleteGroup(c fiber.Ctx) error {
	if err := h.st.DeleteGroup(c.Context(), auth.ActorFrom(c), param(c, "name")); err != nil {
		return storeErr(err, "group not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// listUsers godoc
//
//	@Summary	List everyone who has signed in
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	UserList
//	@Failure	403	{object}	Error
//	@Router		/admin/users [get]
func (h *handler) listUsers(c fiber.Ctx) error {
	users, err := h.st.Users(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(UserList{Users: users})
}

// setUserGroups godoc
//
//	@Summary	Set the groups of a person
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		subject	path		string		true	"user subject"
//	@Param		groups	body		UserGroups	true	"group names"
//	@Success	200		{object}	store.User
//	@Failure	400		{object}	Error
//	@Failure	403		{object}	Error
//	@Failure	404		{object}	Error
//	@Router		/admin/users/{subject}/groups [put]
func (h *handler) setUserGroups(c fiber.Ctx) error {
	var in UserGroups
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	u, err := h.st.SetUserGroups(c.Context(), auth.ActorFrom(c), param(c, "subject"), dedupe(in.Groups))
	if err != nil {
		return storeErr(err, "user not found")
	}
	return c.JSON(u)
}

// updateSettings godoc
//
//	@Summary	Change the portal settings
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		settings	body		store.SettingsInput	true	"settings"
//	@Success	200			{object}	store.Settings
//	@Failure	400			{object}	Invalid
//	@Failure	403			{object}	Error
//	@Router		/admin/settings [put]
func (h *handler) updateSettings(c fiber.Ctx) error {
	var in store.SettingsInput
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.AdminEmail = strings.TrimSpace(in.AdminEmail)
	fields := map[string]string{}
	if in.Name == "" {
		fields["name"] = "Portal name is required"
	}
	if in.AdminEmail != "" {
		if _, err := mail.ParseAddress(in.AdminEmail); err != nil {
			fields["admin_email"] = "Not a valid email"
		}
	}
	if in.IconColorMode != "auto" && in.IconColorMode != "accent" {
		fields["icon_color_mode"] = "Use auto or accent"
	}
	if in.NewAppVisibility != store.VisibilityEveryone && in.NewAppVisibility != store.VisibilityGroups {
		fields["new_app_visibility"] = "Use everyone or groups"
	}
	if len(fields) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(Invalid{Error: "Some fields need a fix", Fields: fields})
	}
	if err := h.st.UpdateSettings(c.Context(), auth.ActorFrom(c), in); err != nil {
		return err
	}
	return h.settings(c)
}

// addOption godoc
//
//	@Summary		Add a category, runs on option or environment
//	@Description	The key is made from the label, e.g. "Cloud Run" becomes cloud-run.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			kind	path		string		true	"category, runs_on or environment"
//	@Param			option	body		OptionInput	true	"label"
//	@Success		201		{object}	store.Settings
//	@Failure		400		{object}	Error
//	@Failure		403		{object}	Error
//	@Failure		409		{object}	Error
//	@Router			/admin/options/{kind} [post]
func (h *handler) addOption(c fiber.Ctx) error {
	kind := param(c, "kind")
	if !store.ValidKind(kind) {
		return fiber.NewError(fiber.StatusNotFound, "unknown option kind")
	}
	var in OptionInput
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	label := strings.TrimSpace(in.Label)
	key := store.OptionKey(label)
	if key == "" {
		return fiber.NewError(fiber.StatusBadRequest, "the name needs at least one letter or number")
	}
	if err := h.st.AddOption(c.Context(), auth.ActorFrom(c), kind, key, label); err != nil {
		if err == store.ErrConflict {
			return fiber.NewError(fiber.StatusConflict, "that option already exists")
		}
		return err
	}
	c.Status(fiber.StatusCreated)
	return h.settings(c)
}

// deleteOption godoc
//
//	@Summary		Remove a category, runs on option or environment
//	@Description	Options still used by apps can't be removed.
//	@Tags			admin
//	@Produce		json
//	@Param			kind	path		string	true	"category, runs_on or environment"
//	@Param			key		path		string	true	"option key"
//	@Success		200		{object}	store.Settings
//	@Failure		403		{object}	Error
//	@Failure		404		{object}	Error
//	@Failure		409		{object}	Error
//	@Router			/admin/options/{kind}/{key} [delete]
func (h *handler) deleteOption(c fiber.Ctx) error {
	kind := param(c, "kind")
	if !store.ValidKind(kind) {
		return fiber.NewError(fiber.StatusNotFound, "unknown option kind")
	}
	if err := h.st.DeleteOption(c.Context(), auth.ActorFrom(c), kind, param(c, "key")); err != nil {
		return storeErr(err, "option not found")
	}
	return h.settings(c)
}

// renameOption godoc
//
//	@Summary		Rename a category, runs on option or environment
//	@Description	Only the label changes, apps keep using the key.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			kind	path		string		true	"category, runs_on or environment"
//	@Param			key		path		string		true	"option key"
//	@Param			option	body		OptionInput	true	"new label"
//	@Success		200		{object}	store.Settings
//	@Failure		400		{object}	Error
//	@Failure		403		{object}	Error
//	@Failure		404		{object}	Error
//	@Router			/admin/options/{kind}/{key} [put]
func (h *handler) renameOption(c fiber.Ctx) error {
	kind := param(c, "kind")
	if !store.ValidKind(kind) {
		return fiber.NewError(fiber.StatusNotFound, "unknown option kind")
	}
	var in OptionInput
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		return fiber.NewError(fiber.StatusBadRequest, "the name can't be empty")
	}
	if err := h.st.RenameOption(c.Context(), auth.ActorFrom(c), kind, param(c, "key"), label); err != nil {
		return storeErr(err, "option not found")
	}
	return h.settings(c)
}

// reorderOptions godoc
//
//	@Summary		Reorder categories, runs on options or environments
//	@Description	Send every key of the kind in the new order. The environment order sets the order on app pages.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			kind	path		string		true	"category, runs_on or environment"
//	@Param			order	body		OptionOrder	true	"all keys, in order"
//	@Success		200		{object}	store.Settings
//	@Failure		403		{object}	Error
//	@Failure		404		{object}	Error
//	@Failure		409		{object}	Error
//	@Router			/admin/options/{kind} [put]
func (h *handler) reorderOptions(c fiber.Ctx) error {
	kind := param(c, "kind")
	if !store.ValidKind(kind) {
		return fiber.NewError(fiber.StatusNotFound, "unknown option kind")
	}
	var in OptionOrder
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	if err := h.st.ReorderOptions(c.Context(), auth.ActorFrom(c), kind, in.Keys); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return fiber.NewError(fiber.StatusConflict, "the list changed, reload the page")
		}
		return err
	}
	return h.settings(c)
}

// healthCheck godoc
//
//	@Summary		Check a health url now
//	@Description	Runs one check with the same rules as the background checker and returns the result. Nothing is saved.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			app	body		HealthCheckInput	true	"health url and app urls"
//	@Success		200	{object}	store.Status
//	@Failure		400	{object}	Error
//	@Failure		403	{object}	Error
//	@Failure		503	{object}	Error
//	@Router			/admin/health-check [post]
func (h *handler) healthCheck(c fiber.Ctx) error {
	if h.prober == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "health checks are off")
	}
	var in HealthCheckInput
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	target := catalog.App{HealthURL: strings.TrimSpace(in.HealthURL), URLs: in.URLs}.HealthTarget()
	if target == "" {
		return fiber.NewError(fiber.StatusBadRequest, "add a health check url or a production url first")
	}
	return c.JSON(h.prober.Check(c.Context(), target))
}
