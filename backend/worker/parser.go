package worker

import (
	"log"
	"time"

	"github.com/Henelik/log-analyzer/csvutil"
	"github.com/Henelik/log-analyzer/db"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	eventParseBatchSize = 100

	defaultParseInterval = 30 * time.Second
)

type EventParseWorker struct {
	database *gorm.DB
	interval time.Duration

	stop chan struct{}
	done chan struct{}
}

func NewEventParseWorker(database *gorm.DB) *EventParseWorker {
	return &EventParseWorker{database: database}
}

// start the parser loop.  0 interval defaults to 30s
func (worker *EventParseWorker) Start(interval time.Duration) {
	if worker.stop != nil {
		return
	}

	if interval <= 0 {
		interval = defaultParseInterval
	}

	worker.interval = interval
	worker.stop = make(chan struct{})
	worker.done = make(chan struct{})

	go worker.loop()
}

func (worker *EventParseWorker) loop() {
	defer close(worker.done)

	worker.run()

	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()

	for {
		select {
		case <-worker.stop:
			return
		case <-ticker.C:
			worker.run()
		}
	}
}

func (worker *EventParseWorker) run() {
	var files []db.File

	if err := worker.database.
		Where("status = ?", db.FileStatusUplodaded).
		Order("id").
		Find(&files).Error; err != nil {
		log.Printf("event parser: failed to query uploaded files: %v", err)

		return
	}

	for _, file := range files {
		if err := worker.parseFile(file); err != nil {
			log.Printf("event parser: failed to parse file %d (%q): %v", file.ID, file.Filename, err)
		}
	}
}

func (worker *EventParseWorker) parseFile(file db.File) error {
	headers, err := csvutil.ParseRow(file.Headers)
	if err != nil {
		return errors.Wrap(err, "failed to parse file headers")
	}

	var lastLine int32

	for {
		batch, err := worker.loadEventBatch(file.ID, lastLine)
		if err != nil {
			return err
		}

		if len(batch) == 0 {
			break
		}

		for i := range batch {
			batch[i].Parse(headers)
		}

		if err := worker.database.Omit(clause.Associations).Save(&batch).Error; err != nil {
			return errors.Wrap(err, "failed to save parsed events")
		}

		lastLine = batch[len(batch)-1].LineNumber
	}

	return worker.finishFile(file.ID)
}

func (worker *EventParseWorker) loadEventBatch(fileID uint, afterLine int32) ([]db.Event, error) {
	batch := make([]db.Event, 0, eventParseBatchSize)

	err := worker.database.
		Where("ingest_id = ? AND line_number > ? AND status = ?", fileID, afterLine, db.EventStatusUnparsed).
		Order("line_number").
		Limit(eventParseBatchSize).
		Find(&batch).Error

	return batch, errors.Wrap(err, "failed to query event batch")
}

func (worker *EventParseWorker) finishFile(fileID uint) error {
	var counts []eventCount

	err := worker.database.
		Model(&db.Event{}).
		Select("status, count(*) AS total").
		Where("ingest_id = ?", fileID).
		Group("status").
		Scan(&counts).Error
	if err != nil {
		return errors.Wrap(err, "count events")
	}

	var lines, accepted, rejected, unparsed int32

	for _, count := range counts {
		total := int32(count.Total)

		lines += total

		switch count.Status {
		case db.EventStatusParsed:
			accepted = total
		case db.EventStatusRejected:
			rejected = total
		case db.EventStatusUnparsed:
			unparsed = total
		}
	}

	if unparsed > 0 {
		return errors.Errorf("%d events left unparsed", unparsed)
	}

	err = worker.database.
		Model(&db.File{}).
		Where("id = ? AND status = ?", fileID, db.FileStatusUplodaded).
		Updates(map[string]any{
			"status":         db.FileStatusParsed,
			"lines":          lines,
			"accepted_lines": accepted,
			"rejected_lines": rejected,
		}).Error

	return errors.Wrap(err, "update file status")
}

func (worker *EventParseWorker) Stop() {
	if worker.stop == nil {
		return
	}

	close(worker.stop)
	<-worker.done

	worker.stop = nil
	worker.done = nil
}

type eventCount struct {
	Status db.EventStatus
	Total  int64
}
