package api

import (
	"context"
	"encoding/json"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
		if allowed := h.allowOAuthClientAttempt(ctx, "client-secret-name"); attempt < 30 && !allowed {
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

func TestOAuthPublicClientRateLimitBindsCredentialAndDirectSource(t *testing.T) {
	newAttempt := func(t *testing.T, h *Handlers, form url.Values) (*httptest.ResponseRecorder, bool) {
		t.Helper()
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", strings.NewReader(form.Encode()))
		ctx.Request.RemoteAddr = "198.51.100.20:8443"
		ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		flow := oauthClientAttemptFlowForGrantType(form.Get("grant_type"))
		return recorder, h.allowOAuthTokenClientAttempt(ctx, lunaCLIClientID, flow, form.Get("refresh_token"))
	}

	t.Run("irrelevant field cannot change refresh credential bucket", func(t *testing.T) {
		server := miniredis.RunT(t)
		h := &Handlers{mode: "production", rateLimiter: newRateLimiter(server.Addr())}
		h.domains = newDomainHandlers(h)
		t.Cleanup(func() { _ = h.rateLimiter.redis.Close() })

		for index := 0; index < oauthCredentialRateLimit; index++ {
			form := url.Values{
				"grant_type":    {"refresh_token"},
				"refresh_token": {"repeated-refresh-token"},
				"device_code":   {"irrelevant-device-" + strconv.Itoa(index)},
			}
			if _, allowed := newAttempt(t, h, form); !allowed {
				t.Fatalf("refresh attempt %d should be allowed", index+1)
			}
		}
		recorder, allowed := newAttempt(t, h, url.Values{
			"grant_type":    {"refresh_token"},
			"refresh_token": {"repeated-refresh-token"},
			"device_code":   {"another-irrelevant-device"},
		})
		if allowed {
			t.Fatal("irrelevant device_code changed the refresh credential bucket")
		}
		if recorder.Header().Get("Retry-After") != "60" {
			t.Fatalf("Retry-After = %q, want 60", recorder.Header().Get("Retry-After"))
		}
		keys := server.Keys()
		if len(keys) != 2 {
			t.Fatalf("rate-limit keys = %#v, want one source and one refresh credential bucket", keys)
		}
		for _, key := range keys {
			if strings.Contains(key, lunaCLIClientID) || strings.Contains(key, "repeated-refresh-token") || strings.Contains(key, "irrelevant-device") {
				t.Fatalf("public OAuth rate-limit key exposes client or credential material: %q", key)
			}
		}
	})

	t.Run("direct peer always has a source bucket", func(t *testing.T) {
		server := miniredis.RunT(t)
		h := &Handlers{mode: "production", rateLimiter: newRateLimiter(server.Addr())}
		h.domains = newDomainHandlers(h)
		t.Cleanup(func() { _ = h.rateLimiter.redis.Close() })

		for index := 0; index < oauthIPRateLimit; index++ {
			form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"refresh-" + strconv.Itoa(index)}}
			if _, allowed := newAttempt(t, h, form); !allowed {
				t.Fatalf("direct source attempt %d should be allowed", index+1)
			}
		}
		if _, allowed := newAttempt(t, h, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {"refresh-over-limit"}}); allowed {
			t.Fatal("direct source exceeded its hard bucket")
		}
		key := "rate_limit:oauth_public_refresh_ip:198.51.100.20"
		if count, err := server.Get(key); err != nil || count != strconv.Itoa(oauthIPRateLimit+1) {
			t.Fatalf("direct source counter = %q, err = %v", count, err)
		}
		if len(server.Keys()) != oauthIPRateLimit+1 {
			t.Fatalf("rate-limit key count = %d, source rejection created an unbounded credential key", len(server.Keys()))
		}
	})
}

