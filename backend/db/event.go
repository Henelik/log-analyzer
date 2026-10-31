package db

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Henelik/log-analyzer/csvutil"
	"github.com/pkg/errors"
)

type Event struct {
	IngestID   uint  `gorm:"primaryKey"`
	LineNumber int32 `gorm:"primaryKey;autoIncrement:false"`

	Time         *time.Time
	User         string `gorm:"not null; default:''"`
	ClientIP     string `gorm:"not null; default:''"`
	ServerIP     string `gorm:"not null; default:''"`
	ServerPort   *int
	URL          string `gorm:"not null; default:''"`
	Method       string `gorm:"not null; default:''"`
	ResponseCode *int
	Action       string `gorm:"not null; default:''"`
	AppName      string `gorm:"not null; default:''"`
	BytesIn      *int
	BytesOut     *int
	RiskScore    *int
	Severity     string `gorm:"not null; default:''"`

	RawLog string `gorm:"not null"`
	Parsed bool   `gorm:"not null"`
	Errors string

	File *File `gorm:"foreignKey:IngestID;references:ID;constraint:OnDelete:CASCADE;not null" json:"-"`
}

// parse the event's RawLog, and fill in the event fields.  Records any errors encountered to the event Errors field.
func (event *Event) Parse(headers []string) {
	errs := []error{}

	record, err := csvutil.ParseRow(event.RawLog)
	if err != nil {
		errs = append(errs, errors.Wrap(err, "failed to parse epochtime"))
		event.finishParsing(errs)

		return
	}

	if idx := slices.Index(headers, "epochtime"); idx >= 0 {
		epoch, err := strconv.Atoi(record[idx])
		if err != nil {
			errs = append(errs, errors.Wrap(err, "failed to parse epochtime"))
		} else {
			epochTime := time.Unix(int64(epoch), 0)
			event.Time = &epochTime
		}
	}

	if idx := slices.Index(headers, "login"); idx >= 0 {
		if !isNone(record[idx]) {
			event.User = record[idx]
		}
	}

	if idx := slices.Index(headers, "cip"); idx >= 0 {
		event.ClientIP = record[idx]
	}

	if idx := slices.Index(headers, "sip"); idx >= 0 {
		event.ServerIP = record[idx]
	}

	if idx := slices.Index(headers, "srv_dport"); idx >= 0 {
		if !isNone(record[idx]) {
			port, err := strconv.Atoi(record[idx])
			if err != nil {
				errs = append(errs, errors.Wrap(err, "invalid srv_dport"))
			} else {
				event.ServerPort = &port
			}
		}
	}

	if idx := slices.Index(headers, "url"); idx >= 0 {
		event.URL = record[idx]
	}

	if idx := slices.Index(headers, "reqmethod"); idx >= 0 {
		event.Method = record[idx]
	}

	if idx := slices.Index(headers, "respcode"); idx >= 0 {
		if !isNone(record[idx]) {
			strippedCode, _, _ := strings.Cut(record[idx], " ")

			responseCode, err := strconv.Atoi(strippedCode)
			if err != nil {
				errs = append(errs, errors.Wrap(err, "invalid response code"))
			} else {
				event.ResponseCode = &responseCode
			}
		}
	}

	if idx := slices.Index(headers, "action"); idx >= 0 {
		if !isNone(record[idx]) {
			event.Action = record[idx]
		}
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
				errs = append(errs, errors.Wrap(err, "invalid reqsize"))
			} else {
				event.BytesIn = &bytesIn
			}
		}
	}

	if idx := slices.Index(headers, "respdatasize"); idx >= 0 {
		if !isNone(record[idx]) {
			bytesOut, err := strconv.Atoi(record[idx])
			if err != nil {
				errs = append(errs, errors.Wrap(err, "invalid respdatasize"))
			} else {
				event.BytesOut = &bytesOut
			}
		}
	}

	if idx := slices.Index(headers, "riskscore"); idx >= 0 {
		if !isNone(record[idx]) {
			riskScore, err := strconv.Atoi(record[idx])
			switch {
			case err != nil:
				errs = append(errs, errors.Wrap(err, "invalid riskscore"))
			case riskScore < 0 || riskScore > 100:
				errs = append(errs, errors.Errorf("invalid riskscore"))
			default:
				event.RiskScore = &riskScore
			}
		}
	}

	if idx := slices.Index(headers, "threatseverity"); idx >= 0 {
		event.Severity = record[idx]
	}

	event.finishParsing(errs)
}

// finishParsing marks the event as parsed and records any parse errors.
func (event *Event) finishParsing(errs []error) {
	event.Parsed = true

	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		messages = append(messages, err.Error())
	}

	event.Errors = strings.Join(messages, "; ")
}

func isNone(s string) bool {
	return slices.Contains([]string{
		"",
		"None",
		"NA",
		"N/A",
	}, s)
}
