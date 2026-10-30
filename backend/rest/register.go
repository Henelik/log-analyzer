package rest

import (
	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"
)

func RegisterAPI(app *fiber.App, db *gorm.DB) {
	app.Get("/healthz", Healthz)
	app.Get("/readyz", Readyz(db))

	api := app.Group("/api/v1")

	files := api.Group("/files")
	files.Post("/", Unimplemented)
	files.Get("/", Unimplemented)
	files.Get("/{id}", Unimplemented)
	files.Delete("/{id}", Unimplemented)
	files.Get("/{id}/analysis", Unimplemented)
	files.Get("/{id}/errors", Unimplemented)
	files.Get("/{id}/events", Unimplemented)

	users := api.Group("/users")
	users.Post("/login", Unimplemented)
}

func Unimplemented(ctx fiber.Ctx) error {
	return ctx.SendString("Unimplemented")
}
