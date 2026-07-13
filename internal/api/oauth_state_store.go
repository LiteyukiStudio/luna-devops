package api

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/redis/go-redis/v9"
)

const (
	gitOAuthStateKeyPrefix = "oauth:git:state:"
	oidcAuthStateKeyPrefix = "oauth:oidc:state:"
)

type gitOAuthStateValue struct {
	ProviderID     string `json:"providerId"`
	UserID         string `json:"userId"`
	RedirectPath   string `json:"redirectPath"`
	FrontendOrigin string `json:"frontendOrigin"`
	CallbackOrigin string `json:"callbackOrigin"`
}

type oidcAuthStateValue struct {
	Nonce        string `json:"nonce"`
	ProviderID   string `json:"providerId"`
	UserID       string `json:"userId"`
	Mode         string `json:"mode"`
	RedirectPath string `json:"redirectPath"`
}

type oauthStateStore interface {
	SaveGit(ctx context.Context, state string, value gitOAuthStateValue, ttl time.Duration) error
	ConsumeGit(ctx context.Context, state string) (gitOAuthStateValue, bool, error)
	SaveOIDC(ctx context.Context, state string, value oidcAuthStateValue, ttl time.Duration) error
	ConsumeOIDC(ctx context.Context, state string) (oidcAuthStateValue, bool, error)
}

func newOAuthStateStore(redisAddr string) oauthStateStore {
	return newOAuthStateStoreWithRedis(redisconfig.Options{Addr: redisAddr})
}

func newOAuthStateStoreWithRedis(options redisconfig.Options) oauthStateStore {
	return &redisOAuthStateStore{client: redis.NewClient(options.GoRedis())}
}

type redisOAuthStateStore struct {
	client *redis.Client
}

func (s *redisOAuthStateStore) SaveGit(ctx context.Context, state string, value gitOAuthStateValue, ttl time.Duration) error {
	return s.save(ctx, gitOAuthStateKeyPrefix, state, value, ttl)
}

func (s *redisOAuthStateStore) ConsumeGit(ctx context.Context, state string) (gitOAuthStateValue, bool, error) {
	return consumeRedisState[gitOAuthStateValue](ctx, s.client, gitOAuthStateKeyPrefix, state)
}

func (s *redisOAuthStateStore) SaveOIDC(ctx context.Context, state string, value oidcAuthStateValue, ttl time.Duration) error {
	return s.save(ctx, oidcAuthStateKeyPrefix, state, value, ttl)
}

func (s *redisOAuthStateStore) ConsumeOIDC(ctx context.Context, state string) (oidcAuthStateValue, bool, error) {
	return consumeRedisState[oidcAuthStateValue](ctx, s.client, oidcAuthStateKeyPrefix, state)
}

func (s *redisOAuthStateStore) save(ctx context.Context, prefix string, state string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, prefix+hashToken(state), data, ttl).Err()
}

func consumeRedisState[T any](ctx context.Context, client *redis.Client, prefix string, state string) (T, bool, error) {
	var value T
	key := prefix + hashToken(state)
	raw, err := client.GetDel(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return value, false, nil
	}
	if err != nil {
		return value, false, err
	}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return value, false, err
	}
	return value, true, nil
}
