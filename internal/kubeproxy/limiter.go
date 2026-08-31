package kubeproxy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type LimiterConfig struct {
	AnonymousPerMinute int
	AnonymousBurst     int
	UserPerMinute      int
	ProjectPerMinute   int
	UserWatch          int
	ProjectWatch       int
	UserLogs           int
	ProjectLogs        int
	UserUpgrade        int
	ProjectUpgrade     int
	MaxBuckets         int
	BucketIdleTTL      time.Duration
	Now                func() time.Time
}

func DefaultLimiterConfig() LimiterConfig {
	return LimiterConfig{
		AnonymousPerMinute: 60, AnonymousBurst: 20, UserPerMinute: 300, ProjectPerMinute: 1200,
		UserWatch: 8, ProjectWatch: 64, UserLogs: 4, ProjectLogs: 32, UserUpgrade: 4, ProjectUpgrade: 32,
		MaxBuckets: 10000, BucketIdleTTL: 10 * time.Minute, Now: time.Now,
	}
}

type tokenBucket struct {
	tokens float64
	last   time.Time
}

type LocalLimiter struct {
	config  LimiterConfig
	mu      sync.Mutex
	buckets map[string]tokenBucket
	streams map[string]int
}

func NewLocalLimiter(config LimiterConfig) *LocalLimiter {
	defaults := DefaultLimiterConfig()
	if config.AnonymousPerMinute <= 0 {
		config.AnonymousPerMinute = defaults.AnonymousPerMinute
	}
	if config.AnonymousBurst <= 0 {
		config.AnonymousBurst = defaults.AnonymousBurst
	}
	if config.UserPerMinute <= 0 {
		config.UserPerMinute = defaults.UserPerMinute
	}
	if config.ProjectPerMinute <= 0 {
		config.ProjectPerMinute = defaults.ProjectPerMinute
	}
	if config.UserWatch <= 0 {
		config.UserWatch = defaults.UserWatch
	}
	if config.ProjectWatch <= 0 {
		config.ProjectWatch = defaults.ProjectWatch
	}
	if config.UserLogs <= 0 {
		config.UserLogs = defaults.UserLogs
	}
	if config.ProjectLogs <= 0 {
		config.ProjectLogs = defaults.ProjectLogs
	}
	if config.UserUpgrade <= 0 {
		config.UserUpgrade = defaults.UserUpgrade
	}
	if config.ProjectUpgrade <= 0 {
		config.ProjectUpgrade = defaults.ProjectUpgrade
	}
	if config.MaxBuckets <= 0 {
		config.MaxBuckets = defaults.MaxBuckets
	}
	if config.BucketIdleTTL <= 0 {
		config.BucketIdleTTL = defaults.BucketIdleTTL
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &LocalLimiter{config: config, buckets: map[string]tokenBucket{}, streams: map[string]int{}}
}

func (limiter *LocalLimiter) AllowPreAuth(_ context.Context, key ClientKey, _ RequestClass) error {
	if limiter == nil || key.Value == "" {
		return RateLimited(fmt.Errorf("anonymous client key is unavailable"))
	}
	if !limiter.take("anonymous:"+key.Value, limiter.config.AnonymousPerMinute, limiter.config.AnonymousBurst) {
		return RateLimited(fmt.Errorf("anonymous request limit exceeded"))
	}
	return nil
}

func (limiter *LocalLimiter) AllowRequest(_ context.Context, access AccessContext, _ RequestInfo) error {
	if limiter == nil || access.UserID == "" || access.ProjectID == "" {
		return RateLimited(fmt.Errorf("authenticated limiter key is unavailable"))
	}
	if !limiter.take("user:"+access.UserID, limiter.config.UserPerMinute, limiter.config.UserPerMinute) {
		return RateLimited(fmt.Errorf("user request limit exceeded"))
	}
	if !limiter.take("project:"+access.ProjectID, limiter.config.ProjectPerMinute, limiter.config.ProjectPerMinute) {
		return RateLimited(fmt.Errorf("project request limit exceeded"))
	}
	return nil
}

func (limiter *LocalLimiter) take(key string, perMinute, burst int) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := limiter.config.Now()
	bucket, exists := limiter.buckets[key]
	if !exists && len(limiter.buckets) >= limiter.config.MaxBuckets {
		for candidate, value := range limiter.buckets {
			if now.Sub(value.last) >= limiter.config.BucketIdleTTL {
				delete(limiter.buckets, candidate)
			}
		}
		if len(limiter.buckets) >= limiter.config.MaxBuckets {
			return false
		}
	}
	if bucket.last.IsZero() {
		bucket.tokens, bucket.last = float64(burst), now
	}
	elapsed := now.Sub(bucket.last).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	bucket.tokens += elapsed * float64(perMinute) / 60
	if bucket.tokens > float64(burst) {
		bucket.tokens = float64(burst)
	}
	bucket.last = now
	if bucket.tokens < 1 {
		limiter.buckets[key] = bucket
		return false
	}
	bucket.tokens--
	limiter.buckets[key] = bucket
	return true
}

func (limiter *LocalLimiter) AcquireStream(_ context.Context, access AccessContext, class StreamClass) (func(), error) {
	if limiter == nil || access.UserID == "" || access.ProjectID == "" {
		return nil, RateLimited(fmt.Errorf("stream limiter key is unavailable"))
	}
	if class != StreamWatch && class != StreamLogs && class != StreamUpgrade {
		return nil, RateLimited(fmt.Errorf("unknown stream class"))
	}
	userLimit, projectLimit := limiter.streamLimits(class)
	userKey := "stream:" + string(class) + ":user:" + access.UserID
	projectKey := "stream:" + string(class) + ":project:" + access.ProjectID
	limiter.mu.Lock()
	if limiter.streams[userKey] >= userLimit || limiter.streams[projectKey] >= projectLimit {
		limiter.mu.Unlock()
		return nil, RateLimited(fmt.Errorf("stream concurrency limit exceeded"))
	}
	limiter.streams[userKey]++
	limiter.streams[projectKey]++
	limiter.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			limiter.mu.Lock()
			defer limiter.mu.Unlock()
			limiter.streams[userKey]--
			limiter.streams[projectKey]--
			if limiter.streams[userKey] == 0 {
				delete(limiter.streams, userKey)
			}
			if limiter.streams[projectKey] == 0 {
				delete(limiter.streams, projectKey)
			}
		})
	}, nil
}

func (limiter *LocalLimiter) streamLimits(class StreamClass) (int, int) {
	switch class {
	case StreamWatch:
		return limiter.config.UserWatch, limiter.config.ProjectWatch
	case StreamLogs:
		return limiter.config.UserLogs, limiter.config.ProjectLogs
	default:
		return limiter.config.UserUpgrade, limiter.config.ProjectUpgrade
	}
}
