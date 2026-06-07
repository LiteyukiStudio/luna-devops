package builder

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/redis/go-redis/v9"
)

const (
	RedisTaskStream  = "liteyuki:builder:tasks"
	RedisEventStream = "liteyuki:builder:events"
	RedisTaskGroup   = "liteyuki-builders"
	RedisEventGroup  = "liteyuki-builder-events"
)

func NewRedisClient(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr})
}

func RedisTaskStreamForBuilder(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return RedisTaskStream
	}
	return RedisTaskStream + ":" + agentID
}

func EnqueueRedisTask(ctx context.Context, client *redis.Client, task Task) error {
	payload, err := json.Marshal(task)
	if err != nil {
		return err
	}
	stream := RedisTaskStreamForBuilder(task.TargetBuilder)
	return client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		MaxLen: 10000,
		Approx: true,
		Values: map[string]any{
			"jobId":   task.JobID,
			"payload": string(payload),
		},
	}).Err()
}
