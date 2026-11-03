package worker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Henelik/log-analyzer/db"
	"github.com/Henelik/log-analyzer/ollama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type chatCall struct {
	request ollama.ChatRequest
}

type fakeChatClient struct {
	mu        sync.Mutex
	responses []fakeChatResponse
	fallback  *fakeChatResponse
	calls     []chatCall
}

type fakeChatResponse struct {
	content string
	err     error
}

func (fake *fakeChatClient) Chat(_ context.Context, request ollama.ChatRequest) (ollama.ChatResponse, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	fake.calls = append(fake.calls, chatCall{request: request})

	var response fakeChatResponse

	switch {
	case len(fake.responses) > 0:
		response = fake.responses[0]
		fake.responses = fake.responses[1:]
	case fake.fallback != nil:
		response = *fake.fallback
	default:
		return ollama.ChatResponse{}, errors.New("unexpected chat call")
	}

	if response.err != nil {
		return ollama.ChatResponse{}, response.err
	}

	return ollama.ChatResponse{
		Message: ollama.Message{Role: ollama.RoleAssistant, Content: response.content},
	}, nil
}

func openDescriptionTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	database, err := db.NewTestDB()
	require.NoError(t, err, "open test database")
	require.NoError(t, database.AutoMigrate(&db.File{}, &db.Event{}, &db.Detection{}), "migrate test database")

	sqlDB, err := database.DB()
	require.NoError(t, err, "get sql database")
	sqlDB.SetMaxOpenConns(1)

	return database
}

func seedDetections(t *testing.T, database *gorm.DB, detections []db.Detection) {
	t.Helper()

	for i := range detections {
		if detections[i].Events == nil {
			detections[i].Events = db.Int32Array{}
		}
	}

	require.NoError(t, database.Create(&detections).Error, "seed detections")
}

func seedDetectionEvents(t *testing.T, database *gorm.DB, events []db.Event) {
	t.Helper()

	require.NoError(t, database.Create(&events).Error, "seed events")
}

func loadDetection(t *testing.T, database *gorm.DB, ingestID, detectionID uint) db.Detection {
	t.Helper()

	var detection db.Detection
	require.NoError(t, database.
		Where("ingest_id = ? AND detection_id = ?", ingestID, detectionID).
		First(&detection).Error, "load detection")

	return detection
}

func validDescription(content string) string {
	encoded, err := json.Marshal(map[string]string{"detection_notes": content})
	if err != nil {
		panic(err)
	}

	return string(encoded)
}

