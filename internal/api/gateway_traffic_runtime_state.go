package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	gatewayTrafficProbeStateTTL       = 10 * time.Minute
	gatewayTrafficProbeStateKeyPrefix = "gateway_traffic_probe:state:"
	gatewayTrafficProbeClusterSetKey  = "gateway_traffic_probe:clusters"
)

type gatewayTrafficRuntimeState struct {
	RuntimeClusterID string     `json:"runtimeClusterId"`
	Status           string     `json:"status"`
	LastHeartbeatAt  time.Time  `json:"lastHeartbeatAt"`
	LastReportedAt   *time.Time `json:"lastReportedAt,omitempty"`
	LastWindowStart  *time.Time `json:"lastWindowStart,omitempty"`
	LastWindowEnd    *time.Time `json:"lastWindowEnd,omitempty"`
	LastError        string     `json:"lastError,omitempty"`
	ExpiresAt        time.Time  `json:"expiresAt"`
}

type gatewayTrafficRuntimeStateStore interface {
	MarkHello(ctx context.Context, runtimeClusterID string) error
	MarkReport(ctx context.Context, runtimeClusterID string, windowStart time.Time, windowEnd time.Time) error
	Summary(ctx context.Context) (gatewayTrafficRuntimeState, bool, error)
}

type gatewayTrafficRuntimeStateStoreWithFallback struct {
	primary  gatewayTrafficRuntimeStateStore
	fallback *memoryGatewayTrafficRuntimeStateStore
}

func newGatewayTrafficRuntimeStateStore(redisAddr string) gatewayTrafficRuntimeStateStore {
	fallback := newMemoryGatewayTrafficRuntimeStateStore()
	if strings.TrimSpace(redisAddr) == "" {
		return fallback
	}
	return &gatewayTrafficRuntimeStateStoreWithFallback{
		primary:  newRedisGatewayTrafficRuntimeStateStore(redisAddr),
		fallback: fallback,
	}
}

func (s *gatewayTrafficRuntimeStateStoreWithFallback) MarkHello(ctx context.Context, runtimeClusterID string) error {
	if err := s.primary.MarkHello(ctx, runtimeClusterID); err == nil {
		return nil
	}
	return s.fallback.MarkHello(ctx, runtimeClusterID)
}

func (s *gatewayTrafficRuntimeStateStoreWithFallback) MarkReport(ctx context.Context, runtimeClusterID string, windowStart time.Time, windowEnd time.Time) error {
	if err := s.primary.MarkReport(ctx, runtimeClusterID, windowStart, windowEnd); err == nil {
		return nil
	}
	return s.fallback.MarkReport(ctx, runtimeClusterID, windowStart, windowEnd)
}

func (s *gatewayTrafficRuntimeStateStoreWithFallback) Summary(ctx context.Context) (gatewayTrafficRuntimeState, bool, error) {
	state, ok, err := s.primary.Summary(ctx)
	if err == nil {
		return state, ok, nil
	}
	return s.fallback.Summary(ctx)
}

type redisGatewayTrafficRuntimeStateStore struct {
	client *redis.Client
}

func newRedisGatewayTrafficRuntimeStateStore(redisAddr string) *redisGatewayTrafficRuntimeStateStore {
	return &redisGatewayTrafficRuntimeStateStore{client: redis.NewClient(&redis.Options{Addr: strings.TrimSpace(redisAddr)})}
}

func (s *redisGatewayTrafficRuntimeStateStore) MarkHello(ctx context.Context, runtimeClusterID string) error {
	runtimeClusterID = strings.TrimSpace(runtimeClusterID)
	if runtimeClusterID == "" {
		return nil
	}
	now := time.Now().UTC()
	state := gatewayTrafficRuntimeState{
		RuntimeClusterID: runtimeClusterID,
		Status:           "deployed",
		LastHeartbeatAt:  now,
		ExpiresAt:        now.Add(gatewayTrafficProbeStateTTL),
	}
	return s.updateState(ctx, state, false)
}

func (s *redisGatewayTrafficRuntimeStateStore) MarkReport(ctx context.Context, runtimeClusterID string, windowStart time.Time, windowEnd time.Time) error {
	runtimeClusterID = strings.TrimSpace(runtimeClusterID)
	if runtimeClusterID == "" {
		return nil
	}
	now := time.Now().UTC()
	state := gatewayTrafficRuntimeState{
		RuntimeClusterID: runtimeClusterID,
		Status:           "ready",
		LastHeartbeatAt:  now,
		LastReportedAt:   &now,
		LastWindowStart:  timePtr(windowStart.UTC()),
		LastWindowEnd:    timePtr(windowEnd.UTC()),
		ExpiresAt:        now.Add(gatewayTrafficProbeStateTTL),
	}
	return s.updateState(ctx, state, true)
}

func (s *redisGatewayTrafficRuntimeStateStore) Summary(ctx context.Context) (gatewayTrafficRuntimeState, bool, error) {
	clusterIDs, err := s.client.SMembers(ctx, gatewayTrafficProbeClusterSetKey).Result()
	if err != nil {
		return gatewayTrafficRuntimeState{}, false, err
	}
	now := time.Now().UTC()
	var best gatewayTrafficRuntimeState
	bestSet := false
	for _, clusterID := range clusterIDs {
		state, ok, err := s.get(ctx, clusterID)
		if err != nil {
			return gatewayTrafficRuntimeState{}, false, err
		}
		if !ok || !state.ExpiresAt.After(now) {
			_ = s.client.SRem(ctx, gatewayTrafficProbeClusterSetKey, clusterID).Err()
			continue
		}
		if shouldPreferGatewayTrafficState(state, best, bestSet) {
			best = state
			bestSet = true
		}
	}
	return best, bestSet, nil
}

