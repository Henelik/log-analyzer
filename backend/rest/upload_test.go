package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Henelik/log-analyzer/db"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	uploadPath       = "/api/v1/files/"
	fullUploadHeader = "time,login,cip,sip,url,reqmethod,respcode,action,appname,reqsize,respdatasize,riskscore,threatseverity"
)

func newUploadApp(t *testing.T) (*fiber.App, *gorm.DB) {
	t.Helper()

	database := openTestDB(t)
	require.NoError(t, database.AutoMigrate(&db.File{}, &db.Event{}), "migrate upload test database")

	return newApp(t, database), database
}

func uploadCSV(t *testing.T, app *fiber.App, filename string, content []byte) (int, []byte) {
	t.Helper()

	return uploadCSVAs(t, app, uploadFileField, filename, content)
}

func uploadCSVAs(t *testing.T, app *fiber.App, field, filename string, content []byte) (int, []byte) {
	t.Helper()

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, filename)
	require.NoError(t, err, "create multipart file part")
	_, err = part.Write(content)
	require.NoError(t, err, "write multipart file part")
	require.NoError(t, writer.Close(), "close multipart writer")

	req := httptest.NewRequest(http.MethodPost, uploadPath, &body)
	req.Header.Set(fiber.HeaderContentType, writer.FormDataContentType())

	resp, err := app.Test(req)
	require.NoError(t, err, "post upload")
	defer func() {
		require.NoError(t, resp.Body.Close(), "close upload body")
	}()

	payload, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read upload body")

	return resp.StatusCode, payload
}

func decodeJSON[T any](t *testing.T, body []byte) T {
	t.Helper()

	var value T
	require.NoError(t, json.Unmarshal(body, &value), "decode body %q", body)

	return value
}

func requireUploadError(t *testing.T, body []byte, want string) {
	t.Helper()

	payload := decodeJSON[uploadError](t, body)
	assert.Equal(t, want, payload.Error)
}

func countRows(t *testing.T, database *gorm.DB, model any) int64 {
	t.Helper()

	var count int64
	require.NoError(t, database.Model(model).Count(&count).Error, "count rows")

	return count
}

func loadEvents(t *testing.T, database *gorm.DB) []db.Event {
	t.Helper()

	var events []db.Event
	require.NoError(t, database.Order("ingest_id, line_number").Find(&events).Error, "load events")

	return events
}

func loadFileEvents(t *testing.T, database *gorm.DB, fileID uint) []db.Event {
	t.Helper()

	var events []db.Event
	require.NoError(t, database.Where("ingest_id = ?", fileID).Order("line_number").Find(&events).Error, "load file events")

	return events
}

func TestUploadFileStoresFileFields(t *testing.T) {
	app, database := newUploadApp(t)

	content := []byte(fullUploadHeader + "\n" +
		"1700000000,alice,10.0.0.1,10.0.0.2,/login,POST,200,allow,portal,128,512,10,low\n")

	code, body := uploadCSV(t, app, "access.csv", content)
	require.Equal(t, http.StatusCreated, code, "body %q", body)

	created := decodeJSON[uploadResponse](t, body)

	assert.NotZero(t, created.ID)
	assert.Equal(t, "access.csv", created.Filename)
	assert.Equal(t, db.FileStatus(db.FileStatusUplodaded), created.Status)
	assert.Zero(t, created.Bytes)

	var file db.File
	require.NoError(t, database.First(&file, created.ID).Error, "load file")

	assert.Equal(t, created.ID, file.ID)
	assert.Equal(t, "access.csv", file.Filename)
	assert.Equal(t, fullUploadHeader, file.Headers)
	assert.Equal(t, db.FileStatus(db.FileStatusUplodaded), file.Status)
	assert.False(t, file.CreatedAt.IsZero(), "created at")
	assert.False(t, file.UpdatedAt.IsZero(), "updated at")

	assert.Zero(t, file.Lines, "lines are left for out-of-band processing")
	assert.Zero(t, file.AcceptedLines, "accepted lines are left for out-of-band processing")
	assert.Zero(t, file.RejectedLines, "rejected lines are left for out-of-band processing")
	assert.Zero(t, file.DuplicateLines, "duplicate lines are left for out-of-band processing")

	assert.Equal(t, int64(1), countRows(t, database, &db.File{}))
	assert.Equal(t, int64(1), countRows(t, database, &db.Event{}))

	events := loadFileEvents(t, database, created.ID)
	require.Len(t, events, 1)
	assert.Equal(t, created.ID, events[0].IngestID, "event belongs to uploaded file")
}

