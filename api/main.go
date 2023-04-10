package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/iamrajiv/url-shortening-service/routes"
	"github.com/joho/godotenv"
)

// Helper function that sets up routes for the app instance
func setupRoutes(app *fiber.App) {
	// Map routes to corresponding handler functions
	app.Get("/:url", routes.ResolveURL)
	app.Get("/api/v1/all", routes.ListAllShortURLs)
	app.Post("/api/v1/create", routes.ShortenURL)
	app.Post("/api/v1/delete", routes.DeleteShortURL)
	app.Post("/api/v1/update", routes.UpdateShortURL)
}

func main() {
	// Load environment variables from .env file
	err := godotenv.Load()
	if err != nil {
		fmt.Println(err)
	}

	// Create a new Fiber app instance
	app := fiber.New()

	// Use request logging middleware
	app.Use(logger.New())

	// Set up routes
	setupRoutes(app)

	// Start the app and listen on the specified port
	log.Fatal(app.Listen(os.Getenv("APP_PORT")))
}
