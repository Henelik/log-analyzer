package worker

import (
	"log"
	"time"

	"github.com/Henelik/log-analyzer/db"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

const (
	detectionInsertBatchSize = 100

	defaultAnalysisInterval = 30 * time.Second
)

type AnalysisWorker struct {
	database *gorm.DB
	interval time.Duration

	stop chan struct{}
	done chan struct{}
}

func NewAnalysisWorker(database *gorm.DB) *AnalysisWorker {
	return &AnalysisWorker{database: database}
}

// start the analysis loop.  0 interval defaults to 30s
func (worker *AnalysisWorker) Start(interval time.Duration) {
	if worker.stop != nil {
		return
	}

	if interval <= 0 {
		interval = defaultAnalysisInterval
	}

	worker.interval = interval
	worker.stop = make(chan struct{})
	worker.done = make(chan struct{})

	go worker.loop()
}

func (worker *AnalysisWorker) loop() {
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

func (worker *AnalysisWorker) run() {
	var files []db.File

	if err := worker.database.
		Where("status = ?", db.FileStatusParsed).
		Order("id").
		Find(&files).Error; err != nil {
		log.Printf("analyzer: failed to query parsed files: %v", err)

		return
	}

	for _, file := range files {
		if err := worker.analyzeFile(file); err != nil {
			log.Printf("analyzer: failed to analyze file %d (%q): %v", file.ID, file.Filename, err)
		}
	}
}

func (worker *AnalysisWorker) analyzeFile(file db.File) error {
	detections, err := worker.detect(file)
	if err != nil {
		return err
	}

	if err := worker.saveDetections(file.ID, detections); err != nil {
		return err
	}

	return nil
}

// detect loads the file's accepted events and runs every detector over them.
func (worker *AnalysisWorker) detect(file db.File) ([]db.Detection, error) {
	var events []db.Event

	err := worker.database.
		Omit("RawLog", "Errors").
		Where("ingest_id = ? AND status = ?", file.ID, db.EventStatusParsed).
		Order(`"time", line_number`).
		Find(&events).Error
	if err != nil {
		return nil, errors.Wrap(err, "query events")
	}

	if len(events) == 0 {
		return nil, nil
	}

	detections := []db.Detection{}
	for _, detector := range Detectors {
		detections = append(detections, detector(events)...)
	}

	return detections, nil
}

// saveDetections inserts the detections and marks the file analyzed in a single transaction,
// so a failure leaves neither partial detections nor an analyzed file.
func (worker *AnalysisWorker) saveDetections(fileID uint, detections []db.Detection) error {
	err := worker.database.Transaction(func(tx *gorm.DB) error {
		var lastID uint

		err := tx.
			Model(&db.Detection{}).
			Where("ingest_id = ?", fileID).
			Select("COALESCE(MAX(detection_id), 0)").
			Scan(&lastID).Error
		if err != nil {
			return errors.Wrap(err, "query last detection ID")
		}

		for i := range detections {
			detections[i].IngestID = fileID
			detections[i].DetectionID = lastID + uint(i) + 1
		}

		if len(detections) > 0 {
			if err := tx.CreateInBatches(&detections, detectionInsertBatchSize).Error; err != nil {
				return errors.Wrap(err, "insert detections")
			}
		}

		err = tx.
			Model(&db.File{}).
			Where("id = ? AND status = ?", fileID, db.FileStatusParsed).
			Update("status", db.FileStatusAnalyzed).Error

		return errors.Wrap(err, "update file status")
	})

	return errors.Wrap(err, "failed to save detections")
}

func (worker *AnalysisWorker) Stop() {
	if worker.stop == nil {
		return
	}

	close(worker.stop)
	<-worker.done

	worker.stop = nil
	worker.done = nil
}
