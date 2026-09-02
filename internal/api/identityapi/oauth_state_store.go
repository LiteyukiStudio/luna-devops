package identityapi

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

const (
	gitOAuthStateKeyPrefix = "oauth:git:state:"
	oidcAuthStateKeyPrefix = "oauth:oidc:state:"
)

type GitOAuthStateValue struct {
	ProviderID     string `json:"providerId"`
	UserID         string `json:"userId"`
	RedirectPath   string `json:"redirectPath"`
	FrontendOrigin string `json:"frontendOrigin"`
	CallbackOrigin string `json:"callbackOrigin"`
}

type gitOAuthStateValue = GitOAuthStateValue

type OIDCAuthStateValue struct {
	Nonce        string `json:"nonce"`
	ProviderID   string `json:"providerId"`
	UserID       string `json:"userId"`
	Mode         string `json:"mode"`
	RedirectPath string `json:"redirectPath"`
}

type oidcAuthStateValue = OIDCAuthStateValue

type OAuthStateStore interface {
	SaveGit(ctx context.Context, state string, value GitOAuthStateValue, ttl time.Duration) error
	ConsumeGit(ctx context.Context, state string) (GitOAuthStateValue, bool, error)
	SaveOIDC(ctx context.Context, state string, value OIDCAuthStateValue, ttl time.Duration) error
	ConsumeOIDC(ctx context.Context, state string) (OIDCAuthStateValue, bool, error)
}

type oauthStateStore = OAuthStateStore

func NewOAuthStateStore(redisAddr string) OAuthStateStore {
	return newOAuthStateStoreWithRedis(redisconfig.Options{Addr: redisAddr})
}

func NewOAuthStateStoreWithRedis(options redisconfig.Options) OAuthStateStore {
	return newOAuthStateStoreWithRedis(options)
}

func newOAuthStateStore(redisAddr string) OAuthStateStore {
	return NewOAuthStateStore(redisAddr)
}

func newOAuthStateStoreWithRedis(options redisconfig.Options) OAuthStateStore {
	client := redis.NewClient(options.GoRedis())
	if err := telemetry.InstrumentRedis(client); err != nil {
		telemetry.LogWarn(context.Background(), "Redis telemetry initialization failed",
			"redis.instrumentation.failed", "redis.instrumentation.initialize",
			"telemetry.initialization.failed", err)
	}
	return &RedisOAuthStateStore{client: client}
}

type RedisOAuthStateStore struct {
	client *redis.Client
}

type redisOAuthStateStore = RedisOAuthStateStore

func (s *RedisOAuthStateStore) SaveGit(ctx context.Context, state string, value GitOAuthStateValue, ttl time.Duration) error {
	return s.save(ctx, gitOAuthStateKeyPrefix, state, value, ttl)
}

func (s *RedisOAuthStateStore) ConsumeGit(ctx context.Context, state string) (GitOAuthStateValue, bool, error) {
	return consumeRedisState[GitOAuthStateValue](ctx, s.client, gitOAuthStateKeyPrefix, state)
}

func (s *RedisOAuthStateStore) SaveOIDC(ctx context.Context, state string, value OIDCAuthStateValue, ttl time.Duration) error {
	return s.save(ctx, oidcAuthStateKeyPrefix, state, value, ttl)
}

func (s *RedisOAuthStateStore) ConsumeOIDC(ctx context.Context, state string) (OIDCAuthStateValue, bool, error) {
	return consumeRedisState[OIDCAuthStateValue](ctx, s.client, oidcAuthStateKeyPrefix, state)
}

func (s *RedisOAuthStateStore) save(ctx context.Context, prefix string, state string, value any, ttl time.Duration) error {
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
