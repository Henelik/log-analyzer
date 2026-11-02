package rest

import (
	"encoding/json"
	"fmt"
	"io"
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

var eventsEpoch = time.Unix(1700000000, 0)

func eventPath(id uint) string {
	return filePath(id) + "/events"
}

func postJSON(t *testing.T, app *fiber.App, path, body string) (int, []byte) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	resp, err := app.Test(req)
	require.NoError(t, err, "post %s", path)
	defer func() {
		require.NoError(t, resp.Body.Close(), "close %s body", path)
	}()

	payload, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read %s body", path)

	return resp.StatusCode, payload
}

func seedFileEvents(t *testing.T, database *gorm.DB, fileID uint, events []db.Event) {
	t.Helper()

	for i := range events {
		events[i].IngestID = fileID
	}

	if len(events) == 0 {
		return
	}

	require.NoError(t, database.CreateInBatches(&events, 100).Error, "seed events")
}

func TestGetEventsReturnsRequestedEvents(t *testing.T) {
	app, database := newFilesApp(t)

	file := seedFiles(t, database)[0]

	port, responseCode, bytesIn, bytesOut := 443, 201, 128, 512

	seedFileEvents(t, database, file.ID, []db.Event{
		{
			LineNumber:   2,
			Time:         eventsEpoch,
			User:         "alice",
			ClientIP:     "10.0.0.1",
			ServerIP:     "10.0.0.2",
			ServerPort:   &port,
			URL:          "/login",
			Method:       "POST",
			ResponseCode: &responseCode,
			Action:       "Login Success",
			AppName:      "portal",
			BytesIn:      &bytesIn,
			BytesOut:     &bytesOut,
			RawLog:       "1700000000,alice,10.0.0.1,10.0.0.2,443,/login,POST,201,Login Success,portal,128,512",
			Status:       db.EventStatusParsed,
		},
		{
			LineNumber: 3,
			RawLog:     "1700000001,bob",
			Status:     db.EventStatusRejected,
			Errors:     "invalid srv_dport",
		},
	})

	code, body := postJSON(t, app, eventPath(file.ID), `[2,3]`)
	require.Equal(t, http.StatusOK, code, "body %q", body)

	payload := decodeJSON[eventsResponse](t, body)
	require.Len(t, payload.Events, 2)

	parsed := payload.Events[0]
	assert.Equal(t, int32(2), parsed.LineNumber)
	require.NotNil(t, parsed.Time)
	assert.True(t, parsed.Time.Equal(eventsEpoch), "time %s", parsed.Time)
	assert.Equal(t, "alice", parsed.User)
	assert.Equal(t, "10.0.0.1", parsed.ClientIP)
	assert.Equal(t, "10.0.0.2", parsed.ServerIP)
	require.NotNil(t, parsed.ServerPort)
	assert.Equal(t, 443, *parsed.ServerPort)
	assert.Equal(t, "/login", parsed.URL)
	assert.Equal(t, "POST", parsed.Method)
	require.NotNil(t, parsed.ResponseCode)
	assert.Equal(t, 201, *parsed.ResponseCode)
	assert.Equal(t, "Login Success", parsed.Action)
	assert.Equal(t, "portal", parsed.AppName)
	require.NotNil(t, parsed.BytesIn)
	assert.Equal(t, 128, *parsed.BytesIn)
	require.NotNil(t, parsed.BytesOut)
	assert.Equal(t, 512, *parsed.BytesOut)
	assert.Equal(t, "1700000000,alice,10.0.0.1,10.0.0.2,443,/login,POST,201,Login Success,portal,128,512", parsed.RawLog)
	assert.Equal(t, db.EventStatusParsed, parsed.Status)
	assert.Empty(t, parsed.Errors)

	rejected := payload.Events[1]
	assert.Equal(t, int32(3), rejected.LineNumber)
	assert.Nil(t, rejected.Time, "zero time is null")
	assert.Empty(t, rejected.User)
	assert.Nil(t, rejected.ServerPort)
	assert.Nil(t, rejected.ResponseCode)
	assert.Nil(t, rejected.BytesIn)
	assert.Nil(t, rejected.BytesOut)
	assert.Equal(t, "1700000001,bob", rejected.RawLog)
	assert.Equal(t, db.EventStatusRejected, rejected.Status)
	assert.Equal(t, "invalid srv_dport", rejected.Errors)
}

