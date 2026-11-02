package worker

import (
	"fmt"
	"testing"
	"time"

	"github.com/Henelik/log-analyzer/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openAnalysisTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	database, err := db.NewTestDB()
	require.NoError(t, err, "open test database")
	require.NoError(t, database.AutoMigrate(&db.File{}, &db.Event{}, &db.Detection{}), "migrate test database")

	return database
}

func seedParsedFile(t *testing.T, database *gorm.DB, status db.FileStatus, events []db.Event) db.File {
	t.Helper()

	file := db.File{
		Filename: "access.csv",
		Headers:  "epochtime,login,action",
		Status:   status,
	}

	require.NoError(t, database.Create(&file).Error, "seed file")

	for i := range events {
		events[i].IngestID = file.ID
	}

	if len(events) > 0 {
		require.NoError(t, database.CreateInBatches(&events, 100).Error, "seed events")
	}

	return file
}

func loginEvents(user string, action string, count int, status db.EventStatus) []db.Event {
	events := make([]db.Event, count)
	for i := range events {
		events[i] = db.Event{
			LineNumber: int32(i + 2),
			Time:       eventsEpoch.Add(time.Duration(i) * time.Second),
			User:       user,
			Action:     action,
			RawLog:     "raw",
			Status:     status,
		}
	}

	return events
}

func loadDetections(t *testing.T, database *gorm.DB, fileID uint) []db.Detection {
	t.Helper()

	var detections []db.Detection
	require.NoError(t, database.Where("ingest_id = ?", fileID).Order("detection_id").Find(&detections).Error, "load detections")

	return detections
}

func TestAnalysisWorkerDetectsBruteForce(t *testing.T) {
	database := openAnalysisTestDB(t)

	const attempts = 12

	file := seedParsedFile(t, database, db.FileStatusParsed, loginEvents("alice", ActionLoginFail, attempts, db.EventStatusParsed))

	worker := NewAnalysisWorker(database)
	worker.run()

	assert.Equal(t, db.FileStatus(db.FileStatusAnalyzed), loadFile(t, database, file.ID).Status)

	detections := loadDetections(t, database, file.ID)
	require.Len(t, detections, 1)

	detection := detections[0]

	assert.Equal(t, file.ID, detection.IngestID)
	assert.Equal(t, uint(1), detection.DetectionID)
	assert.Equal(t, "Brute Force Attempt", detection.Description)
	assert.Equal(t, "alice", detection.Entity)
	assert.Equal(t, float32(attempts), detection.Observed)
	assert.Equal(t, float32(bruteForceThresh), detection.Threshold)
	assert.Equal(t, []int32{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}, []int32(detection.Events))

	assert.Equal(t, int16(100), detection.Severity, "12 attempts over 11s is well above the rate threshold")
	assert.Equal(t, int16(51), detection.Confidence, "30 + 20*log10(12)")
}

func TestAnalysisWorkerWritesDetectionIDsPerFile(t *testing.T) {
	database := openAnalysisTestDB(t)

	first := seedParsedFile(t, database, db.FileStatusParsed, loginEvents("alice", ActionLoginFail, 12, db.EventStatusParsed))
	second := seedParsedFile(t, database, db.FileStatusParsed, loginEvents("bob", ActionLoginFail, 12, db.EventStatusParsed))

	worker := NewAnalysisWorker(database)
	worker.run()

	for _, file := range []db.File{first, second} {
		detections := loadDetections(t, database, file.ID)
		require.Len(t, detections, 1)
		assert.Equal(t, uint(1), detections[0].DetectionID, "detection IDs restart per ingest")
		assert.Equal(t, file.ID, detections[0].IngestID)
	}
}

func TestAnalysisWorkerNoDetectionBelowThreshold(t *testing.T) {
	database := openAnalysisTestDB(t)

	file := seedParsedFile(t, database, db.FileStatusParsed, loginEvents("alice", ActionLoginFail, bruteForceThresh, db.EventStatusParsed))

	worker := NewAnalysisWorker(database)
	worker.run()

	assert.Equal(t, db.FileStatus(db.FileStatusAnalyzed), loadFile(t, database, file.ID).Status)
	assert.Empty(t, loadDetections(t, database, file.ID), "threshold failures are not detected")
}

