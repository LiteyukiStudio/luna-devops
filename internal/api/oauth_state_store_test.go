package api

import "testing"

func TestNewOAuthStateStoreUsesRedisOnly(t *testing.T) {
	store := newOAuthStateStore("localhost:6379")
	if _, ok := store.(*redisOAuthStateStore); !ok {
		t.Fatalf("expected redis oauth state store, got %T", store)
	}
}
