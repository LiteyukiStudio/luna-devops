package builder

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	TransportHTTP  = "http"
	TransportRedis = "redis"
)

var errNoTask = errors.New("no builder task")

type Transport interface {
	Heartbeat(ctx context.Context, heartbeat Heartbeat) error
	Claim(ctx context.Context) (Task, error)
	AppendLogs(ctx context.Context, jobID string, content string) error
	Complete(ctx context.Context, jobID string, result Result) error
	Fail(ctx context.Context, jobID string, message string) error
	Close() error
}

type Heartbeat struct {
	AgentID            string   `json:"agentId"`
	Name               string   `json:"name"`
	Labels             []string `json:"labels"`
	Scopes             []string `json:"scopes"`
	Executor           string   `json:"executor"`
	MaxConcurrency     int      `json:"maxConcurrency"`
	CurrentConcurrency int      `json:"currentConcurrency"`
}

func NewTransport(options Options) (Transport, error) {
	switch strings.ToLower(strings.TrimSpace(options.Transport)) {
	case "", TransportRedis:
		return NewRedisTransport(options)
	case TransportHTTP:
		return nil, errors.New("http builder transport is disabled; use redis builder transport")
	default:
		return nil, fmt.Errorf("unsupported builder transport: %s", options.Transport)
	}
}
