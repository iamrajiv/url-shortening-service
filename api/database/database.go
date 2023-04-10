package database

import (
	"context"
	"os"

	"github.com/go-redis/redis/v8"
)

var Ctx = context.Background()

// CreateClient creates a new Redis client and returns it
// It takes an integer argument dbNo, which specifies the Redis database number to use
// It reads the Redis server address and password from environment variables DB_ADDR and DB_PASS
func CreateClient(dbNo int) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:     os.Getenv("DB_ADDR"),
		Password: os.Getenv("DB_PASS"),
		DB:       dbNo,
	})
	return rdb
}
