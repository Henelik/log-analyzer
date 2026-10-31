package main

import (
	"log"
	"os"
	"time"

	"github.com/Henelik/log-analyzer/db"
	"github.com/Henelik/log-analyzer/rest"
	"github.com/Henelik/log-analyzer/worker"
	"github.com/gofiber/fiber/v3"
)

const parseInterval = 30 * time.Second

func main() {
	var (
		driver = os.Getenv("DB_DRIVER")
		dsn    = os.Getenv("DB_CONN_STR")
	)

	database, err := db.DB(driver, dsn)
	if err != nil {
		panic(err)
	}

	parser := worker.NewEventParseWorker(database)
	parser.Start(parseInterval)
	defer parser.Stop()

	app := fiber.New()

	rest.RegisterAPI(app, database)

	log.Fatal(app.Listen(":3000"))
}