func TestGetEventsPreservesSubmittedOrderAndOmitsUnknownLines(t *testing.T) {
	app, database := newFilesApp(t)

	file := seedFiles(t, database)[0]

	seedFileEvents(t, database, file.ID, []db.Event{
		{LineNumber: 2, RawLog: "two", Status: db.EventStatusParsed},
		{LineNumber: 3, RawLog: "three", Status: db.EventStatusParsed},
		{LineNumber: 4, RawLog: "four", Status: db.EventStatusParsed},
	})

	code, body := postJSON(t, app, eventPath(file.ID), `[4,99,2,4,3]`)
	require.Equal(t, http.StatusOK, code, "body %q", body)

	payload := decodeJSON[eventsResponse](t, body)
	require.Len(t, payload.Events, 3)

	lineNumbers := make([]int32, 0, len(payload.Events))
	for _, event := range payload.Events {
		lineNumbers = append(lineNumbers, event.LineNumber)
	}

	assert.Equal(t, []int32{4, 2, 3}, lineNumbers)
	assert.Equal(t, "four", payload.Events[0].RawLog)
}

func TestGetEventsScopesLineNumbersToFile(t *testing.T) {
	app, database := newFilesApp(t)

	files := seedFiles(t, database)

	seedFileEvents(t, database, files[0].ID, []db.Event{
		{LineNumber: 2, RawLog: "first", Status: db.EventStatusParsed},
	})
	seedFileEvents(t, database, files[1].ID, []db.Event{
		{LineNumber: 2, RawLog: "second", Status: db.EventStatusParsed},
	})

	code, body := postJSON(t, app, eventPath(files[0].ID), `[2]`)
	require.Equal(t, http.StatusOK, code, "body %q", body)

	payload := decodeJSON[eventsResponse](t, body)
	require.Len(t, payload.Events, 1)
	assert.Equal(t, "first", payload.Events[0].RawLog, "events from other files are not returned")
}

func TestGetEventsEmptyList(t *testing.T) {
	app, database := newFilesApp(t)

	file := seedFiles(t, database)[0]

	code, body := postJSON(t, app, eventPath(file.ID), `[]`)
	require.Equal(t, http.StatusOK, code, "body %q", body)

	assert.JSONEq(t, `{"events":[]}`, string(body))
}

