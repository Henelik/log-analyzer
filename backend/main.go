package main

import (
	"log"
	"os"

	"github.com/Henelik/log-analyzer/db"
	"github.com/Henelik/log-analyzer/rest"
	"github.com/gofiber/fiber/v3"
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

	app := fiber.New()

	rest.RegisterAPI(app, database)

	log.Fatal(app.Listen(":3000"))
}