func TestUploadFileRequiresFileField(t *testing.T) {
	t.Run("no multipart body", func(t *testing.T) {
		app, database := newUploadApp(t)

		resp, err := app.Test(httptest.NewRequest(http.MethodPost, uploadPath, nil))
		require.NoError(t, err, "post upload")
		defer func() {
			require.NoError(t, resp.Body.Close(), "close upload body")
		}()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "read upload body")

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		requireUploadError(t, body, "missing CSV file")
		assert.Zero(t, countRows(t, database, &db.File{}))
	})

	t.Run("wrong field name", func(t *testing.T) {
		app, database := newUploadApp(t)

		code, body := uploadCSVAs(t, app, "csv", "access.csv", []byte("time\n1700000000\n"))

		assert.Equal(t, http.StatusBadRequest, code)
		requireUploadError(t, body, "missing CSV file")
		assert.Zero(t, countRows(t, database, &db.File{}))
	})
}

func TestUploadFileRejectsInvalidCSVHeaders(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "empty file",
			content: "",
			want:    "uploaded file is invalid",
		},
		{
			name:    "malformed header",
			content: "\"time\n1700000000\n",
			want:    "uploaded file is invalid",
		},
		{
			name:    "missing time column",
			content: "login,cip\nbob,10.0.0.1\n",
			want:    "cannot read uploaded file: missing time column",
		},
		{
			name:    "time column case mismatch",
			content: "Time\n1700000000\n",
			want:    "cannot read uploaded file: missing time column",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, database := newUploadApp(t)

			code, body := uploadCSV(t, app, "access.csv", []byte(tt.content))

			assert.Equal(t, http.StatusBadRequest, code, "body %q", body)
			requireUploadError(t, body, tt.want)
			assert.Zero(t, countRows(t, database, &db.File{}))
			assert.Zero(t, countRows(t, database, &db.Event{}))
		})
	}
}

func TestUploadFileParsesAllColumns(t *testing.T) {
	app, database := newUploadApp(t)

	record := "1700000000,alice,10.0.0.1,10.0.0.2,/login,POST,201,allow,portal,128,512,75,medium"
	code, body := uploadCSV(t, app, "access.csv", []byte(fullUploadHeader+"\n"+record+"\n"))
	require.Equal(t, http.StatusCreated, code, "body %q", body)

	created := decodeJSON[uploadResponse](t, body)
	events := loadFileEvents(t, database, created.ID)
	require.Len(t, events, 1)

	event := events[0]

	assert.Equal(t, created.ID, event.IngestID)
	assert.Equal(t, int32(2), event.LineNumber)
	assert.True(t, event.Time.Equal(time.Unix(1700000000, 0)), "time %s", event.Time)
	assert.Equal(t, "alice", event.User)
	assert.Equal(t, "10.0.0.1", event.ClientIP)
	assert.Equal(t, "10.0.0.2", event.ServerIP)
	assert.Equal(t, "/login", event.URL)
	assert.Equal(t, "POST", event.Method)
	require.NotNil(t, event.ResponseCode)
	assert.Equal(t, 201, *event.ResponseCode)
	assert.Equal(t, "allow", event.Action)
	assert.Equal(t, "portal", event.AppName)
	require.NotNil(t, event.BytesIn)
	assert.Equal(t, 128, *event.BytesIn)
	require.NotNil(t, event.BytesOut)
	assert.Equal(t, 512, *event.BytesOut)
	require.NotNil(t, event.RiskScore)
	assert.Equal(t, 75, *event.RiskScore)
	assert.Equal(t, "medium", event.Severity)
	assert.Equal(t, record, event.RawLog)
}

func TestUploadFileOmitsOptionalColumns(t *testing.T) {
	app, database := newUploadApp(t)

	code, body := uploadCSV(t, app, "access.csv", []byte("time\n1700000000\n"))
	require.Equal(t, http.StatusCreated, code, "body %q", body)

	created := decodeJSON[uploadResponse](t, body)

	var file db.File
	require.NoError(t, database.First(&file).Error, "load file")
	assert.Equal(t, created.ID, file.ID)
	assert.Equal(t, "time", file.Headers)

	events := loadFileEvents(t, database, created.ID)
	require.Len(t, events, 1)

	event := events[0]

	assert.Equal(t, created.ID, event.IngestID)
	assert.Equal(t, int32(2), event.LineNumber)
	assert.True(t, event.Time.Equal(time.Unix(1700000000, 0)), "time %s", event.Time)
	assert.Empty(t, event.User)
	assert.Empty(t, event.ClientIP)
	assert.Empty(t, event.ServerIP)
	assert.Empty(t, event.URL)
	assert.Empty(t, event.Method)
	assert.Nil(t, event.ResponseCode)
	assert.Empty(t, event.Action)
	assert.Empty(t, event.AppName)
	assert.Nil(t, event.BytesIn)
	assert.Nil(t, event.BytesOut)
	assert.Nil(t, event.RiskScore)
	assert.Empty(t, event.Severity)
	assert.Equal(t, "1700000000", event.RawLog)
}

