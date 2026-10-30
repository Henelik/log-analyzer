package db

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func DB(driver, dsn string) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch driver {
	case "postgres":
		dialector = postgres.Open(dsn)
	case "sqlite":
		dialector = sqlite.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported driver: %q", driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open %s database: %w", driver, err)
	}

	return db, nil
}

type Model struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
