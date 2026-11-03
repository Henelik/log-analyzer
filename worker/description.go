package worker

import (
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"log"
	"strings"
	"time"

	"github.com/Henelik/log-analyzer/db"
	"github.com/Henelik/log-analyzer/ollama"
	"github.com/pkg/errors"
	toon "github.com/toon-format/toon-go"
	"gorm.io/gorm"
)

const (
	DescriptionModelEnv = "OLLAMA_MODEL"

	invalidNotes = "AI summary returned invalid"

	defaultDescriptionInterval = 30 * time.Second
)

//go:embed description_system_prompt.md
var descriptionSystemPrompt string

var descriptionSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"detection_notes": {
			"type": "string",
			"minLength": 1
		}
	},
	"required": ["detection_notes"],
	"additionalProperties": false
}`)

type ChatClient interface {
	Chat(ctx context.Context, request ollama.ChatRequest) (ollama.ChatResponse, error)
}

type DescriptionWorker struct {
	database *gorm.DB
	chat     ChatClient
	model    string
	interval time.Duration

	ctx    context.Context
	cancel context.CancelFunc

	stop chan struct{}
	done chan struct{}
}

func NewDescriptionWorker(database *gorm.DB, chat ChatClient, model string) *DescriptionWorker {
	return &DescriptionWorker{
		database: database,
		chat:     chat,
		model:    model,
	}
}

func (worker *DescriptionWorker) Start(interval time.Duration) {
	if worker.stop != nil {
		return
	}

	if strings.TrimSpace(worker.model) == "" {
		log.Printf("description worker: not starting: %s is not set", DescriptionModelEnv)

		return
	}

	if interval <= 0 {
		interval = defaultDescriptionInterval
	}

	worker.interval = interval
	worker.ctx, worker.cancel = context.WithCancel(context.Background())
	worker.stop = make(chan struct{})
	worker.done = make(chan struct{})

	go worker.loop()
}

func (worker *DescriptionWorker) loop() {
	defer close(worker.done)

	worker.run(worker.ctx)

	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()

	for {
		select {
		case <-worker.stop:
			return
		case <-ticker.C:
			worker.run(worker.ctx)
		}
	}
}

func (worker *DescriptionWorker) run(ctx context.Context) {
	var detections []db.Detection

	err := worker.database.
		Where("notes IS NULL OR notes = ?", "").
		Order("ingest_id, detection_id").
		Find(&detections).Error
	if err != nil {
		log.Printf("description worker: failed to query detections without notes: %v", err)

		return
	}

	for _, detection := range detections {
		events, err := worker.loadEvents(detection)
		if err != nil {
			log.Printf(
				"description worker: failed to load events for detection %d/%d: %v",
				detection.IngestID,
				detection.DetectionID,
				err,
			)

			continue
		}

		response, err := worker.chat.Chat(ctx, worker.request(detection, events))
		if err != nil {
			log.Printf(
				"description worker: ollama chat failed for detection %d/%d: %v; skipping remaining detections this tick",
				detection.IngestID,
				detection.DetectionID,
				err,
			)

			return
		}

		description, err := parseDescription(response.Message.Content)
		if err != nil {
			log.Printf(
				"description worker: ollama returned an invalid response for detection %d/%d: %v; response: %s",
				detection.IngestID,
				detection.DetectionID,
				err,
				response.Message.Content,
			)

			if updateErr := worker.updateNotes(detection, invalidNotes); updateErr != nil {
				log.Printf(
					"description worker: failed to mark detection %d/%d invalid: %v",
					detection.IngestID,
					detection.DetectionID,
					updateErr,
				)
			}

			continue
		}

		if err := worker.updateNotes(detection, description); err != nil {
			log.Printf(
				"description worker: failed to update detection %d/%d: %v",
				detection.IngestID,
				detection.DetectionID,
				err,
			)
		}
	}
}

func (worker *DescriptionWorker) request(detection db.Detection, events []db.Event) ollama.ChatRequest {
	return ollama.ChatRequest{
		Model: worker.model,
		Messages: []ollama.Message{
			{Role: ollama.RoleSystem, Content: descriptionSystemPrompt},
			{Role: ollama.RoleUser, Content: descriptionPrompt(detection, events)},
		},
		Format: descriptionSchema,
		Options: map[string]any{
			"temperature": 0,
		},
	}
}

// loadEvents returns the events a detection points at.  Raw logs, parse status,
// errors, ingest IDs, and line numbers are omitted from the prompt data.
func (worker *DescriptionWorker) loadEvents(detection db.Detection) ([]db.Event, error) {
	lineNumbers := []int32(detection.Events)
	if len(lineNumbers) == 0 {
		return nil, nil
	}

	var events []db.Event

	err := worker.database.
		Omit("RawLog", "Errors").
		Where("ingest_id = ? AND line_number IN ?", detection.IngestID, lineNumbers).
		Order(`"time", line_number`).
		Find(&events).Error

	return events, errors.Wrap(err, "query detection events")
}

func (worker *DescriptionWorker) updateNotes(detection db.Detection, notes string) error {
	err := worker.database.
		Model(&db.Detection{}).
		Where("ingest_id = ? AND detection_id = ?", detection.IngestID, detection.DetectionID).
		Update("notes", notes).Error

	return errors.Wrap(err, "update detection notes")
}

func descriptionPrompt(detection db.Detection, events []db.Event) string {
	fields := []toon.Field{
		{Key: "title", Value: detection.Description},
		{Key: "severity", Value: detection.Severity},
		{Key: "confidence", Value: detection.Confidence},
		{Key: "priority", Value: detection.Priority},
		{Key: "entity", Value: detection.Entity},
		{Key: "observed", Value: detection.Observed},
		{Key: "threshold", Value: detection.Threshold},
	}

	prompt, err := toon.MarshalString(toon.NewObject(
		toon.Field{Key: "detection", Value: toon.NewObject(fields...)},
		toon.Field{Key: "events", Value: descriptionEvents(events)},
	))
	if err != nil {
		// All values used here are scalars and uniform rows, so the encoder
		// cannot fail on caller data.
		return "detection:\n  entity: " + detection.Entity
	}

	return prompt
}

// descriptionEvent is the prompt projection of an event.  It omits the raw
// log, parse status, errors, ingest ID, and line number.
type descriptionEvent struct {
	Time         time.Time `toon:"time"`
	User         string    `toon:"user"`
	ClientIP     string    `toon:"client_ip"`
	ServerIP     string    `toon:"server_ip"`
	ServerPort   *int      `toon:"server_port"`
	URL          string    `toon:"url"`
	Method       string    `toon:"method"`
	ResponseCode *int      `toon:"response_code"`
	Action       string    `toon:"action"`
	AppName      string    `toon:"app_name"`
	BytesIn      *int      `toon:"bytes_in"`
	BytesOut     *int      `toon:"bytes_out"`
}

func descriptionEvents(events []db.Event) []descriptionEvent {
	projected := make([]descriptionEvent, 0, len(events))

	for _, event := range events {
		projected = append(projected, descriptionEvent{
			Time:         event.Time.UTC(),
			User:         event.User,
			ClientIP:     event.ClientIP,
			ServerIP:     event.ServerIP,
			ServerPort:   event.ServerPort,
			URL:          event.URL,
			Method:       event.Method,
			ResponseCode: event.ResponseCode,
			Action:       event.Action,
			AppName:      event.AppName,
			BytesIn:      event.BytesIn,
			BytesOut:     event.BytesOut,
		})
	}

	return projected
}

type descriptionResponse struct {
	Description string `json:"detection_notes"`
}

func parseDescription(content string) (string, error) {
	decoder := json.NewDecoder(strings.NewReader(stripCodeQuoting(content)))
	decoder.DisallowUnknownFields()

	var response descriptionResponse
	if err := decoder.Decode(&response); err != nil {
		return "", errors.Wrap(err, "decode description JSON")
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return "", errors.New("description JSON has trailing content")
	}

	description := strings.TrimSpace(response.Description)
	if description == "" {
		return "", errors.New("description is empty")
	}

	return description, nil
}

// stripCodeQuoting removes a Markdown fenced code block wrapping the whole
// response.  Models sometimes answer with ```json ... ``` even when asked for
// bare JSON.  Anything outside the fence is preserved so unexpected text still
// fails parsing.
func stripCodeQuoting(content string) string {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "```") {
		return content
	}

	// The rest of the opening fence line is an optional language tag.
	_, rest, found := strings.Cut(trimmed[len("```"):], "\n")
	if !found {
		return content
	}

	body, tail, found := strings.Cut(rest, "```")
	if !found {
		return content
	}

	if strings.TrimSpace(tail) != "" {
		return content
	}

	return strings.TrimSpace(body)
}

func (worker *DescriptionWorker) Stop() {
	if worker.stop == nil {
		return
	}

	if worker.cancel != nil {
		worker.cancel()
	}

	close(worker.stop)
	<-worker.done

	worker.ctx = nil
	worker.cancel = nil
	worker.stop = nil
	worker.done = nil
}
