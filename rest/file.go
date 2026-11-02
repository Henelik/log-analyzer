package rest

import (
	"log"
	"strconv"
	"time"

	"github.com/Henelik/log-analyzer/db"
	"github.com/gofiber/fiber/v3"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type fileResponse struct {
	ID             uint          `json:"id"`
	Filename       string        `json:"filename"`
	Headers        string        `json:"headers"`
	Status         db.FileStatus `json:"status"`
	Lines          int32         `json:"lines"`
	AcceptedLines  int32         `json:"accepted_lines"`
	RejectedLines  int32         `json:"rejected_lines"`
	DuplicateLines int32         `json:"duplicate_lines"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type filesResponse struct {
	Files []fileResponse `json:"files"`
}

func newFileResponse(file db.File) fileResponse {
	return fileResponse{
		ID:             file.ID,
		Filename:       file.Filename,
		Headers:        file.Headers,
		Status:         file.Status,
		Lines:          file.Lines,
		AcceptedLines:  file.AcceptedLines,
		RejectedLines:  file.RejectedLines,
		DuplicateLines: file.DuplicateLines,
		CreatedAt:      file.CreatedAt,
		UpdatedAt:      file.UpdatedAt,
	}
}

func ListFiles(database *gorm.DB) fiber.Handler {
	return func(c fiber.Ctx) error {
		var files []db.File

		if err := database.Order("id").Find(&files).Error; err != nil {
			log.Printf("failed to list files: %v", err)

			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "cannot list files"})
		}

		response := filesResponse{Files: make([]fileResponse, 0, len(files))}
		for _, file := range files {
			response.Files = append(response.Files, newFileResponse(file))
		}

		return c.JSON(response)
	}
}

func GetFile(database *gorm.DB) fiber.Handler {
	return func(c fiber.Ctx) error {
		id, ok := fileID(c)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(uploadError{Error: "invalid file ID"})
		}

		var file db.File

		if err := database.First(&file, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return c.Status(fiber.StatusNotFound).JSON(uploadError{Error: "file not found"})
			}

			log.Printf("failed to get file %d: %v", id, err)

			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "cannot get file"})
		}

		return c.JSON(newFileResponse(file))
	}
}

func DeleteFile(database *gorm.DB) fiber.Handler {
	return func(c fiber.Ctx) error {
		id, ok := fileID(c)
		if !ok {
			return c.Status(fiber.StatusBadRequest).JSON(uploadError{Error: "invalid file ID"})
		}

		result := database.Delete(&db.File{}, id)
		if result.Error != nil {
			log.Printf("failed to delete file %d: %v", id, result.Error)

			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "cannot delete file"})
		}

		if result.RowsAffected == 0 {
			return c.Status(fiber.StatusNotFound).JSON(uploadError{Error: "file not found"})
		}

		return c.SendStatus(fiber.StatusNoContent)
	}
}

func fileID(c fiber.Ctx) (uint, bool) {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil {
		return 0, false
	}

	return uint(id), true
}