func TestDescriptionWorkerDescribesUndescribedDetections(t *testing.T) {
	database := openDescriptionTestDB(t)

	require.NoError(t, database.Create(&db.File{Filename: "access.csv", Headers: "epochtime", Status: db.FileStatusAnalyzed}).Error, "seed file")

	port := 443
	responseCode := 200
	bytesIn := 128
	bytesOut := 512

	seedDetectionEvents(t, database, []db.Event{
		{
			IngestID:     1,
			LineNumber:   42,
			Time:         time.Unix(1700000000, 0),
			User:         "alice",
			ClientIP:     "10.0.0.1",
			ServerIP:     "10.0.0.2",
			ServerPort:   &port,
			URL:          "/login",
			Method:       "POST",
			ResponseCode: &responseCode,
			Action:       "Login Fail",
			AppName:      "portal",
			BytesIn:      &bytesIn,
			BytesOut:     &bytesOut,
			RawLog:       "1700000000,alice,10.0.0.1,10.0.0.2,443,/login,POST,200,Login Fail,portal,128,512",
			Status:       db.EventStatusParsed,
			Errors:       "secret error text",
		},
	})

	seedDetections(t, database, []db.Detection{
		{IngestID: 1, DetectionID: 1, Notes: "", Severity: 80, Confidence: 60, Priority: 70, Entity: "alice", Observed: 12, Threshold: 10, Events: db.Int32Array{42}},
		{IngestID: 1, DetectionID: 2, Description: "existing description", Notes: "existing notes", Severity: 50},
	})

	chat := &fakeChatClient{responses: []fakeChatResponse{{content: validDescription("Repeated login failures from alice.")}}}

	worker := NewDescriptionWorker(database, chat, "test-model")
	worker.run(context.Background())

	require.Len(t, chat.calls, 1, "only detections without notes are described")

	request := chat.calls[0].request
	assert.Equal(t, "test-model", request.Model)
	require.Len(t, request.Messages, 2)
	assert.Equal(t, ollama.RoleSystem, request.Messages[0].Role)
	assert.Equal(t, ollama.RoleUser, request.Messages[1].Role)
	assert.JSONEq(t, string(descriptionSchema), string(request.Format), "response schema is sent with the request")

	prompt := request.Messages[1].Content

	for _, field := range []string{
		"severity: 80",
		"confidence: 60",
		"priority: 70",
		"entity: alice",
		"observed: 12",
		"threshold: 10",
	} {
		assert.Contains(t, prompt, field)
	}

	assert.Contains(t, prompt, "events[1]{time,user,client_ip,server_ip,server_port,url,method,response_code,action,app_name,bytes_in,bytes_out}:", "events render as a TOON table")
	assert.Contains(t, prompt, `"2023-11-14T22:13:20Z",alice,10.0.0.1,10.0.0.2,443,/login,POST,200,Login Fail,portal,128,512`)

	for name, excluded := range map[string]string{
		"raw log":       "1700000000,alice",
		"errors":        "secret error text",
		"ingest ID":     "ingest_id",
		"detection ID":  "detection_id",
		"line number":   "line_number",
		"status":        "status",
		"existing note": "existing notes",
		"notes key":     "notes:",
	} {
		assert.NotContains(t, prompt, excluded, "%s is excluded from the prompt", name)
	}

	assert.Equal(t, "Repeated login failures from alice.", loadDetection(t, database, 1, 1).Notes)
	assert.Equal(t, "existing notes", loadDetection(t, database, 1, 2).Notes, "existing notes are not overwritten")
}

func TestDescriptionPromptExcludesNotes(t *testing.T) {
	prompt := descriptionPrompt(db.Detection{Entity: "alice", Severity: 80}, nil)

	assert.NotContains(t, prompt, "notes")
	assert.Contains(t, prompt, "events: []")
}

func TestDescriptionWorkerSkipsRemainingDetectionsWhenChatFails(t *testing.T) {
	database := openDescriptionTestDB(t)

	seedDetections(t, database, []db.Detection{
		{IngestID: 1, DetectionID: 1, Severity: 80},
		{IngestID: 1, DetectionID: 2, Severity: 80},
		{IngestID: 1, DetectionID: 3, Severity: 80},
	})

	chat := &fakeChatClient{responses: []fakeChatResponse{
		{content: validDescription("first")},
		{err: errors.New("connection refused")},
	}}

	worker := NewDescriptionWorker(database, chat, "test-model")
	worker.run(context.Background())

	assert.Len(t, chat.calls, 2, "stops calling Ollama after the first failure")
	assert.Equal(t, "first", loadDetection(t, database, 1, 1).Notes)
	assert.Empty(t, loadDetection(t, database, 1, 2).Notes, "failed detection is not written")
	assert.Empty(t, loadDetection(t, database, 1, 3).Notes, "remaining detections are skipped")
}

func TestDescriptionWorkerMarksInvalidResponses(t *testing.T) {
	database := openDescriptionTestDB(t)

	seedDetections(t, database, []db.Detection{
		{IngestID: 1, DetectionID: 1, Severity: 80},
		{IngestID: 1, DetectionID: 2, Severity: 80},
	})

	chat := &fakeChatClient{responses: []fakeChatResponse{
		{content: "not json"},
		{content: validDescription("second")},
	}}

	worker := NewDescriptionWorker(database, chat, "test-model")
	worker.run(context.Background())

	assert.Equal(t, invalidNotes, loadDetection(t, database, 1, 1).Notes)
	assert.Equal(t, "second", loadDetection(t, database, 1, 2).Notes, "processing continues after an invalid response")
}

