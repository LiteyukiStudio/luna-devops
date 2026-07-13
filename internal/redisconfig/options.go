package redisconfig

import (
	"strings"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// Options is the shared Redis connection configuration used by go-redis and Asynq.
type Options struct {
	Addr     string
	Username string
	Password string
	DB       int
}

func (o Options) Normalized() Options {
	o.Addr = strings.TrimSpace(o.Addr)
	o.Username = strings.TrimSpace(o.Username)
	if o.DB < 0 {
		o.DB = 0
	}
	return o
}

func (o Options) GoRedis() *redis.Options {
	o = o.Normalized()
	return &redis.Options{
		Addr:     o.Addr,
		Username: o.Username,
		Password: o.Password,
		DB:       o.DB,
	}
}

func (o Options) Asynq() asynq.RedisClientOpt {
	o = o.Normalized()
	return asynq.RedisClientOpt{
		Addr:     o.Addr,
		Username: o.Username,
		Password: o.Password,
		DB:       o.DB,
	}
}
