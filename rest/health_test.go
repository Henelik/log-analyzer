package rest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Henelik/log-analyzer/db"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newApp(t *testing.T, db *gorm.DB) *fiber.App {
	t.Helper()

	app := fiber.New()
	RegisterAPI(app, db)

	return app
}

func probe(t *testing.T, app *fiber.App, path string) (int, healthResponse) {
	t.Helper()

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
	require.NoError(t, err, "request %s", path)
	defer func() {
		require.NoError(t, resp.Body.Close(), "close %s body", path)
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read %s body", path)

	var payload healthResponse
	require.NoError(t, json.Unmarshal(body, &payload), "decode %s body %q", path, body)

	return resp.StatusCode, payload
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	database, err := db.NewTestDB()
	require.NoError(t, err, "open test database")

	return database
}

func TestHealthz(t *testing.T) {
	code, payload := probe(t, newApp(t, nil), "/healthz")

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok", payload.Status)
}

func TestReadyzWithoutDB(t *testing.T) {
	code, payload := probe(t, newApp(t, nil), "/readyz")

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok", payload.Status)
}

func TestReadyzHealthyDB(t *testing.T) {
	db := openTestDB(t)

	code, payload := probe(t, newApp(t, db), "/readyz")

	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok", payload.Status)
}

func TestReadyzUnreachableDB(t *testing.T) {
	db := openTestDB(t)

	sqlDB, err := db.DB()
	require.NoError(t, err, "unwrap sql.DB")
	require.NoError(t, sqlDB.Close(), "close sql.DB")

	code, payload := probe(t, newApp(t, db), "/readyz")

	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "unavailable", payload.Status)
}
