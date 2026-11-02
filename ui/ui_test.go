package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newUIApp(t *testing.T) *fiber.App {
	t.Helper()

	app := fiber.New()
	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	app.Get("/api/v1/files/", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"files": []any{}})
	})
	RegisterUI(app)

	return app
}

// requireBundle skips SPA assertions when the frontend has not been built.
// Run `task ui:build` before `go test` to exercise the embedded shell.
func requireBundle(t *testing.T) {
	t.Helper()

	if _, err := Dist().Open("index.html"); err != nil {
		t.Skip("ui/dist not built; run task ui:build")
	}
}

func get(t *testing.T, app *fiber.App, path string) (int, []byte, string) {
	t.Helper()

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
	require.NoError(t, err, "GET %s", path)
	defer func() {
		require.NoError(t, resp.Body.Close(), "close %s body", path)
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read %s body", path)

	return resp.StatusCode, body, resp.Header.Get(fiber.HeaderContentType)
}

func TestIndexServesAppShell(t *testing.T) {
	requireBundle(t)

	code, body, contentType := get(t, newUIApp(t), "/")

	require.Equal(t, http.StatusOK, code, "body %q", body)
	assert.Contains(t, contentType, "text/html")
	assert.Contains(t, string(body), `id="app"`)
}

func TestUnknownRouteFallsBackToIndex(t *testing.T) {
	requireBundle(t)

	code, body, contentType := get(t, newUIApp(t), "/files/12/detections")

	require.Equal(t, http.StatusOK, code, "body %q", body)
	assert.Contains(t, contentType, "text/html")
	assert.Contains(t, string(body), `id="app"`)
}

func TestKnownAPIRouteStillWins(t *testing.T) {
	code, body, _ := get(t, newUIApp(t), "/api/v1/files/")

	require.Equal(t, http.StatusOK, code, "body %q", body)
	assert.JSONEq(t, `{"files":[]}`, string(body))
}

func TestUnknownAPIRouteStays404(t *testing.T) {
	code, body, _ := get(t, newUIApp(t), "/api/v1/nope")

	require.Equal(t, http.StatusNotFound, code, "body %q", body)
	assert.JSONEq(t, `{"error":"not found"}`, string(body))
}

func TestMissingAssetStays404(t *testing.T) {
	requireBundle(t)

	code, body, _ := get(t, newUIApp(t), "/assets/missing.js")

	require.Equal(t, http.StatusNotFound, code, "body %q", body)
	assert.JSONEq(t, `{"error":"not found"}`, string(body))
}

func TestMissingBundleServesBuildInstructions(t *testing.T) {
	if _, err := Dist().Open("index.html"); err == nil {
		t.Skip("ui/dist is built")
	}

	code, body, contentType := get(t, newUIApp(t), "/")

	require.Equal(t, http.StatusOK, code, "body %q", body)
	assert.Contains(t, contentType, "text/html")
	assert.Contains(t, string(body), "UI bundle missing")
}
