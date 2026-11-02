// Package ui serves the embedded Svelte single-page application.
package ui

import (
	"embed"
	"io/fs"
	"path"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/static"
)

//go:embed all:dist
var embedded embed.FS

//go:embed fallback.html
var fallbackIndex []byte

// Dist returns the embedded build output rooted at dist.
func Dist() fs.FS {
	dist, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic(err)
	}

	return dist
}

// RegisterUI serves the SPA on every unmatched GET route.  Unknown API paths
// and missing assets keep their 404 instead of falling through to the index
// document.  When no build output is embedded, a build-instructions page is
// served instead.
func RegisterUI(app *fiber.App) {
	dist := Dist()

	if _, err := fs.Stat(dist, "index.html"); err != nil {
		app.Get("/*", func(c fiber.Ctx) error {
			if isAPIPath(c.Path()) {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
			}

			c.Type("html")

			return c.Send(fallbackIndex)
		})

		return
	}

	app.Get("/*", static.New("", static.Config{
		FS: dist,
		NotFoundHandler: func(c fiber.Ctx) error {
			if isAPIPath(c.Path()) || path.Ext(c.Path()) != "" {
				return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
			}

			c.Status(fiber.StatusOK)

			return c.SendFile("index.html", fiber.SendFile{FS: dist})
		},
	}))
}

func isAPIPath(path string) bool {
	for _, prefix := range []string{"/api", "/healthz", "/readyz"} {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}

	return false
}
