package api

import (
	"strconv"

	"github.com/gofiber/fiber/v3"

	"apps-catalog/internal/auth"
	"apps-catalog/internal/store"
)

// AccessNote is the optional note sent with an access request.
type AccessNote struct {
	Note string `json:"note"`
}

// AccessApprove names the group that gets the person.
type AccessApprove struct {
	Group string `json:"group"`
}

// AccessRequestList is the admin queue, oldest first.
type AccessRequestList struct {
	Requests []store.AccessRequest `json:"requests"`
}

const maxNoteLen = 500

// requestAccess godoc
//
//	@Summary		Ask for access to an app
//	@Description	Only for an app the user can't see. A second call while one is pending returns the pending request.
//	@Tags			apps
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string		true	"app id"
//	@Param			body	body		AccessNote	false	"optional note"
//	@Success		201		{object}	store.AccessRequest
//	@Failure		400		{object}	Error
//	@Failure		404		{object}	Error
//	@Router			/apps/{id}/access-requests [post]
func (h *handler) requestAccess(c fiber.Ctx) error {
	var in AccessNote
	if len(c.Body()) > 0 {
		if err := bindJSON(c, &in); err != nil {
			return err
		}
	}
	note := cleanQuery(in.Note)
	if len([]rune(note)) > maxNoteLen {
		return fiber.NewError(fiber.StatusBadRequest, "the note is over 500 characters")
	}
	r, err := h.st.RequestAccess(c.Context(), auth.ViewerFrom(c), param(c, "id"), note)
	if err != nil {
		return storeErr(err, "app not found")
	}
	return c.Status(fiber.StatusCreated).JSON(r)
}

// listAccessRequests godoc
//
//	@Summary	Access requests
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	AccessRequestList
//	@Failure	403	{object}	Error
//	@Router		/admin/access-requests [get]
func (h *handler) listAccessRequests(c fiber.Ctx) error {
	rs, err := h.st.AccessRequests(c.Context())
	if err != nil {
		return err
	}
	if rs == nil {
		rs = []store.AccessRequest{}
	}
	return c.JSON(AccessRequestList{Requests: rs})
}

func requestID(c fiber.Ctx) (int64, error) {
	id, err := strconv.ParseInt(param(c, "id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fiber.NewError(fiber.StatusBadRequest, "invalid request id")
	}
	return id, nil
}

// approveAccess godoc
//
//	@Summary		Approve an access request
//	@Description	Adds the person to the group, which must already have the app shared with it.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			id		path		int				true	"request id"
//	@Param			body	body		AccessApprove	true	"group to add the person to"
//	@Success		200		{object}	store.AccessRequest
//	@Failure		400		{object}	Error
//	@Failure		403		{object}	Error
//	@Failure		404		{object}	Error
//	@Failure		409		{object}	Error
//	@Router			/admin/access-requests/{id}/approve [post]
func (h *handler) approveAccess(c fiber.Ctx) error {
	id, err := requestID(c)
	if err != nil {
		return err
	}
	var in AccessApprove
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	r, err := h.st.ApproveAccess(c.Context(), auth.ActorFrom(c), id, in.Group)
	if err != nil {
		return storeErr(err, "request not found")
	}
	return c.JSON(r)
}

// declineAccess godoc
//
//	@Summary	Decline an access request
//	@Tags		admin
//	@Produce	json
//	@Param		id	path		int	true	"request id"
//	@Success	200	{object}	store.AccessRequest
//	@Failure	400	{object}	Error
//	@Failure	403	{object}	Error
//	@Failure	404	{object}	Error
//	@Failure	409	{object}	Error
//	@Router		/admin/access-requests/{id}/decline [post]
func (h *handler) declineAccess(c fiber.Ctx) error {
	id, err := requestID(c)
	if err != nil {
		return err
	}
	r, err := h.st.DeclineAccess(c.Context(), auth.ActorFrom(c), id)
	if err != nil {
		return storeErr(err, "request not found")
	}
	return c.JSON(r)
}
