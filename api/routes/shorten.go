package routes

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/iamrajiv/url-shortening-service/database"
	"github.com/iamrajiv/url-shortening-service/helpers"
)

type responseListAllURLs struct {
	URL         string        `json:"url"`
	CustomShort string        `json:"short"`
	Expiry      time.Duration `json:"expiry"`
}

type requestShortenURL struct {
	URL         string        `json:"url"`
	CustomShort string        `json:"short"`
	Expiry      time.Duration `json:"expiry"`
}

type responseShortenURL struct {
	URL             string        `json:"url"`
	CustomShort     string        `json:"short"`
	Expiry          time.Duration `json:"expiry"`
	XRateRemaining  int           `json:"rate_limit"`
	XRateLimitReset time.Duration `json:"rate_limit_reset"`
}

type requestDeleteShortURL struct {
	Short string `json:"short"`
}

type responseUpdateShortURL struct {
	URL             string        `json:"url"`
	OldShort        string        `json:"old_short"`
	UpdatedShort    string        `json:"updated_short"`
	Expiry          time.Duration `json:"expiry"`
	XRateRemaining  int           `json:"rate_limit"`
	XRateLimitReset time.Duration `json:"rate_limit_reset"`
}

type requestUpdateShortURL struct {
	Short        string        `json:"short"`
	UpdatedShort string        `json:"update_short"`
	Expiry       time.Duration `json:"expiry"`
}

// ListAllShortURLs is a function that retrieves all short URLs stored in the Redis database
// and returns them in a JSON format
func ListAllShortURLs(c *fiber.Ctx) error {
	// Create a Redis client using the database package's CreateClient method
	r := database.CreateClient(0)

	// Ensure the Redis client is closed after the function is finished executing
	defer r.Close()

	// Get all keys from the Redis database
	keys, err := r.Keys(database.Ctx, "*").Result()

	// If an error occurs while getting the keys, return an HTTP Internal Server Error (500)
	// with a JSON object containing an error message
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server",
		})
	}

	// Declare a slice of responseListAllURLs to store the retrieved short URLs
	var resp []responseListAllURLs

	// Iterate through the keys and retrieve the corresponding values
	for _, key := range keys {
		// Get the value associated with the key from the Redis database
		val, _ := r.Get(database.Ctx, key).Result()

		// Get the Time To Live (TTL) for the key from the Redis database
		ttl, _ := r.TTL(database.Ctx, key).Result()

		// Append a responseListAllURLs object containing the retrieved URL, custom short URL, and expiry time
		// to the resp slice
		resp = append(resp, responseListAllURLs{
			URL:         val,
			CustomShort: os.Getenv("DOMAIN") + "/" + key,
			Expiry:      ttl / time.Nanosecond / time.Hour,
		})
	}

	// Return an HTTP OK (200) status with the resp slice as a JSON object
	return c.Status(fiber.StatusOK).JSON(resp)
}

// ShortenURL shortens the url
// ShortenURL is a function that handles incoming requests to shorten a given URL.
func ShortenURL(c *fiber.Ctx) error {
	// Parse the incoming request body
	body := new(requestShortenURL)
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "cannot parse JSON",
		})
	}

	// Validate if the input is an actual URL
	if !govalidator.IsURL(body.URL) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid URL",
		})
	}

	// Ensure the URL starts with HTTP
	body.URL = helpers.EnforceHTTP(body.URL)

	// Prevent abuse by not allowing shortening of the domain itself
	if !helpers.RemoveDomainError(body.URL) {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Cannot shorten the domain",
		})
	}

	// Implement rate limiting using Redis
	// Allow API_QUOTA requests per IP address every 30 minutes
	r2 := database.CreateClient(1)
	defer r2.Close()
	_, err := r2.Get(database.Ctx, c.IP()).Result()
	if err == redis.Nil {
		_ = r2.Set(database.Ctx, c.IP(), os.Getenv("API_QUOTA"), 30*60*time.Second).Err()
	} else {
		val, _ := r2.Get(database.Ctx, c.IP()).Result()
		valInt, _ := strconv.Atoi(val)
		if valInt <= 0 {
			limit, _ := r2.TTL(database.Ctx, c.IP()).Result()
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error":            "Rate limit exceeded",
				"rate_limit_reset": limit / time.Nanosecond / time.Minute,
			})
		}
	}

	// Generate a unique ID for the short URL, either custom or using UUID
	var id string
	if body.CustomShort == "" {
		id = uuid.New().String()[:6]
	} else {
		id = body.CustomShort
	}

	// Connect to the Redis database
	r := database.CreateClient(0)
	defer r.Close()

	// Check if the provided custom short URL is already in use
	val, _ := r.Get(database.Ctx, id).Result()
	if val != "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "URL short already in use",
		})
	}

	// Set a default expiry time of 24 hours if not provided
	if body.Expiry == 0 {
		body.Expiry = 24
	}

	// Save the short URL in Redis with the specified expiry time
	err = r.Set(database.Ctx, id, body.URL, body.Expiry*3600*time.Second).Err()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server",
		})
	}

	// Prepare the response with the URL, short URL, expiry, and rate limiting info
	resp := responseShortenURL{
		URL:             body.URL,
		CustomShort:     "",
		Expiry:          body.Expiry,
		XRateRemaining:  10,
		XRateLimitReset: 30,
	}
	r2.Decr(database.Ctx, c.IP())
	val, _ = r2.Get(database.Ctx, c.IP()).Result()
	resp.XRateRemaining, _ = strconv.Atoi(val)
	ttl, _ := r2.TTL(database.Ctx, c.IP()).Result()
	resp.XRateLimitReset = ttl / time.Nanosecond / time.Minute

	// Create the final short URL by appending the unique ID to the domain
	resp.CustomShort = os.Getenv("DOMAIN") + "/" + id

	// Return the response with a status code of 200 OK and the short URL information in JSON format
	return c.Status(fiber.StatusOK).JSON(resp)
}