func TestOAuthPublicClientRateLimitSeparatesTokenFlows(t *testing.T) {
	server := miniredis.RunT(t)
	h := &Handlers{mode: "production", rateLimiter: newRateLimiter(server.Addr())}
	h.domains = newDomainHandlers(h)
	t.Cleanup(func() { _ = h.rateLimiter.redis.Close() })

	attempt := func(flow oauthClientAttemptFlow, field, value string) bool {
		t.Helper()
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		form := url.Values{}
		if field != "" {
			form.Set(field, value)
		}
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", strings.NewReader(form.Encode()))
		ctx.Request.RemoteAddr = "198.51.100.21:8443"
		ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return h.allowOAuthTokenClientAttempt(ctx, lunaCLIClientID, flow, value)
	}

	for index := 0; index < oauthIPRateLimit; index++ {
		if !attempt(oauthClientAttemptDeviceCode, "device_code", "device-"+strconv.Itoa(index)) {
			t.Fatalf("device-code attempt %d should be allowed", index+1)
		}
	}
	if attempt(oauthClientAttemptDeviceCode, "device_code", "device-over-limit") {
		t.Fatal("device-code source exceeded its hard bucket")
	}
	for _, test := range []struct {
		name  string
		flow  oauthClientAttemptFlow
		field string
	}{
		{name: "authorization code", flow: oauthClientAttemptAuthorizationCode, field: "code"},
		{name: "refresh", flow: oauthClientAttemptRefresh, field: "refresh_token"},
		{name: "revoke", flow: oauthClientAttemptRevoke, field: "token"},
	} {
		if !attempt(test.flow, test.field, test.name+"-credential") {
			t.Fatalf("device-code exhaustion blocked %s flow", test.name)
		}
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/oauth/device/authorization", nil)
	ctx.Request.RemoteAddr = "198.51.100.21:8443"
	if !h.allowOAuthClientAttempt(ctx, lunaCLIClientID) {
		t.Fatal("device-code exhaustion blocked device-start flow")
	}
}

func TestOAuthPublicClientRateLimitIgnoresForwardedIPFromUntrustedPeer(t *testing.T) {
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
		if h.allowOAuthTokenClientAttempt(ctx, lunaCLIClientID, oauthClientAttemptRefresh, ctx.PostForm("refresh_token")) {
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
	if !strings.Contains(keys, "oauth_public_refresh_ip:198.51.100.30") {
		t.Fatalf("rate-limit keys did not use socket peer: %s", keys)
	}
	if strings.Contains(keys, "oauth_public_refresh_ip:203.0.113.30") {
		t.Fatalf("rate-limit keys trusted a forwarded IP from an untrusted peer: %s", keys)
	}
}

func TestOAuthPublicClientRateLimitResolvesTrustedProxyChain(t *testing.T) {
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
		if h.allowOAuthTokenClientAttempt(ctx, lunaCLIClientID, oauthClientAttemptRefresh, ctx.PostForm("refresh_token")) {
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
	if !strings.Contains(keys, "oauth_public_refresh_ip:203.0.113.40") {
		t.Fatalf("rate-limit keys did not resolve the client through the trusted chain: %s", keys)
	}
	if strings.Contains(keys, "oauth_public_refresh_ip:198.51.100.40") || strings.Contains(keys, "oauth_public_refresh_ip:192.0.2.10") {
		t.Fatalf("rate-limit keys stopped at a trusted proxy: %s", keys)
	}
}

func TestOAuthPublicClientRateLimitsUseTrustedForwardedIPAndSeparateFlowBuckets(t *testing.T) {
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
	router.POST("/device-start", func(ctx *gin.Context) {
		if h.allowOAuthClientAttempt(ctx, lunaCLIClientID) {
			ctx.Status(http.StatusNoContent)
		}
	})
	router.POST("/token", func(ctx *gin.Context) {
		if h.allowOAuthTokenClientAttempt(ctx, lunaCLIClientID, oauthClientAttemptRefresh, ctx.PostForm("refresh_token")) {
			ctx.Status(http.StatusNoContent)
		}
	})

	attempt := func(path, clientIP string, form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		request.RemoteAddr = "192.0.2.10:8443"
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("X-Forwarded-For", clientIP)
		router.ServeHTTP(recorder, request)
		return recorder
	}

	for index := 0; index < oauthIPRateLimit; index++ {
		if response := attempt("/device-start", "203.0.113.10", nil); response.Code != http.StatusNoContent {
			t.Fatalf("device-start attempt %d = %d %s", index+1, response.Code, response.Body.String())
		}
	}
	if response := attempt("/device-start", "203.0.113.10", nil); response.Code != http.StatusTooManyRequests {
		t.Fatalf("exhausted first client bucket = %d %s", response.Code, response.Body.String())
	}
	if response := attempt("/device-start", "203.0.113.11", nil); response.Code != http.StatusNoContent {
		t.Fatalf("second client behind the same proxy shared the first bucket = %d %s", response.Code, response.Body.String())
	}
	if response := attempt("/token", "203.0.113.10", url.Values{"refresh_token": {"independent-refresh-token"}}); response.Code != http.StatusNoContent {
		t.Fatalf("device-start exhaustion blocked token traffic = %d %s", response.Code, response.Body.String())
	}

	keys := server.Keys()
	for _, expected := range []string{
		"oauth_public_device_start_ip:203.0.113.10",
		"oauth_public_device_start_ip:203.0.113.11",
		"oauth_public_refresh_ip:203.0.113.10",
	} {
		found := false
		for _, key := range keys {
			if strings.Contains(key, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("rate-limit keys %#v do not contain %q", keys, expected)
		}
	}
}

func TestOAuthDeviceVerificationRateLimitUsesIPAndHashedUserID(t *testing.T) {
	server := miniredis.RunT(t)
	h := &Handlers{mode: "production", rateLimiter: newRateLimiter(server.Addr())}
	h.domains = newDomainHandlers(h)
	t.Cleanup(func() { _ = h.rateLimiter.redis.Close() })

	for attempt := 0; attempt < 31; attempt++ {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest("POST", "/api/v1/oauth/device/verification", nil)
		if allowed := h.allowOAuthDeviceVerificationAttempt(ctx, "user-sensitive-id"); attempt < 30 && !allowed {
			t.Fatalf("attempt %d should be allowed", attempt+1)
		} else if attempt == 30 {
			if allowed {
				t.Fatal("attempt above the limit should be rejected")
			}
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode rate limit response: %v", err)
			}
			if body["code"] != "oauth.device.rate_limited" {
				t.Fatalf("OAuth device error = %#v", body)
			}
			if recorder.Header().Get("Retry-After") != "60" {
				t.Fatalf("Retry-After = %q, want 60", recorder.Header().Get("Retry-After"))
			}
		}
	}
	for _, key := range server.Keys() {
		if strings.Contains(key, "user-sensitive-id") {
			t.Fatalf("rate limit key exposes user ID: %q", key)
		}
	}
}
