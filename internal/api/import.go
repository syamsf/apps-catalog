package api

import (
	"errors"

	"github.com/gofiber/fiber/v3"

	"apps-catalog/internal/auth"
	"apps-catalog/internal/store"
)

const maxImportBytes = 1 << 20

type ImportInput struct {
	// the file, see docs/import.md for the format
	YAML string `json:"yaml"`
	// skip or update apps that already exist
	Existing string `json:"existing" enums:"skip,update"`
}

// ImportFailed is a 400 when the file has problems.
type ImportFailed struct {
	Error string           `json:"error"`
	Plan  store.ImportPlan `json:"plan"`
}

func readImport(c fiber.Ctx) (ImportInput, error) {
	var in ImportInput
	if err := bindJSON(c, &in); err != nil {
		return in, err
	}
	if len(in.YAML) > maxImportBytes {
		return in, fiber.NewError(fiber.StatusRequestEntityTooLarge, "the file is over 1 MB")
	}
	return in, nil
}

// previewImport godoc
//
//	@Summary		Check a YAML import file
//	@Description	Lists the apps, what already exists, what would be created and every problem. Saves nothing.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			file	body		ImportInput	true	"the file"
//	@Success		200		{object}	store.ImportPlan
//	@Failure		400		{object}	Error
//	@Failure		403		{object}	Error
//	@Router			/admin/import/preview [post]
func (h *handler) previewImport(c fiber.Ctx) error {
	in, err := readImport(c)
	if err != nil {
		return err
	}
	plan, err := h.st.PlanImport(c.Context(), []byte(in.YAML))
	if err != nil {
		return err
	}
	return c.JSON(plan)
}

// runImport godoc
//
//	@Summary		Import apps from a YAML file
//	@Description	All or nothing: a file with any problem imports nothing and returns the plan.
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			file	body		ImportInput	true	"the file"
//	@Success		200		{object}	store.ImportResult
//	@Failure		400		{object}	ImportFailed
//	@Failure		403		{object}	Error
//	@Router			/admin/import [post]
func (h *handler) runImport(c fiber.Ctx) error {
	in, err := readImport(c)
	if err != nil {
		return err
	}
	if in.Existing != "skip" && in.Existing != "update" {
		return fiber.NewError(fiber.StatusBadRequest, "existing must be skip or update")
	}
	res, plan, err := h.st.ApplyImport(c.Context(), auth.ActorFrom(c), []byte(in.YAML), in.Existing == "update")
	if errors.Is(err, store.ErrImportInvalid) {
		return c.Status(fiber.StatusBadRequest).JSON(ImportFailed{Error: "The file has problems, nothing was imported", Plan: plan})
	}
	if err != nil {
		return err
	}
	return c.JSON(res)
}