// DeleteShortURL is a function that handles the deletion of a given short URL
func DeleteShortURL(c *fiber.Ctx) error {
	// Parse the incoming request body and initialize a new requestDeleteShortURL struct
	body := new(requestDeleteShortURL)
	if err := c.BodyParser(&body); err != nil {
		// If parsing fails, return a bad request status with an error message
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "cannot parse JSON",
		})
	}
	// Validate if the provided short URL is an actual URL
	if !govalidator.IsURL(body.Short) {
		// If not a valid URL, return a bad request status with an error message
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid URL",
		})
	}

	// Ensure the provided short URL has an HTTP scheme
	body.Short = helpers.EnforceHTTP(body.Short)

	// Connect to the database to check if the custom short URL exists
	r := database.CreateClient(0)
	defer r.Close()
	keys, err := r.Keys(database.Ctx, "*").Result()
	if err != nil {
		// If connection fails, return an internal server error with an error message
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server",
		})
	}

	// Find the key for the provided short URL in the database
	var foundKey string
	for _, key := range keys {
		val := os.Getenv("DOMAIN") + "/" + key
		val = helpers.EnforceHTTP(val)
		if val == body.Short {
			foundKey = key
			break
		}
	}

	// If the short URL key is not found, return a not found status with an error message
	if foundKey == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Short URL not found",
		})
	}

	// Attempt to delete the custom short URL from the database using the found key
	err = r.Del(database.Ctx, foundKey).Err()
	if err != nil {
		// If deletion fails, return an internal server error with an error message
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server",
		})
	}

	// Return a success status and a message containing the name of the deleted short URL
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": fmt.Sprintf("Short URL '%s' successfully deleted", os.Getenv("DOMAIN")+"/"+foundKey),
	})
}

// UpdateShortURL updates the short URL
func UpdateShortURL(c *fiber.Ctx) error {
	// Check for the incoming request body
	body := new(requestUpdateShortURL)
	if err := c.BodyParser(&body); err != nil {
		// Return an error response with a status code of 400 and a JSON object with an error message
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "cannot parse JSON",
		})
	}
	// Check if the custom short URL exists in the database
	r := database.CreateClient(0)
	defer r.Close()
	keys, err := r.Keys(database.Ctx, "*").Result()
	if err != nil {
		// Return an error response with a status code of 500 and a JSON object with an error message
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server",
		})
	}

	// Get the original URL associated with the requested short URL
	orginalURL, _ := r.Get(database.Ctx, body.Short).Result()

	// Check if the requested short URL exists in the database
	var foundKey string
	for _, key := range keys {
		if key == body.Short {
			foundKey = key
			break
		}
	}

	// If the requested short URL does not exist, return an error response with a status code of 404 and a JSON object with an error message
	if foundKey == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Short URL not found",
		})
	}

	// Check if the new short URL is already in use
	valInUse, _ := r.Get(database.Ctx, body.UpdatedShort).Result()
	if valInUse != "" {
		// Return an error response with a status code of 403 and a JSON object with an error message
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "URL short already in use",
		})
	}

	// Rename the requested short URL to the updated short URL in the database
	err = r.Rename(database.Ctx, body.Short, body.UpdatedShort).Err()
	if err != nil {
		// Return an error response with a status code of 500 and a JSON object with an error message
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server",
		})
	}

	// Set the expiry time for the updated short URL in the database
	err = r.Expire(database.Ctx, body.UpdatedShort, time.Duration(body.Expiry)*time.Hour).Err()
	if err != nil {
		// Return an error response with a status code of 500 and a JSON object with an error message
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server",
		})
	}

	// Respond with the updated short URL, expiry in hours, calls remaining, and time to reset
	resp := responseUpdateShortURL{
		URL:             orginalURL,
		OldShort:        os.Getenv("DOMAIN") + "/" + body.Short,
		UpdatedShort:    os.Getenv("DOMAIN") + "/" + body.UpdatedShort,
		Expiry:          body.Expiry,
		XRateRemaining:  10,
		XRateLimitReset: 30,
	}

	// Implement rate limiting
	r2 := database.CreateClient(1)
	defer r2.Close()
	r2.Decr(database.Ctx, c.IP())
	val, _ := r2.Get(database.Ctx, c.IP()).Result()
	resp.XRateRemaining, _ = strconv.Atoi(val)
	ttl, _ := r2.TTL(database.Ctx, c.IP()).Result()
	resp.XRateLimitReset = ttl / time.Nanosecond / time.Minute
	// Return a success response with a status code of 200 and a JSON object with the updated short URL information
	return c.Status(fiber.StatusOK).JSON(resp)
}
