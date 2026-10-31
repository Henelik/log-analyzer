package db

import (
	"time"
)

type Event struct {
	IngestID   uint  `gorm:"primaryKey"`
	LineNumber int32 `gorm:"primaryKey;autoIncrement:false"`

	Time         time.Time
	User         string `gorn:"not null; default:''"`
	ClientIP     string `gorn:"not null; default:''"`
	ServerIP     string `gorn:"not null; default:''"`
	URL          string `gorn:"not null; default:''"`
	Method       string `gorn:"not null; default:''"`
	ResponseCode *int
	Action       string `gorn:"not null; default:''"`
	AppName      string `gorn:"not null; default:''"`
	BytesIn      *int
	BytesOut     *int
	RiskScore    *int
	Severity     string `gorn:"not null; default:''"`

	RawLog string `gorm:"not null"`

	File *File `gorm:"foreignKey:IngestID;references:ID;constraint:OnDelete:CASCADE;not null" json:"-"`
}
