package routes

import (
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/iamrajiv/url-shortening-service/database"
)

// ResolveURL retrieves the URL corresponding to the given short URL parameter from the Redis database
// and redirects the user to the corresponding full URL
func ResolveURL(c *fiber.Ctx) error {
	// Extract the short URL parameter from the request context
	url := c.Params("url")

	// Create a new Redis client with database 0
	r := database.CreateClient(0)
	defer r.Close()

	// Retrieve the value corresponding to the given key (short URL) from the Redis database
	value, err := r.Get(database.Ctx, url).Result()

	// Handle the case where the key (short URL) is not found in the database
	if err == redis.Nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "short not found on database",
		})
	} else if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "cannot connect to DB",
		})
	}

	// Create a new Redis client with database 1
	rInr := database.CreateClient(1)
	defer rInr.Close()

	// Increment the "counter" key in database 1
	_ = rInr.Incr(database.Ctx, "counter")

	// Redirect the user to the full URL corresponding to the given short URL
	return c.Redirect(value, 301)
}
