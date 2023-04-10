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
	OldURL          string        `json:"old_url"`
	UpdatedURL      string        `json:"updated_url"`
	OldShort        string        `json:"old_short"`
	UpdatedShort    string        `json:"updated_short"`
	Expiry          time.Duration `json:"expiry"`
	XRateRemaining  int           `json:"rate_limit"`
	XRateLimitReset time.Duration `json:"rate_limit_reset"`
}

type requestUpdateShortURL struct {
	Short  string        `json:"short"`
	URL    string        `json:"url"`
	Expiry time.Duration `json:"expiry"`
}

// ListURLs lists all shortened urls in the database with their expiry time and original url

func ListAllShortURLs(c *fiber.Ctx) error {
	r := database.CreateClient(0)
	defer r.Close()
	keys, err := r.Keys(database.Ctx, "*").Result()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server",
		})
	}
	var resp []responseListAllURLs
	for _, key := range keys {
		val, _ := r.Get(database.Ctx, key).Result()
		ttl, _ := r.TTL(database.Ctx, key).Result()
		resp = append(resp, responseListAllURLs{
			URL:         val,
			CustomShort: os.Getenv("DOMAIN") + "/" + key,
			Expiry:      ttl / time.Nanosecond / time.Hour,
		})
	}
	return c.Status(fiber.StatusOK).JSON(resp)
}

func ShortenURL(c *fiber.Ctx) error {
	// check for the incoming request body
	body := new(requestShortenURL)
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "cannot parse JSON",
		})
	}

	// implement rate limiting
	// everytime a user queries, check if the IP is already in database,
	// if yes, decrement the calls remaining by one, else add the IP to database
	// with expiry of `30mins`. So in this case the user will be able to send 10
	// requests every 30 minutes
	r2 := database.CreateClient(1)
	defer r2.Close()
	val, err := r2.Get(database.Ctx, c.IP()).Result()
	if err == redis.Nil {
		_ = r2.Set(database.Ctx, c.IP(), os.Getenv("API_QUOTA"), 30*60*time.Second).Err() //change the rate_limit_reset here, change `30` to your number
	} else {
		val, _ = r2.Get(database.Ctx, c.IP()).Result()
		valInt, _ := strconv.Atoi(val)
		if valInt <= 0 {
			limit, _ := r2.TTL(database.Ctx, c.IP()).Result()
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error":            "Rate limit exceeded",
				"rate_limit_reset": limit / time.Nanosecond / time.Minute,
			})
		}
	}

	// check if the input is an actual URL
	if !govalidator.IsURL(body.URL) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid URL",
		})
	}

	// check for the domain error
	// users may abuse the shortener by shorting the domain `localhost:3000` itself
	// leading to a inifite loop, so don't accept the domain for shortening
	if !helpers.RemoveDomainError(body.URL) {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "haha... nice try",
		})
	}

	// enforce https
	// all url will be converted to https before storing in database
	body.URL = helpers.EnforceHTTP(body.URL)

	// check if the user has provided any custom dhort urls
	// if yes, proceed,
	// else, create a new short using the first 6 digits of uuid
	// haven't performed any collision checks on this
	// you can create one for your own
	var id string
	if body.CustomShort == "" {
		id = uuid.New().String()[:6]
	} else {
		id = body.CustomShort
	}

	r := database.CreateClient(0)
	defer r.Close()

	val, _ = r.Get(database.Ctx, id).Result()
	// check if the user provided short is already in use
	if val != "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "URL short already in use",
		})
	}
	if body.Expiry == 0 {
		body.Expiry = 24 // default expiry of 24 hours
	}
	err = r.Set(database.Ctx, id, body.URL, body.Expiry*3600*time.Second).Err()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server",
		})
	}
	// respond with the url, short, expiry in hours, calls remaining and time to reset
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

	resp.CustomShort = os.Getenv("DOMAIN") + "/" + id
	return c.Status(fiber.StatusOK).JSON(resp)
}

func DeleteShortURL(c *fiber.Ctx) error {
	// check for the incoming request body
	body := new(requestDeleteShortURL)
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "cannot parse JSON",
		})
	}

	// check if the input is an actual URL
	if !govalidator.IsURL(body.Short) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid URL",
		})
	}

	// enforce https
	// all url will be converted to https before storing in database
	body.Short = helpers.EnforceHTTP(body.Short)

	// check if the custom short URL exists in the database
	r := database.CreateClient(0)
	defer r.Close()
	keys, err := r.Keys(database.Ctx, "*").Result()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server",
		})
	}

	var foundKey string
	for _, key := range keys {
		val := os.Getenv("DOMAIN") + "/" + key
		val = helpers.EnforceHTTP(val)
		if val == body.Short {
			foundKey = key
			break
		}
	}

	if foundKey == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Short URL not found",
		})
	}

	// delete the custom short URL from the database
	err = r.Del(database.Ctx, foundKey).Err()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server",
		})
	}

	// respond with a success message that includes the name of the deleted short URL
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": fmt.Sprintf("Short URL '%s' successfully deleted", os.Getenv("DOMAIN")+"/"+foundKey),
	})
}

// Function to update the short url after update return
func UpdateShortURL(c *fiber.Ctx) error {
	// check for the incoming request body
	body := new(requestUpdateShortURL)
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "cannot parse JSON",
		})
	}

	// check if the input is an actual URL
	if !govalidator.IsURL(body.URL) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid URL",
		})
	}

	// enforce https
	// all url will be converted to https before storing in database
	body.URL = helpers.EnforceHTTP(body.URL)

	// check if the custom short URL exists in the database
	r := database.CreateClient(0)
	defer r.Close()
	keys, err := r.Keys(database.Ctx, "*").Result()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server",
		})
	}

	var foundKey string
	for _, key := range keys {
		val := os.Getenv("DOMAIN") + "/" + key
		val = helpers.EnforceHTTP(val)
		if val == body.Short {
			foundKey = key
			break
		}
	}

	if foundKey == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Short URL not found",
		})
	}

	// update the URL associated with the custom short URL in the database
	err = r.Set(database.Ctx, foundKey, body.URL, body.Expiry*3600*time.Second).Err()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Unable to connect to server",
		})
	}

	// respond with the updated URL and custom short URL, and the time remaining until the rate limit resets
	resp := responseUpdateShortURL{
		OldURL:          "",
		UpdatedURL:      body.URL,
		OldShort:        "",
		UpdatedShort:    body.Short,
		Expiry:          body.Expiry,
		XRateRemaining:  10,
		XRateLimitReset: 30,
	}
	r2 := database.CreateClient(1)
	defer r2.Close()
	r2.Decr(database.Ctx, c.IP())
	val, _ := r2.Get(database.Ctx, c.IP()).Result()
	resp.XRateRemaining, _ = strconv.Atoi(val)
	ttl, _ := r2.TTL(database.Ctx, c.IP()).Result()
	resp.XRateLimitReset = ttl / time.Nanosecond / time.Minute

	resp.OldURL, _ = r.Get(database.Ctx, foundKey).Result()
	resp.OldShort = os.Getenv("DOMAIN") + "/" + foundKey

	return c.Status(fiber.StatusOK).JSON(resp)
}