func TestDescriptionWorkerRejectsMalformedJSON(t *testing.T) {
	cases := map[string]string{
		"empty":           "",
		"missing field":   `{}`,
		"wrong type":      `{"detection_notes": 12}`,
		"empty value":     `{"detection_notes": "   "}`,
		"unknown field":   `{"detection_notes": "ok", "extra": true}`,
		"trailing data":   `{"detection_notes": "ok"}{"detection_notes": "again"}`,
		"array":           `[{"detection_notes": "ok"}]`,
		"unquoted string": `detection_notes: ok`,
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := parseDescription(content)
			assert.Error(t, err)
		})
	}
}

func TestParseDescriptionTrimsContent(t *testing.T) {
	description, err := parseDescription(validDescription("  summarized  "))
	require.NoError(t, err)
	assert.Equal(t, "summarized", description)
}

func TestParseDescriptionStripsCodeQuoting(t *testing.T) {
	const notes = "A single external IP is scanning multiple internal servers."

	cases := map[string]string{
		"json fence":       "```json\n" + validDescription(notes) + "\n```",
		"json fence upper": "```JSON\n" + validDescription(notes) + "\n```",
		"plain fence":      "```\n" + validDescription(notes) + "\n```",
		"padded":           "\n\n```json\n" + validDescription(notes) + "\n```\n\n",
		"crlf":             "```json\r\n" + validDescription(notes) + "\r\n```",
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			description, err := parseDescription(content)
			require.NoError(t, err)
			assert.Equal(t, notes, description)
		})
	}
}

func TestParseDescriptionRejectsCodeQuotingWithTrailingText(t *testing.T) {
	content := "Here you go:\n```json\n" + validDescription("notes") + "\n```\n"

	_, err := parseDescription(content)
	assert.Error(t, err, "text outside the fence is not silently discarded")
}

func TestParseDescriptionRejectsInvalidFencedJSON(t *testing.T) {
	_, err := parseDescription("```json\nnot json\n```")
	assert.Error(t, err)
}

func TestStripCodeQuotingLeavesPlainContentAlone(t *testing.T) {
	for _, content := range []string{
		validDescription("notes"),
		"```unterminated",
		"```json\nno closing fence",
		"text\n```json\n{}\n```",
	} {
		assert.Equal(t, content, stripCodeQuoting(content))
	}
}

func TestDescriptionWorkerAcceptsFencedResponse(t *testing.T) {
	database := openDescriptionTestDB(t)

	seedDetections(t, database, []db.Detection{{IngestID: 1, DetectionID: 1}})

	fenced := "```json\n" + validDescription("fenced notes") + "\n```"
	chat := &fakeChatClient{responses: []fakeChatResponse{{content: fenced}}}

	worker := NewDescriptionWorker(database, chat, "test-model")
	worker.run(context.Background())

	assert.Equal(t, "fenced notes", loadDetection(t, database, 1, 1).Notes)
}

func TestDescriptionWorkerStartRequiresModel(t *testing.T) {
	database := openDescriptionTestDB(t)

	seedDetections(t, database, []db.Detection{{IngestID: 1, DetectionID: 1}})

	chat := &fakeChatClient{}

	worker := NewDescriptionWorker(database, chat, "")
	worker.Start(0)

	assert.Empty(t, chat.calls, "worker does not run without a model")

	worker.Stop()
}

func TestDescriptionWorkerStartStop(t *testing.T) {
	database := openDescriptionTestDB(t)

	seedDetections(t, database, []db.Detection{{IngestID: 1, DetectionID: 1}})

	chat := &fakeChatClient{fallback: &fakeChatResponse{content: validDescription("described")}}

	worker := NewDescriptionWorker(database, chat, "test-model")
	worker.Start(50 * time.Millisecond)

	require.Eventually(t, func() bool {
		var detection db.Detection
		if err := database.
			Where("ingest_id = ? AND detection_id = ?", 1, 1).
			First(&detection).Error; err != nil {
			return false
		}

		return detection.Notes == "described"
	}, 5*time.Second, 10*time.Millisecond, "worker describes on start")

	worker.Stop()
	worker.Stop()
}