func TestUploadFileTreatsNullMarkersAsNull(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "none", value: "None"},
		{name: "na", value: "NA"},
		{name: "n slash a", value: "N/A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, database := newUploadApp(t)

			content := fmt.Sprintf(
				"time,respcode,reqsize,respdatasize,riskscore\n1700000000,%[1]s,%[1]s,%[1]s,%[1]s\n",
				tt.value,
			)
			code, body := uploadCSV(t, app, "access.csv", []byte(content))
			require.Equal(t, http.StatusCreated, code, "body %q", body)

			created := decodeJSON[uploadResponse](t, body)
			events := loadFileEvents(t, database, created.ID)
			require.Len(t, events, 1)

			event := events[0]
			assert.Equal(t, created.ID, event.IngestID)
			assert.Nil(t, event.ResponseCode)
			assert.Nil(t, event.BytesIn)
			assert.Nil(t, event.BytesOut)
			assert.Nil(t, event.RiskScore)
		})
	}
}

func TestUploadFileAcceptsRiskScoreBoundaries(t *testing.T) {
	app, database := newUploadApp(t)

	code, body := uploadCSV(t, app, "access.csv", []byte("time,riskscore\n1700000000,0\n1700000001,50\n1700000002,100\n"))
	require.Equal(t, http.StatusCreated, code, "body %q", body)

	created := decodeJSON[uploadResponse](t, body)
	events := loadFileEvents(t, database, created.ID)
	require.Len(t, events, 3)

	for i, want := range []int{0, 50, 100} {
		assert.Equal(t, created.ID, events[i].IngestID)
		require.NotNil(t, events[i].RiskScore)
		assert.Equal(t, want, *events[i].RiskScore)
	}
}

func TestUploadFileRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		header string
		record string
	}{
		{
			name:   "invalid time",
			header: "time",
			record: "not-a-number",
		},
		{
			name:   "invalid response code",
			header: "time,respcode",
			record: "1700000000,abc",
		},
		{
			name:   "invalid request size",
			header: "time,reqsize",
			record: "1700000000,abc",
		},
		{
			name:   "invalid response data size",
			header: "time,respdatasize",
			record: "1700000000,abc",
		},
		{
			name:   "invalid risk score",
			header: "time,riskscore",
			record: "1700000000,abc",
		},
		{
			name:   "risk score below range",
			header: "time,riskscore",
			record: "1700000000,-1",
		},
		{
			name:   "risk score above range",
			header: "time,riskscore",
			record: "1700000000,101",
		},
		{
			name:   "ragged record",
			header: "time,login",
			record: "1700000000",
		},
		{
			name:   "malformed record",
			header: "time,login",
			record: "\"1700000000,alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, database := newUploadApp(t)

			content := fmt.Sprintf("%s\n%s\n", tt.header, tt.record)
			code, body := uploadCSV(t, app, "access.csv", []byte(content))

			assert.Equal(t, http.StatusInternalServerError, code, "body %q", body)
			requireUploadError(t, body, "cannot store file")
			assert.Zero(t, countRows(t, database, &db.File{}), "file must not persist")
			assert.Zero(t, countRows(t, database, &db.Event{}), "events must not persist")
		})
	}
}