func (s *redisGatewayTrafficRuntimeStateStore) updateState(ctx context.Context, incoming gatewayTrafficRuntimeState, keepReport bool) error {
	if keepReport {
		return s.set(ctx, incoming)
	}
	if existing, ok, err := s.get(ctx, incoming.RuntimeClusterID); err != nil {
		return err
	} else if ok && existing.ExpiresAt.After(time.Now().UTC()) && existing.Status == "ready" && existing.LastReportedAt != nil {
		incoming.Status = existing.Status
		incoming.LastReportedAt = existing.LastReportedAt
		incoming.LastWindowStart = existing.LastWindowStart
		incoming.LastWindowEnd = existing.LastWindowEnd
	}
	return s.set(ctx, incoming)
}

func (s *redisGatewayTrafficRuntimeStateStore) get(ctx context.Context, runtimeClusterID string) (gatewayTrafficRuntimeState, bool, error) {
	data, err := s.client.Get(ctx, gatewayTrafficProbeStateKeyPrefix+runtimeClusterID).Bytes()
	if errors.Is(err, redis.Nil) {
		return gatewayTrafficRuntimeState{}, false, nil
	}
	if err != nil {
		return gatewayTrafficRuntimeState{}, false, err
	}
	var state gatewayTrafficRuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return gatewayTrafficRuntimeState{}, false, err
	}
	return state, true, nil
}

func (s *redisGatewayTrafficRuntimeStateStore) set(ctx context.Context, state gatewayTrafficRuntimeState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := s.client.Set(ctx, gatewayTrafficProbeStateKeyPrefix+state.RuntimeClusterID, data, gatewayTrafficProbeStateTTL).Err(); err != nil {
		return err
	}
	return s.client.SAdd(ctx, gatewayTrafficProbeClusterSetKey, state.RuntimeClusterID).Err()
}

type memoryGatewayTrafficRuntimeStateStore struct {
	mu     sync.Mutex
	states map[string]gatewayTrafficRuntimeState
}

func newMemoryGatewayTrafficRuntimeStateStore() *memoryGatewayTrafficRuntimeStateStore {
	return &memoryGatewayTrafficRuntimeStateStore{states: map[string]gatewayTrafficRuntimeState{}}
}

func (s *memoryGatewayTrafficRuntimeStateStore) MarkHello(_ context.Context, runtimeClusterID string) error {
	runtimeClusterID = strings.TrimSpace(runtimeClusterID)
	if runtimeClusterID == "" {
		return nil
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[runtimeClusterID]
	if !state.ExpiresAt.After(now) {
		state = gatewayTrafficRuntimeState{}
	}
	state.RuntimeClusterID = runtimeClusterID
	state.LastHeartbeatAt = now
	state.ExpiresAt = now.Add(gatewayTrafficProbeStateTTL)
	if state.Status == "" {
		state.Status = "deployed"
	}
	s.states[runtimeClusterID] = state
	return nil
}

func (s *memoryGatewayTrafficRuntimeStateStore) MarkReport(_ context.Context, runtimeClusterID string, windowStart time.Time, windowEnd time.Time) error {
	runtimeClusterID = strings.TrimSpace(runtimeClusterID)
	if runtimeClusterID == "" {
		return nil
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[runtimeClusterID] = gatewayTrafficRuntimeState{
		RuntimeClusterID: runtimeClusterID,
		Status:           "ready",
		LastHeartbeatAt:  now,
		LastReportedAt:   &now,
		LastWindowStart:  timePtr(windowStart.UTC()),
		LastWindowEnd:    timePtr(windowEnd.UTC()),
		ExpiresAt:        now.Add(gatewayTrafficProbeStateTTL),
	}
	return nil
}

func (s *memoryGatewayTrafficRuntimeStateStore) Summary(_ context.Context) (gatewayTrafficRuntimeState, bool, error) {
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	var best gatewayTrafficRuntimeState
	bestSet := false
	for key, state := range s.states {
		if !state.ExpiresAt.After(now) {
			delete(s.states, key)
			continue
		}
		if shouldPreferGatewayTrafficState(state, best, bestSet) {
			best = state
			bestSet = true
		}
	}
	return best, bestSet, nil
}

func gatewayTrafficStateRank(state gatewayTrafficRuntimeState) int {
	switch state.Status {
	case "ready":
		return 0
	case "deployed":
		return 1
	default:
		return 2
	}
}

func shouldPreferGatewayTrafficState(candidate gatewayTrafficRuntimeState, current gatewayTrafficRuntimeState, currentSet bool) bool {
	if !currentSet {
		return true
	}
	candidateRank := gatewayTrafficStateRank(candidate)
	currentRank := gatewayTrafficStateRank(current)
	if candidateRank != currentRank {
		return candidateRank < currentRank
	}
	return candidate.LastHeartbeatAt.After(current.LastHeartbeatAt)
}

func timePtr(value time.Time) *time.Time {
	return &value
}
