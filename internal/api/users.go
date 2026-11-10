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

type UserInput struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
}

type PasswordInput struct {
	Password string `json:"password"`
}

type PasswordChange struct {
	Current  string `json:"current"`
	Password string `json:"password"`
}

func checkProfile(name, email *string, fields map[string]string) {
	*name = strings.TrimSpace(*name)
	*email = strings.TrimSpace(*email)
	if *email != "" {
		if _, err := mail.ParseAddress(*email); err != nil {
			fields["email"] = "Not a valid email"
		}
	}
}

func passwordError(err error) string {
	switch {
	case errors.Is(err, auth.ErrPasswordShort):
		return "Use at least 8 characters"
	case errors.Is(err, auth.ErrPasswordLong):
		return "Use at most 72 bytes"
	}
	return ""
}

func invalid(c fiber.Ctx, fields map[string]string) error {
	return c.Status(fiber.StatusBadRequest).JSON(Invalid{Error: "Some fields need a fix", Fields: fields})
}

// createUser godoc
//
//	@Summary		Add a local user
//	@Description	Local users sign in with a username and password instead of sso.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			user	body		store.LocalUserInput	true	"the user"
//	@Success		201		{object}	store.User
//	@Failure		400		{object}	Invalid
//	@Failure		403		{object}	Error
//	@Failure		409		{object}	Invalid
//	@Router			/admin/users [post]
func (h *handler) createUser(c fiber.Ctx) error {
	var in store.LocalUserInput
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	fields := map[string]string{}
	in.Username = strings.TrimSpace(in.Username)
	if !catalog.ValidID(strings.ToLower(in.Username)) {
		fields["username"] = "Use letters, numbers and single dashes"
	}
	checkProfile(&in.Name, &in.Email, fields)
	in.Groups = dedupe(in.Groups)
	hash, err := auth.HashPassword(in.Password)
	if msg := passwordError(err); msg != "" {
		fields["password"] = msg
	} else if err != nil {
		return err
	}
	if len(fields) > 0 {
		return invalid(c, fields)
	}
	u, err := h.st.CreateLocalUser(c.Context(), auth.ActorFrom(c), in, hash)
	if errors.Is(err, store.ErrConflict) {
		return c.Status(fiber.StatusConflict).JSON(Invalid{
			Error: "A user with this username already exists", Fields: map[string]string{"username": "This username is taken"},
		})
	}
	if err != nil {
		return storeErr(err, "user not found")
	}
	return c.Status(fiber.StatusCreated).JSON(u)
}

// updateUser godoc
//
//	@Summary		Edit a local user
//	@Description	Name, email and admin role. Sso users get these from sso.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			subject	path		string		true	"user subject"
//	@Param			user	body		UserInput	true	"the user"
//	@Success		200		{object}	store.User
//	@Failure		400		{object}	Invalid
//	@Failure		403		{object}	Error
//	@Failure		404		{object}	Error
//	@Router			/admin/users/{subject} [put]
func (h *handler) updateUser(c fiber.Ctx) error {
	var in UserInput
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	subject := param(c, "subject")
	fields := map[string]string{}
	checkProfile(&in.Name, &in.Email, fields)
	if subject == auth.UserFrom(c).Subject && !in.IsAdmin {
		fields["is_admin"] = "You can't remove your own admin role"
	}
	if len(fields) > 0 {
		return invalid(c, fields)
	}
	u, err := h.st.UpdateLocalUser(c.Context(), auth.ActorFrom(c), subject, in.Name, in.Email, in.IsAdmin)
	if err != nil {
		return storeErr(err, "user not found")
	}
	return c.JSON(u)
}

// setUserPassword godoc
//
//	@Summary	Set a new password for a local user
//	@Tags		admin
//	@Accept		json
//	@Param		subject		path	string			true	"user subject"
//	@Param		password	body	PasswordInput	true	"new password"
//	@Success	204
//	@Failure	400	{object}	Invalid
//	@Failure	403	{object}	Error
//	@Failure	404	{object}	Error
//	@Router		/admin/users/{subject}/password [put]
func (h *handler) setUserPassword(c fiber.Ctx) error {
	var in PasswordInput
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	hash, err := auth.HashPassword(in.Password)
	if msg := passwordError(err); msg != "" {
		return invalid(c, map[string]string{"password": msg})
	} else if err != nil {
		return err
	}
	by := auth.ActorFrom(c)
	if err := h.st.SetPassword(c.Context(), &by, param(c, "subject"), hash); err != nil {
		return storeErr(err, "user not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// deleteUser godoc
//
//	@Summary		Remove a user
//	@Description	Also signs them out. An sso user comes back on their next sign-in, without groups.
//	@Tags			admin
//	@Param			subject	path	string	true	"user subject"
//	@Success		204
//	@Failure		400	{object}	Error
//	@Failure		403	{object}	Error
//	@Failure		404	{object}	Error
//	@Router			/admin/users/{subject} [delete]
func (h *handler) deleteUser(c fiber.Ctx) error {
	subject := param(c, "subject")
	if subject == auth.UserFrom(c).Subject {
		return fiber.NewError(fiber.StatusBadRequest, "You can't remove yourself")
	}
	if err := h.st.DeleteUser(c.Context(), auth.ActorFrom(c), subject); err != nil {
		return storeErr(err, "user not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// changePassword godoc
//
//	@Summary		Change my password
//	@Description	Only for local users.
//	@Tags			portal
//	@Accept			json
//	@Param			password	body	PasswordChange	true	"current and new password"
//	@Success		204
//	@Failure		400	{object}	Invalid
//	@Failure		401	{object}	Error
//	@Router			/me/password [put]
func (h *handler) changePassword(c fiber.Ctx) error {
	var in PasswordChange
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	u := auth.UserFrom(c)
	if u.Source != store.SourceLocal {
		return fiber.NewError(fiber.StatusBadRequest, "You sign in with sso, change your password there")
	}
	hash, err := h.st.PasswordHash(c.Context(), u.Subject)
	if err != nil {
		return storeErr(err, "user not found")
	}
	if !auth.CheckPassword(hash, in.Current) {
		return invalid(c, map[string]string{"current": "This is not your current password"})
	}
	newHash, err := auth.HashPassword(in.Password)
	if msg := passwordError(err); msg != "" {
		return invalid(c, map[string]string{"password": msg})
	} else if err != nil {
		return err
	}
	if err := h.st.SetPassword(c.Context(), nil, u.Subject, newHash); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
