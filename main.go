package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Henelik/log-analyzer/db"
	"github.com/Henelik/log-analyzer/rest"
	"github.com/Henelik/log-analyzer/ui"
	"github.com/Henelik/log-analyzer/worker"
	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"
)

const (
	parseInterval    = 30 * time.Second
	analysisInterval = 30 * time.Second

	seedUsernameEnv = "SEED_USERNAME"
	seedPasswordEnv = "SEED_PASSWORD"
)

func main() {
	var (
		driver = os.Getenv("DB_DRIVER")
		dsn    = os.Getenv("DB_CONN_STR")
	)

	database, err := db.DB(driver, dsn)
	if err != nil {
		panic(err)
	}

	if err := db.Migrate(database); err != nil {
		panic(err)
	}

	if err := seedTestUser(database); err != nil {
		panic(err)
	}

	parser := worker.NewEventParseWorker(database)
	parser.Start(parseInterval)
	defer parser.Stop()

	analyzer := worker.NewAnalysisWorker(database)
	analyzer.Start(analysisInterval)
	defer analyzer.Stop()

	app := fiber.New()

	rest.RegisterAPI(app, database)
	ui.RegisterUI(app)

	log.Fatal(app.Listen(":3000"))
}

// seedTestUser creates a login user from the environment when both credential
// variables are set.
func seedTestUser(database *gorm.DB) error {
	username := os.Getenv(seedUsernameEnv)
	password := os.Getenv(seedPasswordEnv)

	if username == "" && password == "" {
		return nil
	}

	if username == "" || password == "" {
		log.Printf("skipping test user seed: both %s and %s must be set", seedUsernameEnv, seedPasswordEnv)

		return nil
	}

	if err := db.SeedUser(database, username, password); err != nil {
		return fmt.Errorf("seed test user %q: %w", username, err)
	}

	log.Printf("seeded test user %q", username)

	return nil
}
