package volumetransferapi

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const ticketStoreTimeout = 500 * time.Millisecond

type TicketStore interface {
	Put(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Consume(ctx context.Context, key string) ([]byte, bool, error)
}

type RedisTicketStore struct {
	client *redis.Client
}

func NewRedisTicketStore(client *redis.Client) *RedisTicketStore {
	return &RedisTicketStore{client: client}
}

func (store *RedisTicketStore) Put(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if store == nil || store.client == nil || key == "" || ttl <= 0 {
		return errors.New("volume transfer ticket store is unavailable")
	}
	redisCtx, cancel := context.WithTimeout(ctx, ticketStoreTimeout)
	defer cancel()
	return store.client.Set(redisCtx, key, value, ttl).Err()
}

func (store *RedisTicketStore) Consume(ctx context.Context, key string) ([]byte, bool, error) {
	if store == nil || store.client == nil || key == "" {
		return nil, false, errors.New("volume transfer ticket store is unavailable")
	}
	redisCtx, cancel := context.WithTimeout(ctx, ticketStoreTimeout)
	defer cancel()
	value, err := store.client.GetDel(redisCtx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	return value, err == nil, err
}

func (store *RedisTicketStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if store == nil || store.client == nil || key == "" {
		return nil, false, errors.New("volume transfer ticket store is unavailable")
	}
	redisCtx, cancel := context.WithTimeout(ctx, ticketStoreTimeout)
	defer cancel()
	value, err := store.client.Get(redisCtx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	return value, err == nil, err
}

type memoryTicketValue struct {
	value     []byte
	expiresAt time.Time
}

// MemoryTicketStore is intended for development and tests only. Production
// construction uses Redis so tickets remain one-time across API replicas.
type MemoryTicketStore struct {
	mu     sync.Mutex
	values map[string]memoryTicketValue
}

func NewMemoryTicketStore() *MemoryTicketStore {
	return &MemoryTicketStore{values: map[string]memoryTicketValue{}}
}

func (store *MemoryTicketStore) Put(_ context.Context, key string, value []byte, ttl time.Duration) error {
	if store == nil || key == "" || ttl <= 0 {
		return errors.New("volume transfer ticket store is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.values[key] = memoryTicketValue{value: append([]byte(nil), value...), expiresAt: time.Now().Add(ttl)}
	return nil
}

func (store *MemoryTicketStore) Consume(_ context.Context, key string) ([]byte, bool, error) {
	if store == nil {
		return nil, false, errors.New("volume transfer ticket store is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	value, ok := store.values[key]
	delete(store.values, key)
	if !ok || !value.expiresAt.After(time.Now()) {
		return nil, false, nil
	}
	return append([]byte(nil), value.value...), true, nil
}

func (store *MemoryTicketStore) Get(_ context.Context, key string) ([]byte, bool, error) {
	if store == nil {
		return nil, false, errors.New("volume transfer ticket store is unavailable")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	value, ok := store.values[key]
	if !ok || !value.expiresAt.After(time.Now()) {
		delete(store.values, key)
		return nil, false, nil
	}
	return append([]byte(nil), value.value...), true, nil
}
