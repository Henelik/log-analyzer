package rest

import (
	"net/http"
	"testing"

	"github.com/Henelik/log-analyzer/db"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newDetectionsApp(t *testing.T) (*fiber.App, *gorm.DB) {
	t.Helper()

	database := openTestDB(t)
	require.NoError(t, database.AutoMigrate(&db.File{}, &db.Detection{}), "migrate detections test database")

	return newApp(t, database), database
}

func seedDetections(t *testing.T, database *gorm.DB, fileID uint, detections []db.Detection) {
	t.Helper()

	for i := range detections {
		detections[i].IngestID = fileID
	}

	if len(detections) == 0 {
		return
	}

	require.NoError(t, database.CreateInBatches(&detections, 100).Error, "seed detections")
}

func detectionsPath(id uint) string {
	return filePath(id) + "/detections"
}

func TestGetDetectionsReturnsSeededDetections(t *testing.T) {
	app, database := newDetectionsApp(t)

	file := seedFiles(t, database)[0]

	seedDetections(t, database, file.ID, []db.Detection{
		{
			DetectionID: 1,
			Description: "Brute Force Attempt",
			Severity:    100,
			Confidence:  51,
			Priority:    80,
			Entity:      "alice",
			Observed:    12,
			Threshold:   10,
			Events:      db.Int32Array{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13},
			Notes:       "twelve failures",
		},
		{
			DetectionID: 2,
			Description: "Port Scan",
			Severity:    60,
			Confidence:  40,
			Priority:    50,
			Entity:      "10.0.0.1",
			Observed:    25,
			Threshold:   20,
			Events:      db.Int32Array{20, 21},
		},
	})

	code, body := doRequest(t, app, http.MethodGet, detectionsPath(file.ID))
	require.Equal(t, http.StatusOK, code, "body %q", body)

	payload := decodeJSON[detectionsResponse](t, body)
	require.Len(t, payload.Detections, 2)

	assert.Equal(t, detectionResponse{
		DetectionID: 1,
		Description: "Brute Force Attempt",
		Severity:    100,
		Confidence:  51,
		Priority:    80,
		Entity:      "alice",
		Observed:    12,
		Threshold:   10,
		Events:      []int32{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13},
		Notes:       "twelve failures",
	}, payload.Detections[0])

	assert.Equal(t, detectionResponse{
		DetectionID: 2,
		Description: "Port Scan",
		Severity:    60,
		Confidence:  40,
		Priority:    50,
		Entity:      "10.0.0.1",
		Observed:    25,
		Threshold:   20,
		Events:      []int32{20, 21},
	}, payload.Detections[1])
}

func TestGetDetectionsOrdersByDetectionID(t *testing.T) {
	app, database := newDetectionsApp(t)

	file := seedFiles(t, database)[0]

	seedDetections(t, database, file.ID, []db.Detection{
		{DetectionID: 3, Description: "third", Events: db.Int32Array{}},
		{DetectionID: 1, Description: "first", Events: db.Int32Array{}},
		{DetectionID: 2, Description: "second", Events: db.Int32Array{}},
	})

	code, body := doRequest(t, app, http.MethodGet, detectionsPath(file.ID))
	require.Equal(t, http.StatusOK, code, "body %q", body)

	payload := decodeJSON[detectionsResponse](t, body)
	require.Len(t, payload.Detections, 3)

	ids := make([]uint, 0, len(payload.Detections))
	for _, detection := range payload.Detections {
		ids = append(ids, detection.DetectionID)
	}

	assert.Equal(t, []uint{1, 2, 3}, ids)
}

func TestGetDetectionsEmpty(t *testing.T) {
	app, database := newDetectionsApp(t)

	file := seedFiles(t, database)[0]

	code, body := doRequest(t, app, http.MethodGet, detectionsPath(file.ID))
	require.Equal(t, http.StatusOK, code, "body %q", body)

	assert.JSONEq(t, `{"detections":[]}`, string(body))
}

func TestGetDetectionsScopesToFile(t *testing.T) {
	app, database := newDetectionsApp(t)

	files := seedFiles(t, database)

	seedDetections(t, database, files[0].ID, []db.Detection{
		{DetectionID: 1, Description: "first", Events: db.Int32Array{2}},
	})
	seedDetections(t, database, files[1].ID, []db.Detection{
		{DetectionID: 1, Description: "second", Events: db.Int32Array{3}},
	})

	code, body := doRequest(t, app, http.MethodGet, detectionsPath(files[0].ID))
	require.Equal(t, http.StatusOK, code, "body %q", body)

	payload := decodeJSON[detectionsResponse](t, body)
	require.Len(t, payload.Detections, 1)
	assert.Equal(t, "first", payload.Detections[0].Description, "detections from other files are not returned")
}

func TestGetDetectionsInvalidFileID(t *testing.T) {
	app, _ := newDetectionsApp(t)

	for _, id := range []string{"abc", "1.5"} {
		t.Run(id, func(t *testing.T) {
			code, body := doRequest(t, app, http.MethodGet, "/api/v1/files/"+id+"/detections")

			assert.Equal(t, http.StatusBadRequest, code)
			requireUploadError(t, body, "invalid file ID")
		})
	}
}

func TestGetDetectionsFileNotFound(t *testing.T) {
	app, database := newDetectionsApp(t)

	seedFiles(t, database)

	code, body := doRequest(t, app, http.MethodGet, "/api/v1/files/999/detections")

	assert.Equal(t, http.StatusNotFound, code)
	requireUploadError(t, body, "file not found")
}
