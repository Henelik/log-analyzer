package rest

import (
	"log"

	"github.com/Henelik/log-analyzer/db"
	"github.com/gofiber/fiber/v3"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type detectionResponse struct {
	DetectionID uint    `json:"detection_id"`
	Description string  `json:"description"`
	Severity    int16   `json:"severity"`
	Confidence  int16   `json:"confidence"`
	Priority    int16   `json:"priority"`
	Entity      string  `json:"entity"`
	Observed    float32 `json:"observed"`
	Threshold   float32 `json:"threshold"`
	Events      []int32 `json:"events"`
	Notes       string  `json:"notes"`
}

type detectionsResponse struct {
	Detections []detectionResponse `json:"detections"`
}

func newDetectionResponse(detection db.Detection) detectionResponse {
	events := []int32(detection.Events)
	if events == nil {
		events = []int32{}
	}

	return detectionResponse{
		DetectionID: detection.DetectionID,
		Description: detection.Description,
		Severity:    detection.Severity,
		Confidence:  detection.Confidence,
		Priority:    detection.Priority,
		Entity:      detection.Entity,
		Observed:    detection.Observed,
		Threshold:   detection.Threshold,
		Events:      events,
		Notes:       detection.Notes,
	}
}

// GetDetections returns the file's detections ordered by detection ID.
func GetDetections(database *gorm.DB) fiber.Handler {
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

			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "cannot get detections"})
		}

		var detections []db.Detection

		if err := database.
			Where("ingest_id = ?", id).
			Order("detection_id").
			Find(&detections).Error; err != nil {
			log.Printf("failed to get detections for file %d: %v", id, err)

			return c.Status(fiber.StatusInternalServerError).JSON(uploadError{Error: "cannot get detections"})
		}

		response := detectionsResponse{Detections: make([]detectionResponse, 0, len(detections))}
		for _, detection := range detections {
			response.Detections = append(response.Detections, newDetectionResponse(detection))
		}

		return c.JSON(response)
	}
}
