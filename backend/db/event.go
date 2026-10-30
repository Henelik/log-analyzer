package db

import (
	"time"
)

type Event struct {
	ID uint `gorm:"primarykey"`

	RecordID         int
	Time             time.Time
	User             string
	Department       string
	Location         string
	ClientIP         string
	ServerIP         string
	URL              string
	URLCategory      string
	URLSuperCategory string
	Host             string
	Method           string
	Status           string
	UserAgent        string
	Action           string
	RuleType         string
	RuleLabel        string
	AppName          string
	AppClass         string
	ThreatName       string
	BytesIn          int
	BytesOut         int
	ProfileID        int
	IngestID         int
	RiskScore        uint32
	Severity         string

	RawLog string
	Error  string
}
