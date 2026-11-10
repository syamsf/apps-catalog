package api

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"apps-catalog/internal/icons"
)

// tablerIndex godoc
//
//	@Summary		Every Tabler icon, for the icon picker
//	@Description	{"version": "3.49.0", "icons": [{"n": name, "t": tags, "c": category, "b": svg body}]}. Sent gzipped when the client accepts it.
//	@Tags			icons
//	@Produce		json
//	@Success		200
//	@Failure		401	{object}	Error
//	@Router			/icons/tabler [get]
func (h *handler) tablerIndex(c fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSONCharsetUTF8)
	c.Set(fiber.HeaderCacheControl, "private, max-age=86400")
	c.Set(fiber.HeaderVary, fiber.HeaderAcceptEncoding)
	c.Set(fiber.HeaderETag, `"tabler-`+icons.Version()+`"`)
	if c.Get(fiber.HeaderIfNoneMatch) == `"tabler-`+icons.Version()+`"` {
		return c.SendStatus(fiber.StatusNotModified)
	}
	if strings.Contains(c.Get(fiber.HeaderAcceptEncoding), "gzip") {
		c.Set(fiber.HeaderContentEncoding, "gzip")
		return c.Send(icons.Index())
	}
	data, err := icons.IndexJSON()
	if err != nil {
		return err
	}
	return c.Send(data)
}

// tablerIcon godoc
//
//	@Summary	One Tabler icon as svg, drawn in currentColor
//	@Tags		icons
//	@Produce	image/svg+xml
//	@Param		name	path	string	true	"icon name, like brand-docker, .svg is optional"
//	@Success	200
//	@Failure	404	{object}	Error
//	@Router		/icons/tabler/{name} [get]
func (h *handler) tablerIcon(c fiber.Ctx) error {
	svg, ok := icons.SVG(strings.TrimSuffix(param(c, "name"), ".svg"))
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "no such icon")
	}
	c.Set(fiber.HeaderContentType, "image/svg+xml")
	c.Set(fiber.HeaderCacheControl, "private, max-age=604800")
	c.Set("X-Content-Type-Options", "nosniff")
	return c.Send(svg)
}
