package api

import (
	"errors"

	"github.com/gofiber/fiber/v3"

	"apps-catalog/internal/auth"
	"apps-catalog/internal/store"
)

// editableApp returns the app when the viewer may edit it. Apps the
// viewer can't see are a plain 404.
func (h *handler) editableApp(c fiber.Ctx) (store.App, error) {
	app, err := h.st.Get(c.Context(), auth.ViewerFrom(c), param(c, "id"))
	var na *store.NoAccessError
	if errors.As(err, &na) || errors.Is(err, store.ErrNotFound) {
		return store.App{}, fiber.NewError(fiber.StatusNotFound, "app not found")
	}
	if err != nil {
		return store.App{}, err
	}
	if !app.CanEdit {
		return store.App{}, fiber.NewError(fiber.StatusForbidden, "only the owner team and admins can edit this app")
	}
	return app, nil
}

func (h *handler) requireEdit(c fiber.Ctx) error {
	_, err := h.editableApp(c)
	return err
}

// updateDetails godoc
//
//	@Summary		Edit the details of an app
//	@Description	For admins and members of the owner team. Name, owner, category, runs on, sharing and start here stay admin only.
//	@Tags			apps
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"app id"
//	@Param			details	body		store.DetailsInput	true	"the details"
//	@Success		200		{object}	store.App
//	@Failure		400		{object}	Invalid
//	@Failure		403		{object}	Error
//	@Failure		404		{object}	Error
//	@Router			/apps/{id}/details [put]
func (h *handler) updateDetails(c fiber.Ctx) error {
	cur, err := h.editableApp(c)
	if err != nil {
		return err
	}
	var in store.DetailsInput
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	st, err := h.st.Settings(c.Context())
	if err != nil {
		return err
	}
	ai := store.AppInput{
		ID: cur.ID, Name: cur.Name, Owner: cur.Owner, Category: cur.Category, RunsOn: cur.RunsOn,
		StartHere: cur.StartHere, Visibility: cur.Visibility, Groups: cur.Groups,
	}
	in.Apply(&ai)
	if fields := store.ValidateApp(st, &ai, false); fields != nil {
		return c.Status(fiber.StatusBadRequest).JSON(Invalid{Error: "Some fields need a fix", Fields: fields})
	}
	// save the cleaned up values
	in = store.DetailsInput{
		Description: ai.Description, Contact: ai.Contact, Version: ai.Version, Access: ai.Access, HealthURL: ai.HealthURL,
		URLs: ai.URLs, Links: ai.Links, IconColor: ai.IconColor, Icon: ai.Icon,
	}
	app, err := h.st.UpdateDetails(c.Context(), auth.ActorFrom(c), cur.ID, in)
	if err != nil {
		return storeErr(err, "app not found")
	}
	app.CanEdit = true
	return c.JSON(app)
}

// getTeam godoc
//
//	@Summary		A team and the apps it owns
//	@Description	Member names and the owned apps the user can see.
//	@Tags			apps
//	@Produce		json
//	@Param			name	path		string	true	"team name"
//	@Success		200		{object}	store.Team
//	@Failure		401		{object}	Error
//	@Failure		404		{object}	Error
//	@Router			/teams/{name} [get]
func (h *handler) getTeam(c fiber.Ctx) error {
	t, err := h.st.Team(c.Context(), auth.ViewerFrom(c), param(c, "name"))
	if err != nil {
		return storeErr(err, "team not found")
	}
	return c.JSON(t)
}
