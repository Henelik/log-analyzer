package worker

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Henelik/log-analyzer/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	database, err := db.NewTestDB()
	require.NoError(t, err, "open test database")
	require.NoError(t, database.AutoMigrate(&db.File{}, &db.Event{}), "migrate test database")

	return database
}

func seedFile(t *testing.T, database *gorm.DB, headers string, lines []string) db.File {
	t.Helper()

	file := db.File{
		Filename: "access.csv",
		Headers:  headers,
		Status:   db.FileStatusUplodaded,
	}

	require.NoError(t, database.Create(&file).Error, "seed file")

	events := make([]db.Event, 0, len(lines))
	for i, line := range lines {
		events = append(events, db.Event{
			IngestID:   file.ID,
			LineNumber: int32(i + 2),
			RawLog:     line,
		})
	}

	if len(events) > 0 {
		require.NoError(t, database.CreateInBatches(&events, 100).Error, "seed events")
	}

	return file
}

func loadFile(t *testing.T, database *gorm.DB, id uint) db.File {
	t.Helper()

	var file db.File
	require.NoError(t, database.First(&file, id).Error, "load file")

	return file
}

func loadFileEvents(t *testing.T, database *gorm.DB, fileID uint) []db.Event {
	t.Helper()

	var events []db.Event
	require.NoError(t, database.Where("ingest_id = ?", fileID).Order("line_number").Find(&events).Error, "load file events")

	return events
}

func TestEventParseWorkerParsesUploadedFile(t *testing.T) {
	database := openTestDB(t)

	headers := "epochtime,login,cip,respcode,action"
	lines := []string{
		"1700000000,alice,10.0.0.1,200,Login Success",
		"1700000001,bob,10.0.0.2,404,Login Fail",
		"not-a-number,carol,10.0.0.3,500,Login Fail",
	}

	file := seedFile(t, database, headers, lines)

	worker := NewEventParseWorker(database)
	worker.run()

	stored := loadFile(t, database, file.ID)

	assert.Equal(t, db.FileStatus(db.FileStatusParsed), stored.Status)

	assert.Equal(t, int32(len(lines)), stored.Lines)
	assert.Equal(t, int32(2), stored.AcceptedLines)
	assert.Equal(t, int32(1), stored.RejectedLines)
	assert.Zero(t, stored.DuplicateLines)

	events := loadFileEvents(t, database, file.ID)
	require.Len(t, events, len(lines))

	assert.Equal(t, db.EventStatusParsed, events[0].Status)
	assert.True(t, events[0].Time.Equal(time.Unix(1700000000, 0)), "time %s", events[0].Time)
	assert.Equal(t, "alice", events[0].User)
	assert.Equal(t, "10.0.0.1", events[0].ClientIP)
	require.NotNil(t, events[0].ResponseCode)
	assert.Equal(t, 200, *events[0].ResponseCode)
	assert.Equal(t, "Login Success", events[0].Action)
	assert.Empty(t, events[0].Errors)

	assert.Equal(t, lines[0], events[0].RawLog, "raw log is preserved")
	assert.Equal(t, int32(2), events[0].LineNumber, "line number is preserved")

	assert.Equal(t, db.EventStatusRejected, events[2].Status)
	assert.Contains(t, events[2].Errors, "failed to parse epochtime")
}

func TestEventParseWorkerParsesAllBatches(t *testing.T) {
	database := openTestDB(t)

	const rows = 2*eventParseBatchSize + 1

	lines := make([]string, 0, rows)
	for i := range rows {
		lines = append(lines, fmt.Sprintf("%d", 1700000000+i))
	}

	file := seedFile(t, database, "epochtime", lines)

	worker := NewEventParseWorker(database)
	worker.run()

	stored := loadFile(t, database, file.ID)
	assert.Equal(t, db.FileStatus(db.FileStatusParsed), stored.Status)
	assert.Equal(t, int32(rows), stored.Lines)
	assert.Equal(t, int32(rows), stored.AcceptedLines)
	assert.Zero(t, stored.RejectedLines)

	events := loadFileEvents(t, database, file.ID)
	require.Len(t, events, rows)

	for i, event := range events {
		assert.Equal(t, db.EventStatusParsed, event.Status, "line %d", event.LineNumber)
		assert.True(t, event.Time.Equal(time.Unix(int64(1700000000+i), 0)), "line %d time %s", event.LineNumber, event.Time)
	}
}

