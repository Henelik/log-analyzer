package rest

import (
	"encoding/csv"
	"io"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Henelik/log-analyzer/db"
	"github.com/gofiber/fiber/v3"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

const (
	uploadFileField = "file"
	uploadChunkSize = 1000
)

type uploadResponse struct {
	ID       uint          `json:"id"`
	Filename string        `json:"filename"`
	Bytes    uint          `json:"bytes"`
	Status   db.FileStatus `json:"status"`
}

type uploadError struct {
	Error string `json:"error"`
}

func UploadFile(database *gorm.DB) fiber.Handler {
	return func(c fiber.Ctx) error {
		header, err := c.FormFile(uploadFileField)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(uploadError{Error: "missing CSV file"})
		}

		handle, err := header.Open()
		if err != nil {
			log.Printf("failed to open uploaded file %q: %v", header.Filename, err)
			return c.Status(fiber.StatusBadRequest).JSON(uploadError{Error: "cannot read uploaded file"})
		}
		defer func() {
			if err := handle.Close(); err != nil {
				log.Printf("close uploaded file %q: %v", header.Filename, err)
			}
		}()

		reader := newBatchCSVReader(handle)

		headers, err := reader.Read()
		if err != nil {
			log.Printf("failed to read uploaded file %q: %v", header.Filename, err)
			return c.Status(fiber.StatusBadRequest).JSON(uploadError{Error: "uploaded file is invalid"})
		}

		timeIdx := slices.Index(headers, "time")
		if timeIdx == -1 {
			log.Printf("failed to read uploaded file %q: %v", header.Filename, err)
			return c.Status(fiber.StatusBadRequest).JSON(uploadError{Error: "cannot read uploaded file: missing time column"})
		}

		file := db.File{
			Filename: header.Filename,
			Headers:  strings.Join(headers, ","),
			Status:   db.FileStatusUplodaded,
		}

		if err := database.Transaction(func(tx *gorm.DB) error {
			batch := make([]db.Event, 0, uploadChunkSize)
			rowsInChunk := 0

			flush := func() error {
				if len(batch) == 0 {
					return nil
				}

				err := tx.CreateInBatches(batch, uploadChunkSize).Error

				batch = batch[:0]

				return err
			}

			// create the file record in the db
			if err := database.Create(&file).Error; err != nil {
				return err
			}

			// the header was line 1
			var lineNum int32 = 2

			// create the events in the db
			for {
				record, readErr := reader.Read()
				switch {
				case errors.Is(readErr, io.EOF):
					return flush()
				case readErr != nil:
					return errors.Wrapf(readErr, "invalid CSV at line %d", lineNum)
				}

				event, err := parseCSVEvent(headers, record, lineNum, timeIdx)
				if err != nil {
					return err
				}

				batch = append(batch, event)
				rowsInChunk++

				lineNum++
			}
		}); err != nil {
			log.Printf("failed to store uploaded file %q: %v", header.Filename, err)

			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "cannot store file"})
		}

		return c.Status(fiber.StatusCreated).JSON(uploadResponse{
			ID:       file.ID,
			Filename: file.Filename,
			Status:   file.Status,
		})
	}
}

func newBatchCSVReader(r io.Reader) *csv.Reader {
	reader := csv.NewReader(r)
	reader.ReuseRecord = true

	// enforce all rows have the same number of records as the header
	reader.FieldsPerRecord = 0

	return reader
}

func parseCSVEvent(headers []string, record []string, lineNum int32, timeIdx int) (db.Event, error) {
	epoch, err := strconv.Atoi(record[timeIdx])
	if err != nil {
		return db.Event{}, errors.Wrapf(err, "invalid event time on line %d: %s", lineNum, record[timeIdx])
	}

	event := db.Event{
		LineNumber: lineNum,
		Time:       time.Unix(int64(epoch), 0),
		RawLog:     strings.Join(record, ","),
	}

	if idx := slices.Index(headers, "login"); idx >= 0 {
		event.User = record[idx]
	}

	if idx := slices.Index(headers, "cip"); idx >= 0 {
		event.ClientIP = record[idx]
	}

	if idx := slices.Index(headers, "sip"); idx >= 0 {
		event.ServerIP = record[idx]
	}

	if idx := slices.Index(headers, "url"); idx >= 0 {
		event.URL = record[idx]
	}

	if idx := slices.Index(headers, "reqmethod"); idx >= 0 {
		event.Method = record[idx]
	}

	if idx := slices.Index(headers, "respcode"); idx >= 0 {
		if !isNone(record[idx]) {
			responseCode, err := strconv.Atoi(record[idx])
			if err != nil {
				return db.Event{}, errors.Wrapf(err, "invalid response code on line %d: %s", lineNum, record[timeIdx])
			}

			event.ResponseCode = &responseCode
		}
	}

	if idx := slices.Index(headers, "action"); idx >= 0 {
		event.Action = record[idx]
	}

	if idx := slices.Index(headers, "appname"); idx >= 0 {
		event.AppName = record[idx]
	}

	if idx := slices.Index(headers, "appname"); idx >= 0 {
		event.AppName = record[idx]
	}

	if idx := slices.Index(headers, "reqsize"); idx >= 0 {
		if !isNone(record[idx]) {
			bytesIn, err := strconv.Atoi(record[idx])
			if err != nil {
				return db.Event{}, errors.Wrapf(err, "invalid reqsize on line %d: %s", lineNum, record[timeIdx])
			}

			event.BytesIn = &bytesIn
		}
	}

	if idx := slices.Index(headers, "respdatasize"); idx >= 0 {
		if !isNone(record[idx]) {
			bytesOut, err := strconv.Atoi(record[idx])
			if err != nil {
				return db.Event{}, errors.Wrapf(err, "invalid respsize on line %d: %s", lineNum, record[timeIdx])
			}

			event.BytesOut = &bytesOut
		}
	}

	if idx := slices.Index(headers, "riskscore"); idx >= 0 {
		if !isNone(record[idx]) {
			riskScore, err := strconv.Atoi(record[idx])
			if err != nil || riskScore < 0 || riskScore > 100 {
				return db.Event{}, errors.Wrapf(err, "invalid riskscore on line %d: %s", lineNum, record[timeIdx])
			}

			event.RiskScore = &riskScore
		}
	}

	if idx := slices.Index(headers, "threatseverity"); idx >= 0 {
		event.Severity = record[idx]
	}

	return event, nil
}

func isNone(s string) bool {
	return slices.Contains([]string{
		"",
		"None",
		"NA",
		"N/A",
	}, s)
}