func TestAnalysisWorkerBucketsResetOnSuccess(t *testing.T) {
	database := openAnalysisTestDB(t)

	events := loginEvents("alice", ActionLoginFail, 6, db.EventStatusParsed)
	success := loginEvents("alice", ActionLoginSuccess, 1, db.EventStatusParsed)[0]
	success.LineNumber = 8
	success.Time = events[5].Time.Add(time.Second)

	events = append(events, success)
	events = append(events, loginEvents("alice", ActionLoginFail, 6, db.EventStatusParsed)...)

	for i := len(events) - 6; i < len(events); i++ {
		events[i].LineNumber = int32(i + 2)
		events[i].Time = success.Time.Add(time.Duration(i-len(events)+6) * time.Second)
	}

	file := seedParsedFile(t, database, db.FileStatusParsed, events)

	worker := NewAnalysisWorker(database)
	worker.run()

	assert.Empty(t, loadDetections(t, database, file.ID), "two failure buckets of six are below the threshold")
}

func TestAnalysisWorkerIgnoresUnparsedAndRejectedEvents(t *testing.T) {
	database := openAnalysisTestDB(t)

	events := loginEvents("alice", ActionLoginFail, 12, db.EventStatusRejected)
	events[0].Status = db.EventStatusUnparsed

	file := seedParsedFile(t, database, db.FileStatusParsed, events)

	worker := NewAnalysisWorker(database)
	worker.run()

	assert.Equal(t, db.FileStatus(db.FileStatusAnalyzed), loadFile(t, database, file.ID).Status)
	assert.Empty(t, loadDetections(t, database, file.ID), "only accepted events are analyzed")
}

func TestAnalysisWorkerSkipsFilesNotParsed(t *testing.T) {
	database := openAnalysisTestDB(t)

	statuses := []db.FileStatus{
		db.FileStatusUplodaded,
		db.FileStatusAnalyzed,
	}

	files := make([]db.File, 0, len(statuses))
	for i, status := range statuses {
		file := seedParsedFile(t, database, status, loginEvents(fmt.Sprintf("user%d", i), ActionLoginFail, 12, db.EventStatusParsed))
		files = append(files, file)
	}

	worker := NewAnalysisWorker(database)
	worker.run()

	for i, file := range files {
		assert.Equal(t, statuses[i], loadFile(t, database, file.ID).Status, "file %d status", i)
		assert.Empty(t, loadDetections(t, database, file.ID), "file %d detections", i)
	}
}

func TestAnalysisWorkerHandlesFileWithoutEvents(t *testing.T) {
	database := openAnalysisTestDB(t)

	file := seedParsedFile(t, database, db.FileStatusParsed, nil)

	worker := NewAnalysisWorker(database)
	worker.run()

	assert.Equal(t, db.FileStatus(db.FileStatusAnalyzed), loadFile(t, database, file.ID).Status)
	assert.Empty(t, loadDetections(t, database, file.ID))
}

func TestAnalysisWorkerRunIsIdempotent(t *testing.T) {
	database := openAnalysisTestDB(t)

	file := seedParsedFile(t, database, db.FileStatusParsed, loginEvents("alice", ActionLoginFail, 12, db.EventStatusParsed))

	worker := NewAnalysisWorker(database)
	worker.run()
	worker.run()

	assert.Equal(t, db.FileStatus(db.FileStatusAnalyzed), loadFile(t, database, file.ID).Status)
	assert.Len(t, loadDetections(t, database, file.ID), 1, "repeated runs do not duplicate detections")
}

func TestAnalysisWorkerStartStop(t *testing.T) {
	database := openAnalysisTestDB(t)

	seedParsedFile(t, database, db.FileStatusParsed, loginEvents("alice", ActionLoginFail, 12, db.EventStatusParsed))

	worker := NewAnalysisWorker(database)
	worker.Start(50 * time.Millisecond)

	require.Eventually(t, func() bool {
		return len(loadDetections(t, database, 1)) == 1
	}, 5*time.Second, 10*time.Millisecond, "worker analyzes on start")

	worker.Stop()
	worker.Stop()
}

func TestInt32ArrayValueAndScan(t *testing.T) {
	array := db.Int32Array{1, 2, 3}

	value, err := array.Value()
	require.NoError(t, err)
	assert.Equal(t, "{1,2,3}", value)

	var scanned db.Int32Array
	require.NoError(t, scanned.Scan(value))
	assert.Equal(t, array, scanned)

	require.NoError(t, scanned.Scan([]byte("{7}")))
	assert.Equal(t, db.Int32Array{7}, scanned)

	require.NoError(t, scanned.Scan("{}"))
	assert.Empty(t, scanned)

	require.NoError(t, scanned.Scan(nil))
	assert.Nil(t, scanned)

	assert.Error(t, scanned.Scan(12))
}
