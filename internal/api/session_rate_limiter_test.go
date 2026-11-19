package api

import (
	"context"
	"encoding/json"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
)

func TestRateLimiterUsesRedisPassword(t *testing.T) {
	server := miniredis.RunT(t)
	server.RequireAuth("secret")
	limiter := newRateLimiterWithRedis(redisconfig.Options{Addr: server.Addr(), Password: "secret"})
	t.Cleanup(func() { _ = limiter.redis.Close() })

	allowed, err := limiter.allow(context.Background(), "authenticated", 1, time.Minute)
	if err != nil {
		t.Fatalf("allow returned error: %v", err)
	}
	if !allowed {
		t.Fatal("authenticated Redis request was unexpectedly denied")
	}
}

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
			_, err := limiter.allow(context.Background(), "atomic", attempts, time.Minute)
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

func TestRateLimiterResetClearsCounter(t *testing.T) {
	server := miniredis.RunT(t)
	limiter := newRateLimiter(server.Addr())
	t.Cleanup(func() { _ = limiter.redis.Close() })

	if allowed, err := limiter.allow(context.Background(), "resettable", 1, time.Minute); err != nil || !allowed {
		t.Fatalf("first attempt: allowed=%v err=%v", allowed, err)
	}
	if allowed, err := limiter.allow(context.Background(), "resettable", 1, time.Minute); err != nil || allowed {
		t.Fatalf("second attempt: allowed=%v err=%v", allowed, err)
	}
	if err := limiter.reset(context.Background(), "resettable"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if allowed, err := limiter.allow(context.Background(), "resettable", 1, time.Minute); err != nil || !allowed {
		t.Fatalf("attempt after reset: allowed=%v err=%v", allowed, err)
	}
}

func TestLoginAccountRateLimitKeyDoesNotExposeAccount(t *testing.T) {
	server := miniredis.RunT(t)
	h := &Handlers{mode: "production", rateLimiter: newRateLimiter(server.Addr())}
	h.domains = newDomainHandlers(h)
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
	wantSuffix := transportapi.HashToken("user@example.com")
	if !strings.HasSuffix(keys[0], wantSuffix) {
		t.Fatalf("rate limit key = %q, want hash suffix %q", keys[0], wantSuffix)
	}
}

func TestOAuthClientRateLimitUsesIPAndHashedClientID(t *testing.T) {
	server := miniredis.RunT(t)
	h := &Handlers{mode: "production", rateLimiter: newRateLimiter(server.Addr())}
	h.domains = newDomainHandlers(h)
	t.Cleanup(func() { _ = h.rateLimiter.redis.Close() })

	for attempt := 0; attempt < 31; attempt++ {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest("POST", "/api/v1/oauth/token", nil)
		if allowed := h.allowOAuthTokenClientAttempt(ctx, "client-secret-name", oauthClientAttemptAuthorizationCode, ""); attempt < 30 && !allowed {
			t.Fatalf("attempt %d should be allowed", attempt+1)
		} else if attempt == 30 {
			if allowed {
				t.Fatal("attempt above the limit should be rejected")
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode rate limit response: %v", err)
			}
			if body["error"] != "temporarily_unavailable" {
				t.Fatalf("OAuth error = %#v", body)
			}
		}
	}
	for _, key := range server.Keys() {
		if strings.Contains(key, "client-secret-name") {
			t.Fatalf("rate limit key exposes client ID: %q", key)
		}
	}
}

func TestOAuthClientRateLimitIgnoresForwardedIPFromUntrustedPeer(t *testing.T) {
	server := miniredis.RunT(t)
	h := &Handlers{
		config:      Config{TrustedProxyCIDRs: []string{"192.0.2.0/24"}},
		mode:        "production",
		rateLimiter: newRateLimiter(server.Addr()),
	}
	h.domains = newDomainHandlers(h)
	t.Cleanup(func() { _ = h.rateLimiter.redis.Close() })

	router := gin.New()
	configureTrustedProxies(router, h.config.TrustedProxyCIDRs)
	router.POST("/token", func(ctx *gin.Context) {
		if h.allowOAuthTokenClientAttempt(ctx, "confidential-client", oauthClientAttemptRefresh, ctx.PostForm("refresh_token")) {
			ctx.Status(http.StatusNoContent)
		}
	})
	form := url.Values{"refresh_token": {"refresh-token"}}
	request := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	request.RemoteAddr = "198.51.100.30:8443"
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("X-Forwarded-For", "203.0.113.30")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("request = %d %s", recorder.Code, recorder.Body.String())
	}

	keys := strings.Join(server.Keys(), "\n")
	if !strings.Contains(keys, "oauth_client_ip:198.51.100.30") {
		t.Fatalf("rate-limit keys did not use socket peer: %s", keys)
	}
	if strings.Contains(keys, "oauth_client_ip:203.0.113.30") {
		t.Fatalf("rate-limit keys trusted a forwarded IP from an untrusted peer: %s", keys)
	}
}

func TestOAuthClientRateLimitResolvesTrustedProxyChain(t *testing.T) {
	server := miniredis.RunT(t)
	h := &Handlers{
		config:      Config{TrustedProxyCIDRs: []string{"192.0.2.0/24", "198.51.100.0/24"}},
		mode:        "production",
		rateLimiter: newRateLimiter(server.Addr()),
	}
	h.domains = newDomainHandlers(h)
	t.Cleanup(func() { _ = h.rateLimiter.redis.Close() })

	router := gin.New()
	configureTrustedProxies(router, h.config.TrustedProxyCIDRs)
	router.POST("/token", func(ctx *gin.Context) {
		if h.allowOAuthTokenClientAttempt(ctx, "confidential-client", oauthClientAttemptRefresh, ctx.PostForm("refresh_token")) {
			ctx.Status(http.StatusNoContent)
		}
	})
	form := url.Values{"refresh_token": {"refresh-token"}}
	request := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form.Encode()))
	request.RemoteAddr = "192.0.2.10:8443"
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("X-Forwarded-For", "203.0.113.40, 198.51.100.40")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("request = %d %s", recorder.Code, recorder.Body.String())
	}

	keys := strings.Join(server.Keys(), "\n")
	if !strings.Contains(keys, "oauth_client_ip:203.0.113.40") {
		t.Fatalf("rate-limit keys did not resolve the client through the trusted chain: %s", keys)
	}
	if strings.Contains(keys, "oauth_client_ip:198.51.100.40") || strings.Contains(keys, "oauth_client_ip:192.0.2.10") {
		t.Fatalf("rate-limit keys stopped at a trusted proxy: %s", keys)
	}
}
