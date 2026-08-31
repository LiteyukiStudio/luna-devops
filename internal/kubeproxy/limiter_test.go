package kubeproxy

import (
	"testing"
	"time"
)

func TestLocalLimiterReleasesStreamLeaseOnce(t *testing.T) {
	config := DefaultLimiterConfig()
	config.UserUpgrade, config.ProjectUpgrade = 1, 1
	limiter := NewLocalLimiter(config)
	release, err := limiter.AcquireStream(t.Context(), baseAccess(), StreamUpgrade)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := limiter.AcquireStream(t.Context(), baseAccess(), StreamUpgrade); err == nil {
		t.Fatal("second stream should be limited")
	}
	release()
	release()
	if _, err := limiter.AcquireStream(t.Context(), baseAccess(), StreamUpgrade); err != nil {
		t.Fatalf("lease was not released: %v", err)
	}
}

func TestAnonymousTokenBucketRefills(t *testing.T) {
	now := time.Unix(0, 0)
	config := DefaultLimiterConfig()
	config.AnonymousBurst, config.AnonymousPerMinute = 1, 60
	config.Now = func() time.Time { return now }
	limiter := NewLocalLimiter(config)
	key := ClientKey{Value: "client"}
	if err := limiter.AllowPreAuth(t.Context(), key, RequestClassAnonymous); err != nil {
		t.Fatal(err)
	}
	if err := limiter.AllowPreAuth(t.Context(), key, RequestClassAnonymous); err == nil {
		t.Fatal("bucket should be empty")
	}
	now = now.Add(time.Second)
	if err := limiter.AllowPreAuth(t.Context(), key, RequestClassAnonymous); err != nil {
		t.Fatalf("bucket did not refill: %v", err)
	}
}

func TestLocalLimiterBoundsAttackerControlledBucketKeys(t *testing.T) {
	config := DefaultLimiterConfig()
	config.MaxBuckets = 2
	config.BucketIdleTTL = time.Hour
	limiter := NewLocalLimiter(config)
	for _, key := range []string{"one", "two"} {
		if err := limiter.AllowPreAuth(t.Context(), ClientKey{Value: key}, RequestClassAnonymous); err != nil {
			t.Fatal(err)
		}
	}
	if err := limiter.AllowPreAuth(t.Context(), ClientKey{Value: "three"}, RequestClassAnonymous); err == nil {
		t.Fatal("new attacker-controlled key must be rejected at the bounded capacity")
	}
	if len(limiter.buckets) != 2 {
		t.Fatalf("bucket map exceeded its bound: %d", len(limiter.buckets))
	}
}
