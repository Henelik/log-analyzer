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
	files.Post("/", UploadFile(db))
	files.Get("/", ListFiles(db))
	files.Get("/:id", GetFile(db))
	files.Delete("/:id", DeleteFile(db))
	files.Get("/:id/detections", GetDetections(db))
	files.Get("/:id/errors", GetEventErrors(db))
	files.Post("/:id/events", GetEvents(db))

	api.Post("/login", Login(db))
}
