package db

type Detection struct {
	IngestID    uint `gorm:"primaryKey"`
	DetectionID uint `gorm:"primaryKey"`

	Description string `gorm:"not null"`
	Severity    int16  `gorm:"not null"` // 0-100
	Confidence  int16  `gorm:"not null"` // 0-100
	Priority    int16  `gorm:"not null"` // 0-100
	Entity      string
	Observed    float32 `gorm:"not null"`
	Threshold   float32 `gorm:"not null"`
	Events      []int32 `gorm:"not null;type:integer[]"`
	Notes       string
}
