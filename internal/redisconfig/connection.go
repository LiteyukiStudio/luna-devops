package redisconfig

import (
	"context"
	"fmt"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

const startupPingTimeout = 5 * time.Second

// CheckConnection verifies that Redis is reachable and accepts the configured
// credentials before a service starts accepting work.
func CheckConnection(parent context.Context, options Options) error {
	options = options.Normalized()
	if options.Addr == "" {
		return fmt.Errorf("REDIS_ADDR is required")
	}

	ctx, cancel := context.WithTimeout(parent, startupPingTimeout)
	defer cancel()

	clientOptions := options.GoRedis()
	clientOptions.MaxRetries = -1
	client := redis.NewClient(clientOptions)
	defer client.Close()
	if err := telemetry.InstrumentRedis(client); err != nil {
		return fmt.Errorf("instrument Redis: %w", err)
	}
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping Redis: %w", err)
	}
	return nil
}
