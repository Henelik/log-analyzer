package rest

import (
	"encoding/json"
	"log"
	"time"

	"github.com/Henelik/log-analyzer/db"
	"github.com/gofiber/fiber/v3"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

const eventQueryChunkSize = 500

type eventResponse struct {
	LineNumber   int32          `json:"line_number"`
	Time         *time.Time     `json:"time"`
	User         string         `json:"user"`
	ClientIP     string         `json:"client_ip"`
	ServerIP     string         `json:"server_ip"`
	ServerPort   *int           `json:"server_port"`
	URL          string         `json:"url"`
	Method       string         `json:"method"`
	ResponseCode *int           `json:"response_code"`
	Action       string         `json:"action"`
	AppName      string         `json:"app_name"`
	BytesIn      *int           `json:"bytes_in"`
	BytesOut     *int           `json:"bytes_out"`
	RawLog       string         `json:"raw_log"`
	Status       db.EventStatus `json:"status"`
	Errors       string         `json:"errors"`
}

type eventsResponse struct {
	Events []eventResponse `json:"events"`
}

type eventErrorResponse struct {
	LineNumber int32  `json:"line_number"`
	RawLog     string `json:"raw_log"`
	Error      string `json:"error"`
}

type eventErrorsResponse struct {
	Errors []eventErrorResponse `json:"errors"`
}

func newEventResponse(event db.Event) eventResponse {
	response := eventResponse{
		LineNumber:   event.LineNumber,
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
		RawLog:       event.RawLog,
		Status:       event.Status,
		Errors:       event.Errors,
	}

	if !event.Time.IsZero() {
		response.Time = &event.Time
	}

	return response
}

// GetEvents returns the file's events for the line numbers submitted as a JSON list.
// Line numbers without a matching event are omitted.  The response preserves the
// submitted order and contains each event once.
func GetEvents(database *gorm.DB) fiber.Handler {
	return func(c fiber.Ctx) error {
		id, ok := fileID(c)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(uploadError{Error: "invalid file ID"})
		}

		var lineNumbers []int32

		if err := json.Unmarshal(c.Body(), &lineNumbers); err != nil || lineNumbers == nil {
			return c.Status(fiber.StatusBadRequest).JSON(uploadError{Error: "invalid line numbers"})
		}

		for _, lineNumber := range lineNumbers {
			if lineNumber < 1 {
				return c.Status(fiber.StatusBadRequest).JSON(uploadError{Error: "invalid line numbers"})
			}
		}

		lineNumbers = uniqueLineNumbers(lineNumbers)

		var file db.File

		if err := database.Select("id").First(&file, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(uploadError{Error: "file not found"})
			}

			log.Printf("failed to get file %d: %v", id, err)

			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "cannot get events"})
		}

		response := eventsResponse{Events: make([]eventResponse, 0, len(lineNumbers))}

		if len(lineNumbers) == 0 {
			return c.JSON(response)
		}

		events, err := loadEventsByLineNumber(database, id, lineNumbers)
		if err != nil {
			log.Printf("failed to get events for file %d: %v", id, err)

			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "cannot get events"})
		}

		byLineNumber := make(map[int32]db.Event, len(events))
		for _, event := range events {
			byLineNumber[event.LineNumber] = event
		}

		for _, lineNumber := range lineNumbers {
			if event, found := byLineNumber[lineNumber]; found {
				response.Events = append(response.Events, newEventResponse(event))
			}
		}

		return c.JSON(response)
	}
}

// GetEventErrors returns the line number, raw log, and error message for every
// rejected event belonging to the file, ordered by line number.
func GetEventErrors(database *gorm.DB) fiber.Handler {
	return func(c fiber.Ctx) error {
		id, ok := fileID(c)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(uploadError{Error: "invalid file ID"})
		}

		var file db.File

		if err := database.Select("id").First(&file, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(uploadError{Error: "file not found"})
			}

			log.Printf("failed to get file %d: %v", id, err)

			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "cannot get event errors"})
		}

		var events []db.Event

		err := database.
			Select("line_number", "raw_log", "errors").
			Where("ingest_id = ? AND status = ?", id, db.EventStatusRejected).
			Order("line_number").
			Find(&events).Error
		if err != nil {
			log.Printf("failed to get event errors for file %d: %v", id, err)

			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "cannot get event errors"})
		}

		response := eventErrorsResponse{Errors: make([]eventErrorResponse, 0, len(events))}
		for _, event := range events {
			response.Errors = append(response.Errors, eventErrorResponse{
				LineNumber: event.LineNumber,
				RawLog:     event.RawLog,
				Error:      event.Errors,
			})
		}

		return c.JSON(response)
	}
}

// uniqueLineNumbers removes duplicates while preserving the submitted order.
func uniqueLineNumbers(lineNumbers []int32) []int32 {
	seen := make(map[int32]struct{}, len(lineNumbers))
	unique := make([]int32, 0, len(lineNumbers))

	for _, lineNumber := range lineNumbers {
		if _, found := seen[lineNumber]; found {
			continue
		}

		seen[lineNumber] = struct{}{}
		unique = append(unique, lineNumber)
	}

	return unique
}

// loadEventsByLineNumber queries the requested events in chunks to stay within the
// database driver's bind parameter limit.
func loadEventsByLineNumber(database *gorm.DB, fileID uint, lineNumbers []int32) ([]db.Event, error) {
	events := make([]db.Event, 0, len(lineNumbers))

	for start := 0; start < len(lineNumbers); start += eventQueryChunkSize {
		end := min(start+eventQueryChunkSize, len(lineNumbers))

		var batch []db.Event

		err := database.
			Where("ingest_id = ? AND line_number IN ?", fileID, lineNumbers[start:end]).
			Find(&batch).Error
		if err != nil {
			return nil, errors.Wrap(err, "query events")
		}

		events = append(events, batch...)
	}

	return events, nil
}
