package redisconnect

import (
	"context"
	"fmt"

	nrredis "github.com/newrelic/go-agent/v3/integrations/nrredis-v9"
	"github.com/redis/go-redis/v9"
)

func ConnectRedis(config RedisConfig) (redisClient *redis.Client, err error) {
	ctx := context.Background()
	opts := &redis.Options{
		Addr:     fmt.Sprintf("%s:%s", config.Host, config.Port),
		Password: config.Password,
		DB:       0,
	}
	redisClient = redis.NewClient(
		opts,
	)
	redisClient.AddHook(nrredis.NewHook(opts))

	err = redisClient.Ping(ctx).Err()
	if err != nil {
		return
	}
	return
}