func TestGetEventsRejectsInvalidLineNumbers(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty body", body: ""},
		{name: "malformed json", body: "["},
		{name: "null", body: "null"},
		{name: "object", body: `{"line_numbers":[2]}`},
		{name: "string element", body: `["2"]`},
		{name: "fractional element", body: `[2.5]`},
		{name: "zero line", body: `[0]`},
		{name: "negative line", body: `[-3]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, database := newFilesApp(t)

			file := seedFiles(t, database)[0]
			seedFileEvents(t, database, file.ID, []db.Event{
				{LineNumber: 2, RawLog: "two", Status: db.EventStatusParsed},
			})

			code, body := postJSON(t, app, eventPath(file.ID), tt.body)

			assert.Equal(t, http.StatusBadRequest, code, "body %q", body)
			requireUploadError(t, body, "invalid line numbers")
		})
	}
}

func TestGetEventsInvalidFileID(t *testing.T) {
	app, _ := newFilesApp(t)

	for _, id := range []string{"abc", "1.5"} {
		t.Run(id, func(t *testing.T) {
			code, body := postJSON(t, app, "/api/v1/files/"+id+"/events", `[2]`)

			assert.Equal(t, http.StatusBadRequest, code)
			requireUploadError(t, body, "invalid file ID")
		})
	}
}

func TestGetEventsFileNotFound(t *testing.T) {
	app, database := newFilesApp(t)

	seedFiles(t, database)

	code, body := postJSON(t, app, "/api/v1/files/999/events", `[2]`)

	assert.Equal(t, http.StatusNotFound, code)
	requireUploadError(t, body, "file not found")
}

func TestGetEventsHandlesMoreLineNumbersThanOneQueryChunk(t *testing.T) {
	app, database := newFilesApp(t)

	file := seedFiles(t, database)[0]

	rows := 2*eventQueryChunkSize + 5

	events := make([]db.Event, 0, rows)
	lineNumbers := make([]int32, 0, rows)

	for i := range rows {
		lineNumber := int32(i + 2)

		events = append(events, db.Event{
			LineNumber: lineNumber,
			Time:       eventsEpoch.Add(time.Duration(i) * time.Second),
			RawLog:     fmt.Sprintf("%d", 1700000000+i),
			Status:     db.EventStatusParsed,
		})
		lineNumbers = append(lineNumbers, lineNumber)
	}

	seedFileEvents(t, database, file.ID, events)

	request, err := json.Marshal(lineNumbers)
	require.NoError(t, err, "encode line numbers")

	code, body := postJSON(t, app, eventPath(file.ID), string(request))
	require.Equal(t, http.StatusOK, code, "body %q", body)

	payload := decodeJSON[eventsResponse](t, body)
	require.Len(t, payload.Events, rows)

	for i, event := range payload.Events {
		assert.Equal(t, lineNumbers[i], event.LineNumber, "event %d", i)
	}
}

func getEventErrorsPath(id uint) string {
	return filePath(id) + "/errors"
}

func TestGetEventErrorsReturnsOnlyRejectedEvents(t *testing.T) {
	app, database := newFilesApp(t)

	file := seedFiles(t, database)[0]

	seedFileEvents(t, database, file.ID, []db.Event{
		{LineNumber: 2, RawLog: "1700000000,alice", Status: db.EventStatusParsed},
		{LineNumber: 3, RawLog: "1700000001,bob", Status: db.EventStatusRejected, Errors: "invalid srv_dport"},
		{LineNumber: 4, RawLog: "1700000002,carol", Status: db.EventStatusParsed},
		{LineNumber: 5, RawLog: "1700000003,dave", Status: db.EventStatusRejected, Errors: "failed to parse epochtime; invalid reqsize"},
	})

	code, body := doRequest(t, app, http.MethodGet, getEventErrorsPath(file.ID))
	require.Equal(t, http.StatusOK, code, "body %q", body)

	payload := decodeJSON[eventErrorsResponse](t, body)
	require.Len(t, payload.Errors, 2)

	assert.Equal(t, eventErrorResponse{
		LineNumber: 3,
		RawLog:     "1700000001,bob",
		Error:      "invalid srv_dport",
	}, payload.Errors[0])

	assert.Equal(t, eventErrorResponse{
		LineNumber: 5,
		RawLog:     "1700000003,dave",
		Error:      "failed to parse epochtime; invalid reqsize",
	}, payload.Errors[1])
}

func TestGetEventErrorsEmpty(t *testing.T) {
	app, database := newFilesApp(t)

	file := seedFiles(t, database)[0]

	seedFileEvents(t, database, file.ID, []db.Event{
		{LineNumber: 2, RawLog: "two", Status: db.EventStatusParsed},
	})

	code, body := doRequest(t, app, http.MethodGet, getEventErrorsPath(file.ID))
	require.Equal(t, http.StatusOK, code, "body %q", body)

	assert.JSONEq(t, `{"errors":[]}`, string(body))

	t.Run("file without events", func(t *testing.T) {
		app, database := newFilesApp(t)

		file := seedFiles(t, database)[0]

		code, body := doRequest(t, app, http.MethodGet, getEventErrorsPath(file.ID))
		require.Equal(t, http.StatusOK, code, "body %q", body)

		assert.JSONEq(t, `{"errors":[]}`, string(body))
	})
}

func TestGetEventErrorsScopesToFile(t *testing.T) {
	app, database := newFilesApp(t)

	files := seedFiles(t, database)

	seedFileEvents(t, database, files[0].ID, []db.Event{
		{LineNumber: 2, RawLog: "first", Status: db.EventStatusRejected, Errors: "first error"},
	})
	seedFileEvents(t, database, files[1].ID, []db.Event{
		{LineNumber: 2, RawLog: "second", Status: db.EventStatusRejected, Errors: "second error"},
	})

	code, body := doRequest(t, app, http.MethodGet, getEventErrorsPath(files[0].ID))
	require.Equal(t, http.StatusOK, code, "body %q", body)

	payload := decodeJSON[eventErrorsResponse](t, body)
	require.Len(t, payload.Errors, 1)
	assert.Equal(t, "first", payload.Errors[0].RawLog)
}

func TestGetEventErrorsInvalidFileID(t *testing.T) {
	app, _ := newFilesApp(t)

	for _, id := range []string{"abc", "1.5"} {
		t.Run(id, func(t *testing.T) {
			code, body := doRequest(t, app, http.MethodGet, "/api/v1/files/"+id+"/errors")

			assert.Equal(t, http.StatusBadRequest, code)
			requireUploadError(t, body, "invalid file ID")
		})
	}
}

func TestGetEventErrorsFileNotFound(t *testing.T) {
	app, database := newFilesApp(t)

	seedFiles(t, database)

	code, body := doRequest(t, app, http.MethodGet, "/api/v1/files/999/errors")

	assert.Equal(t, http.StatusNotFound, code)
	requireUploadError(t, body, "file not found")
}