func TestUploadFileHandlesMultipleBatches(t *testing.T) {
	app, database := newUploadApp(t)

	const rows = 2*uploadChunkSize + 1

	var content strings.Builder
	content.WriteString("time\n")
	for i := range rows {
		fmt.Fprintf(&content, "%d\n", 1700000000+i)
	}

	code, body := uploadCSV(t, app, "large.csv", []byte(content.String()))
	require.Equal(t, http.StatusCreated, code, "body %q", body)

	created := decodeJSON[uploadResponse](t, body)

	var file db.File
	require.NoError(t, database.First(&file).Error, "load file")
	assert.Equal(t, created.ID, file.ID)
	assert.Equal(t, "large.csv", file.Filename)
	assert.Zero(t, file.Lines)
	assert.Zero(t, file.AcceptedLines)
	assert.Zero(t, file.RejectedLines)
	assert.Zero(t, file.DuplicateLines)

	events := loadFileEvents(t, database, created.ID)
	require.Len(t, events, rows)
	assert.Equal(t, int32(2), events[0].LineNumber)
	assert.Equal(t, int32(rows+1), events[len(events)-1].LineNumber)

	for _, event := range events {
		assert.Equal(t, created.ID, event.IngestID, "event on line %d", event.LineNumber)
	}
}

func TestUploadFileStoresMultipleFilesWithRelatedEvents(t *testing.T) {
	app, database := newUploadApp(t)

	uploads := []struct {
		filename string
		header   string
		records  []string
	}{
		{
			filename: "first.csv",
			header:   "time,login,url",
			records: []string{
				"1700000000,alice,/one",
				"1700000001,bob,/two",
			},
		},
		{
			filename: "second.csv",
			header:   "time,cip,respcode",
			records: []string{
				"1700000100,10.0.0.9,200",
				"1700000101,10.0.0.10,404",
				"1700000102,10.0.0.11,500",
			},
		},
		{
			filename: "third.csv",
			header:   "time",
			records: []string{
				"1700000200",
			},
		},
	}

	created := make([]uploadResponse, 0, len(uploads))
	for _, upload := range uploads {
		content := upload.header + "\n" + strings.Join(upload.records, "\n") + "\n"

		code, body := uploadCSV(t, app, upload.filename, []byte(content))
		require.Equal(t, http.StatusCreated, code, "upload %q body %q", upload.filename, body)

		created = append(created, decodeJSON[uploadResponse](t, body))
	}

	assert.Equal(t, int64(len(uploads)), countRows(t, database, &db.File{}))
	assert.Equal(t, int64(6), countRows(t, database, &db.Event{}), "events from every file persist")

	seenIDs := make(map[uint]struct{}, len(created))
	for i, upload := range uploads {
		file := created[i]

		assert.NotZero(t, file.ID, "upload %q id", upload.filename)
		assert.Equal(t, upload.filename, file.Filename)
		assert.NotContains(t, seenIDs, file.ID, "file IDs are unique")
		seenIDs[file.ID] = struct{}{}

		var stored db.File
		require.NoError(t, database.First(&stored, file.ID).Error, "load file %q", upload.filename)

		assert.Equal(t, upload.filename, stored.Filename)
		assert.Equal(t, upload.header, stored.Headers)
		assert.Equal(t, db.FileStatus(db.FileStatusUplodaded), stored.Status)
		assert.Zero(t, stored.Lines)
		assert.Zero(t, stored.AcceptedLines)
		assert.Zero(t, stored.RejectedLines)
		assert.Zero(t, stored.DuplicateLines)

		events := loadFileEvents(t, database, file.ID)
		require.Len(t, events, len(upload.records), "events for %q", upload.filename)

		for row, event := range events {
			assert.Equal(t, file.ID, event.IngestID, "event %d of %q", row, upload.filename)
			assert.Equal(t, int32(row+2), event.LineNumber, "event %d of %q", row, upload.filename)
			assert.Equal(t, upload.records[row], event.RawLog, "event %d of %q", row, upload.filename)
		}
	}

	for _, upload := range uploads {
		var file db.File
		require.NoError(t, database.Where("filename = ?", upload.filename).First(&file).Error, "load file by name %q", upload.filename)
		assert.Equal(t, int64(len(upload.records)), countFileEvents(t, database, file.ID), "event count for %q", upload.filename)
	}

	events := loadEvents(t, database)
	require.Len(t, events, 6)

	for _, event := range events {
		assert.Contains(t, seenIDs, event.IngestID, "event on line %d maps to an uploaded file", event.LineNumber)

		var file db.File
		require.NoError(t, database.First(&file, event.IngestID).Error, "load file for event on line %d", event.LineNumber)
		assert.NotEmpty(t, file.Filename)
	}
}

func countFileEvents(t *testing.T, database *gorm.DB, fileID uint) int64 {
	t.Helper()

	var count int64
	require.NoError(t, database.Model(&db.Event{}).Where("ingest_id = ?", fileID).Count(&count).Error, "count file events")

	return count
}
