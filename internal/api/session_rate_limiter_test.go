package api

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
)

func TestRateLimiterAtomicallyIncrementsAndSetsTTL(t *testing.T) {
	server := miniredis.RunT(t)
	limiter := newRateLimiter(server.Addr())
	t.Cleanup(func() { _ = limiter.redis.Close() })

	const attempts = 32
	start := make(chan struct{})
	results := make(chan error, attempts)
	var workers sync.WaitGroup
	for range attempts {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, err := limiter.allow("atomic", attempts, time.Minute)
			results <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("allow: %v", err)
		}
	}

	if got, err := server.Get("rate_limit:atomic"); err != nil || got != "32" {
		t.Fatalf("counter = %q, err = %v", got, err)
	}
	if ttl := server.TTL("rate_limit:atomic"); ttl <= 0 || ttl > time.Minute {
		t.Fatalf("TTL = %s", ttl)
	}
}

func TestLoginAccountRateLimitKeyDoesNotExposeAccount(t *testing.T) {
	server := miniredis.RunT(t)
	h := &Handlers{mode: "production", rateLimiter: newRateLimiter(server.Addr())}
	t.Cleanup(func() { _ = h.rateLimiter.redis.Close() })
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/api/v1/auth/login", nil)

	if !h.allowLoginAccountAttempt(ctx, " User@Example.com ", 2, time.Minute) {
		t.Fatal("first account attempt must be allowed")
	}
	if !h.allowLoginAccountAttempt(ctx, "user@example.com", 2, time.Minute) {
		t.Fatal("second account attempt must be allowed")
	}
	if h.allowLoginAccountAttempt(ctx, "USER@EXAMPLE.COM", 2, time.Minute) {
		t.Fatal("normalized account must be limited after reaching the threshold")
	}
	keys := server.Keys()
	if len(keys) != 1 || strings.Contains(keys[0], "user@example.com") {
		t.Fatalf("rate limit keys = %#v", keys)
	}
	wantSuffix := hashToken("user@example.com")
	if !strings.HasSuffix(keys[0], wantSuffix) {
		t.Fatalf("rate limit key = %q, want hash suffix %q", keys[0], wantSuffix)
	}
}