func TestEventParseWorkerSkipsFilesNotUploaded(t *testing.T) {
	database := openTestDB(t)

	statuses := []db.FileStatus{
		db.FileStatusParsed,
		db.FileStatusAnalyzed,
	}

	files := make([]db.File, 0, len(statuses))
	for i, status := range statuses {
		file := db.File{
			Filename: fmt.Sprintf("file-%d.csv", i),
			Headers:  "epochtime",
			Status:   status,
		}

		require.NoError(t, database.Create(&file).Error, "seed file %d", i)
		require.NoError(t, database.Create(&db.Event{
			IngestID:   file.ID,
			LineNumber: 2,
			RawLog:     "1700000000",
		}).Error, "seed event %d", i)

		files = append(files, file)
	}

	worker := NewEventParseWorker(database)
	worker.run()

	for i, file := range files {
		stored := loadFile(t, database, file.ID)
		assert.Equal(t, statuses[i], stored.Status, "file %d status", i)

		events := loadFileEvents(t, database, file.ID)
		require.Len(t, events, 1)
		assert.Equal(t, db.EventStatusUnparsed, events[0].Status, "file %d event status", i)
		assert.True(t, events[0].Time.IsZero(), "file %d event time %s", i, events[0].Time)
	}
}

func TestEventParseWorkerHandlesEmptyFile(t *testing.T) {
	database := openTestDB(t)

	file := seedFile(t, database, "epochtime", nil)

	worker := NewEventParseWorker(database)
	worker.run()

	stored := loadFile(t, database, file.ID)
	assert.Equal(t, db.FileStatus(db.FileStatusParsed), stored.Status)
	assert.Zero(t, stored.Lines)
	assert.Zero(t, stored.AcceptedLines)
	assert.Zero(t, stored.RejectedLines)
}

func TestEventParseWorkerPreservesUnparsableHeadersAsRejected(t *testing.T) {
	database := openTestDB(t)

	file := seedFile(t, database, "epochtime", []string{"1700000000"})

	worker := NewEventParseWorker(database)
	worker.run()
	assert.Equal(t, db.FileStatus(db.FileStatusParsed), loadFile(t, database, file.ID).Status)

	badFile := seedFile(t, database, `"epochtime`, []string{"1700000000"})

	worker.run()

	stored := loadFile(t, database, badFile.ID)
	assert.Equal(t, db.FileStatus(db.FileStatusUplodaded), stored.Status, "file with unreadable header is not finished")
}

func TestEventParseWorkerStartStop(t *testing.T) {
	database := openTestDB(t)

	worker := NewEventParseWorker(database)
	worker.Start(time.Hour)

	worker.Stop()
	worker.Stop()
}

func TestEventParseWorkerRunIsIdempotent(t *testing.T) {
	database := openTestDB(t)

	headers := "epochtime,login"
	lines := []string{"1700000000,alice", "bad,bob"}

	file := seedFile(t, database, headers, lines)

	worker := NewEventParseWorker(database)
	worker.run()

	first := loadFileEvents(t, database, file.ID)

	worker.run()

	second := loadFileEvents(t, database, file.ID)

	require.Len(t, second, len(first))
	for i := range first {
		assert.Equal(t, first[i].Status, second[i].Status)
		assert.Equal(t, first[i].User, second[i].User)
		assert.Equal(t, first[i].Errors, second[i].Errors)
	}

	stored := loadFile(t, database, file.ID)
	assert.Equal(t, db.FileStatus(db.FileStatusParsed), stored.Status)
	assert.Equal(t, int32(len(lines)), stored.Lines)
	assert.Equal(t, int32(1), stored.AcceptedLines)
	assert.Equal(t, int32(1), stored.RejectedLines)
}

func TestEventParseWorkerQuotedRawLog(t *testing.T) {
	database := openTestDB(t)

	record := `1700000000,alice,"/search?q=a,b"`

	file := seedFile(t, database, "epochtime,login,url", []string{record})

	worker := NewEventParseWorker(database)
	worker.run()

	events := loadFileEvents(t, database, file.ID)
	require.Len(t, events, 1)

	assert.Equal(t, db.EventStatusParsed, events[0].Status)
	assert.Equal(t, "alice", events[0].User)
	assert.Equal(t, "/search?q=a,b", events[0].URL)
	assert.True(t, strings.Contains(events[0].RawLog, `"`), "raw log %q", events[0].RawLog)
}
