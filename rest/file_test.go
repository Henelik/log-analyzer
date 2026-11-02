package rest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Henelik/log-analyzer/db"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newFilesApp(t *testing.T) (*fiber.App, *gorm.DB) {
	t.Helper()

	database := openTestDB(t)
	require.NoError(t, database.AutoMigrate(&db.File{}, &db.Event{}), "migrate files test database")

	return newApp(t, database), database
}

func seedFiles(t *testing.T, database *gorm.DB) []db.File {
	t.Helper()

	files := []db.File{
		{Filename: "first.csv", Headers: "time,login", Status: db.FileStatusUplodaded, Lines: 10, AcceptedLines: 9, RejectedLines: 1},
		{Filename: "second.csv", Headers: "time,url", Status: db.FileStatusAnalyzed, Lines: 5, AcceptedLines: 5},
	}

	for i := range files {
		require.NoError(t, database.Create(&files[i]).Error, "seed file %q", files[i].Filename)
	}

	return files
}

func doRequest(t *testing.T, app *fiber.App, method, path string) (int, []byte) {
	t.Helper()

	req := authorizeRequest(t, httptest.NewRequest(method, path, nil))

	resp, err := app.Test(req)
	require.NoError(t, err, "%s %s", method, path)
	defer func() {
		require.NoError(t, resp.Body.Close(), "close %s %s body", method, path)
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read %s %s body", method, path)

	return resp.StatusCode, body
}

func filePath(id uint) string {
	return uploadPath + strconv.FormatUint(uint64(id), 10)
}

func TestListFilesEmpty(t *testing.T) {
	app, _ := newFilesApp(t)

	code, body := doRequest(t, app, http.MethodGet, uploadPath)

	require.Equal(t, http.StatusOK, code, "body %q", body)
	assert.JSONEq(t, `{"files":[]}`, string(body))
}

func TestListFilesReturnsSeededFiles(t *testing.T) {
	app, database := newFilesApp(t)

	seeded := seedFiles(t, database)

	code, body := doRequest(t, app, http.MethodGet, uploadPath)
	require.Equal(t, http.StatusOK, code, "body %q", body)

	payload := decodeJSON[filesResponse](t, body)

	require.Len(t, payload.Files, len(seeded))

	for i, file := range seeded {
		assert.Equal(t, file.ID, payload.Files[i].ID)
		assert.Equal(t, file.Filename, payload.Files[i].Filename)
		assert.Equal(t, file.Status, payload.Files[i].Status)
	}
}

func TestGetFile(t *testing.T) {
	app, database := newFilesApp(t)

	seeded := seedFiles(t, database)
	file := seeded[0]

	code, body := doRequest(t, app, http.MethodGet, filePath(file.ID))
	require.Equal(t, http.StatusOK, code, "body %q", body)

	payload := decodeJSON[fileResponse](t, body)

	assert.Equal(t, file.ID, payload.ID)
	assert.Equal(t, file.Filename, payload.Filename)
	assert.Equal(t, file.Headers, payload.Headers)
	assert.Equal(t, file.Status, payload.Status)
	assert.Equal(t, file.Lines, payload.Lines)
	assert.Equal(t, file.AcceptedLines, payload.AcceptedLines)
	assert.Equal(t, file.RejectedLines, payload.RejectedLines)
	assert.Equal(t, file.DuplicateLines, payload.DuplicateLines)
	assert.False(t, payload.CreatedAt.IsZero(), "created at")
	assert.False(t, payload.UpdatedAt.IsZero(), "updated at")
}

func TestGetFileNotFound(t *testing.T) {
	app, _ := newFilesApp(t)

	code, body := doRequest(t, app, http.MethodGet, uploadPath+"999")

	assert.Equal(t, http.StatusNotFound, code)
	requireUploadError(t, body, "file not found")
}

func TestGetFileInvalidID(t *testing.T) {
	app, _ := newFilesApp(t)

	for _, id := range []string{"abc", "1.5"} {
		t.Run(id, func(t *testing.T) {
			code, body := doRequest(t, app, http.MethodGet, uploadPath+id)

			assert.Equal(t, http.StatusBadRequest, code)
			requireUploadError(t, body, "invalid file ID")
		})
	}
}

func TestDeleteFile(t *testing.T) {
	app, database := newFilesApp(t)

	seeded := seedFiles(t, database)
	file := seeded[0]

	code, body := doRequest(t, app, http.MethodDelete, filePath(file.ID))

	assert.Equal(t, http.StatusNoContent, code, "body %q", body)
	assert.Empty(t, body)

	var count int64
	require.NoError(t, database.Model(&db.File{}).Where("id = ?", file.ID).Count(&count).Error, "count file")
	assert.Zero(t, count)

	assert.Equal(t, int64(len(seeded)-1), countRows(t, database, &db.File{}))
}

func TestDeleteFileNotFound(t *testing.T) {
	app, database := newFilesApp(t)

	seeded := seedFiles(t, database)

	code, body := doRequest(t, app, http.MethodDelete, uploadPath+"999")

	assert.Equal(t, http.StatusNotFound, code)
	requireUploadError(t, body, "file not found")
	assert.Equal(t, int64(len(seeded)), countRows(t, database, &db.File{}))
}

func TestDeleteFileInvalidID(t *testing.T) {
	app, database := newFilesApp(t)

	seeded := seedFiles(t, database)

	code, body := doRequest(t, app, http.MethodDelete, uploadPath+"abc")

	assert.Equal(t, http.StatusBadRequest, code)
	requireUploadError(t, body, "invalid file ID")
	assert.Equal(t, int64(len(seeded)), countRows(t, database, &db.File{}))
}
