package api

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"apps-catalog/internal/auth"
)

// SearchLog is one search the user ran.
type SearchLog struct {
	Q     string `json:"q"`
	Found bool   `json:"found"`
}

// SearchDismiss names a failed search to hide from the dashboard.
type SearchDismiss struct {
	Q string `json:"q"`
}

// cleanQuery drops NUL bytes (Postgres rejects them) and surrounding space.
func cleanQuery(q string) string {
	return strings.TrimSpace(strings.ReplaceAll(q, "\x00", ""))
}

// logSearch godoc
//
//	@Summary	Record a search
//	@Tags		apps
//	@Accept		json
//	@Param		body	body	SearchLog	true	"search text and whether it found anything"
//	@Success	204
//	@Failure	400	{object}	Error
//	@Router		/searches [post]
func (h *handler) logSearch(c fiber.Ctx) error {
	var in SearchLog
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	q := cleanQuery(in.Q)
	if q == "" {
		return fiber.NewError(fiber.StatusBadRequest, "q is required")
	}
	if err := h.st.LogSearch(c.Context(), auth.ViewerFrom(c).Subject, q, in.Found); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// logOpen godoc
//
//	@Summary	Record that the user opened an app
//	@Tags		apps
//	@Param		id	path	string	true	"app id"
//	@Success	204
//	@Failure	404	{object}	Error
//	@Router		/apps/{id}/open [post]
func (h *handler) logOpen(c fiber.Ctx) error {
	if err := h.st.LogOpen(c.Context(), auth.ViewerFrom(c), param(c, "id")); err != nil {
		return storeErr(err, "app not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// dismissSearch godoc
//
//	@Summary	Hide a failed search from the dashboard
//	@Tags		admin
//	@Accept		json
//	@Param		body	body	SearchDismiss	true	"search text"
//	@Success	204
//	@Failure	400	{object}	Error
//	@Failure	403	{object}	Error
//	@Router		/admin/searches/dismiss [post]
func (h *handler) dismissSearch(c fiber.Ctx) error {
	var in SearchDismiss
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	q := cleanQuery(in.Q)
	if q == "" {
		return fiber.NewError(fiber.StatusBadRequest, "q is required")
	}
	if err := h.st.DismissSearch(c.Context(), auth.ActorFrom(c), q); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
