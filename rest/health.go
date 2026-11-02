package rest

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"
)

const readinessTimeout = 2 * time.Second

type healthResponse struct {
	Status string `json:"status"`
}

func Healthz(c fiber.Ctx) error {
	return c.JSON(healthResponse{Status: "ok"})
}

func Readyz(db *gorm.DB) fiber.Handler {
	return func(c fiber.Ctx) error {
		if db == nil {
			return c.JSON(healthResponse{Status: "ok"})
		}

		ctx, cancel := context.WithTimeout(c.Context(), readinessTimeout)
		defer cancel()

		sqlDB, err := db.DB()
		if err == nil {
			err = sqlDB.PingContext(ctx)
		}
		if err != nil {
			log.Printf("readiness check failed: %v", err)
			return c.Status(fiber.StatusServiceUnavailable).JSON(healthResponse{Status: "unavailable"})
		}

		return c.JSON(healthResponse{Status: "ok"})
	}
}
