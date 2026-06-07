package builder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisTransport struct {
	client         *redis.Client
	agentID        string
	agentName      string
	taskStream     string
	executor       string
	maxConcurrency int
	pollInterval   time.Duration
	mu             sync.Mutex
	streamIDs      map[string]string
}

func NewRedisTransport(options Options) (*RedisTransport, error) {
	addr := strings.TrimSpace(options.RedisAddr)
	if addr == "" {
		return nil, errors.New("redis addr is required for redis builder transport")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	transport := &RedisTransport{
		client:         client,
		agentID:        strings.TrimSpace(options.AgentID),
		agentName:      strings.TrimSpace(options.Name),
		executor:       strings.TrimSpace(options.Executor),
		maxConcurrency: options.MaxConcurrency,
		pollInterval:   options.PollInterval,
		streamIDs:      map[string]string{},
	}
	if transport.pollInterval <= 0 {
		transport.pollInterval = 5 * time.Second
	}
	transport.taskStream = RedisTaskStreamForBuilder(transport.agentID)
	if err := transport.ensureGroup(context.Background()); err != nil {
		_ = client.Close()
		return nil, err
	}
	return transport, nil
}

func (t *RedisTransport) Heartbeat(ctx context.Context, heartbeat Heartbeat) error {
	return t.emit(ctx, "heartbeat", "", map[string]any{
		"heartbeat": heartbeat,
	})
}

func (t *RedisTransport) Claim(ctx context.Context) (Task, error) {
	streams, err := t.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    RedisTaskGroup,
		Consumer: t.agentID,
		Streams:  []string{t.taskStream, ">"},
		Count:    1,
		Block:    t.pollInterval,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return Task{}, errNoTask
	}
	if err != nil {
		return Task{}, err
	}
	for _, stream := range streams {
		for _, message := range stream.Messages {
			payload, _ := message.Values["payload"].(string)
			if strings.TrimSpace(payload) == "" {
				_ = t.client.XAck(ctx, t.taskStream, RedisTaskGroup, message.ID).Err()
				_ = t.client.XDel(ctx, t.taskStream, message.ID).Err()
				continue
			}
			var task Task
			if err := json.Unmarshal([]byte(payload), &task); err != nil {
				_ = t.client.XAck(ctx, t.taskStream, RedisTaskGroup, message.ID).Err()
				_ = t.client.XDel(ctx, t.taskStream, message.ID).Err()
				return Task{}, err
			}
			task.StreamID = message.ID
			t.remember(task.JobID, message.ID)
			if err := t.emit(ctx, "claimed", task.JobID, map[string]any{
				"buildRunId": task.BuildRunID,
				"projectId":  task.ProjectID,
			}); err != nil {
				return Task{}, err
			}
			return task, nil
		}
	}
	return Task{}, errNoTask
}

func (t *RedisTransport) AppendLogs(ctx context.Context, jobID string, content string) error {
	return t.emit(ctx, "log", jobID, map[string]any{"content": content})
}

func (t *RedisTransport) Complete(ctx context.Context, jobID string, result Result) error {
	if err := t.emit(ctx, "complete", jobID, map[string]any{"result": result}); err != nil {
		return err
	}
	return t.ack(ctx, jobID)
}

func (t *RedisTransport) Fail(ctx context.Context, jobID string, message string) error {
	if err := t.emit(ctx, "fail", jobID, map[string]any{"message": message}); err != nil {
		return err
	}
	return t.ack(ctx, jobID)
}

func (t *RedisTransport) Close() error {
	return t.client.Close()
}

func (t *RedisTransport) ensureGroup(ctx context.Context) error {
	err := t.client.XGroupCreateMkStream(ctx, t.taskStream, RedisTaskGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

func (t *RedisTransport) emit(ctx context.Context, eventType string, jobID string, fields map[string]any) error {
	payload, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	values := map[string]any{
		"type":    eventType,
		"agentId": t.agentID,
		"jobId":   jobID,
		"payload": string(payload),
	}
	return t.client.XAdd(ctx, &redis.XAddArgs{Stream: RedisEventStream, MaxLen: 50000, Approx: true, Values: values}).Err()
}

func (t *RedisTransport) ack(ctx context.Context, jobID string) error {
	if strings.TrimSpace(jobID) == "" {
		return nil
	}
	t.mu.Lock()
	streamID := t.streamIDs[jobID]
	delete(t.streamIDs, jobID)
	t.mu.Unlock()
	if streamID == "" {
		return nil
	}
	if err := t.client.XAck(ctx, t.taskStream, RedisTaskGroup, streamID).Err(); err != nil {
		return err
	}
	return t.client.XDel(ctx, t.taskStream, streamID).Err()
}

func (t *RedisTransport) remember(jobID string, streamID string) {
	if strings.TrimSpace(jobID) == "" || strings.TrimSpace(streamID) == "" {
		return
	}
	t.mu.Lock()
	t.streamIDs[jobID] = streamID
	t.mu.Unlock()
}

func (t *RedisTransport) String() string {
	return fmt.Sprintf("redis:%s", t.agentID)
}
