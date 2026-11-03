package rest

import (
	"bufio"
	"crypto/sha256"
	"log"
	"slices"

	"github.com/Henelik/log-analyzer/csvutil"
	"github.com/Henelik/log-analyzer/db"
	"github.com/gofiber/fiber/v3"
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

		scanner := bufio.NewScanner(handle)

		// read the header
		scanner.Scan()
		headerString := scanner.Text()

		// parse the header to ensure the epochtime column exists and get the column count
		headers, err := csvutil.ParseRow(headerString)
		if err != nil {
			log.Printf("failed to read uploaded file %q: %v", header.Filename, err)
			return c.Status(fiber.StatusBadRequest).JSON(uploadError{Error: "cannot read uploaded file: malformed header"})
		}

		if !slices.Contains(headers, "epochtime") {
			log.Printf("failed to read uploaded file %q: %v", header.Filename, err)
			return c.Status(fiber.StatusBadRequest).JSON(uploadError{Error: "cannot read uploaded file: missing epochtime column"})
		}

		file := db.File{
			Filename: header.Filename,
			Headers:  headerString,
			Status:   db.FileStatusUplodaded,
		}

		if err := database.Transaction(func(tx *gorm.DB) error {
			batch := make([]db.Event, 0, uploadChunkSize)
			seen := make(map[[sha256.Size]byte]struct{}, uploadChunkSize)

			flush := func() error {
				if len(batch) == 0 {
					return nil
				}

				err := tx.CreateInBatches(batch, uploadChunkSize).Error

				batch = batch[:0]

				return err
			}

			// create the file record in the db
			if err := tx.Create(&file).Error; err != nil {
				return err
			}

			// the header was line 1
			var lineNum int32 = 2

			// create the events in the db, skipping rows already seen in this file
			for scanner.Scan() {
				raw := scanner.Text()

				hash := sha256.Sum256([]byte(raw))
				if _, found := seen[hash]; found {
					file.DuplicateLines++
					lineNum++
					continue
				}

				seen[hash] = struct{}{}

				batch = append(batch, db.Event{
					IngestID:   file.ID,
					LineNumber: lineNum,
					RawLog:     raw,
				})

				if len(batch) >= uploadChunkSize {
					if err := flush(); err != nil {
						return err
					}
				}

				lineNum++
			}

			if err := flush(); err != nil {
				return err
			}

			if err := scanner.Err(); err != nil {
				return err
			}

			// 2 extra lines counted: the header, and the extra increment at the end of the loop
			file.Lines = lineNum - 2

			return tx.Save(&file).Error
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
