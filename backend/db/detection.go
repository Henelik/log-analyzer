package db

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
)

type Detection struct {
	IngestID    uint `gorm:"primaryKey"`
	DetectionID uint `gorm:"primaryKey"`

	Description string `gorm:"not null"`
	Severity    int16  `gorm:"not null"` // 0-100
	Confidence  int16  `gorm:"not null"` // 0-100
	Priority    int16  `gorm:"not null"` // 0-100
	Entity      string
	Observed    float32    `gorm:"not null"`
	Threshold   float32    `gorm:"not null"`
	Events      Int32Array `gorm:"not null;type:integer[]"`
	Notes       string
}

// Int32Array is a []int32 that round-trips through a PostgreSQL integer array,
// and falls back to its text representation on other drivers.
type Int32Array []int32

func (array Int32Array) Value() (driver.Value, error) {
	parts := make([]string, len(array))
	for i, value := range array {
		parts[i] = strconv.FormatInt(int64(value), 10)
	}

	return "{" + strings.Join(parts, ",") + "}", nil
}

func (array *Int32Array) Scan(src any) error {
	if src == nil {
		*array = nil

		return nil
	}

	var text string

	switch value := src.(type) {
	case string:
		text = value
	case []byte:
		text = string(value)
	default:
		return fmt.Errorf("cannot scan %T into Int32Array", src)
	}

	text = strings.TrimPrefix(text, "{")
	text = strings.TrimSuffix(text, "}")

	scanned := make(Int32Array, 0)

	if trimmed := strings.TrimSpace(text); trimmed != "" {
		for _, part := range strings.Split(trimmed, ",") {
			parsed, err := strconv.ParseInt(strings.TrimSpace(part), 10, 32)
			if err != nil {
				return fmt.Errorf("parse int32 array element %q: %w", part, err)
			}

			scanned = append(scanned, int32(parsed))
		}
	}

	*array = scanned

	return nil
}
