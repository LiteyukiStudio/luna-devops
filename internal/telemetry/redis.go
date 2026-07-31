package telemetry

import (
	"errors"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

func InstrumentRedis(client *redis.Client) error {
	if client == nil {
		return nil
	}
	return errors.Join(
		redisotel.InstrumentTracing(client,
			redisotel.WithDBStatement(false),
			redisotel.WithCallerEnabled(false),
		),
		redisotel.InstrumentMetrics(client),
	)
}
