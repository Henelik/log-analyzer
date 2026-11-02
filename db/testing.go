package db

import (
	"fmt"
	"sync/atomic"

	"gorm.io/gorm"
)

var testDBCount atomic.Uint64

// NewTestDB opens an isolated in-memory SQLite database for tests.
func NewTestDB() (*gorm.DB, error) {
	dsn := fmt.Sprintf("file:testdb%d?mode=memory&cache=shared", testDBCount.Add(1))

	return DB("sqlite", dsn)
}
