package redisconfig

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestCheckConnectionPingsRedis(t *testing.T) {
	server := miniredis.RunT(t)
	options, err := Parse("redis://" + server.Addr() + "/0")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if err := CheckConnection(context.Background(), options); err != nil {
		t.Fatalf("CheckConnection() error = %v", err)
	}
}

func TestCheckConnectionRejectsMissingAddress(t *testing.T) {
	err := CheckConnection(context.Background(), Options{})
	if err == nil || !strings.Contains(err.Error(), "REDIS_ADDR is required") {
		t.Fatalf("CheckConnection() error = %v", err)
	}
}

func TestCheckConnectionRejectsUnavailableRedis(t *testing.T) {
	server := miniredis.RunT(t)
	options, err := Parse("redis://" + server.Addr() + "/0")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := CheckConnection(ctx, options); err == nil {
		t.Fatal("CheckConnection() succeeded for unavailable Redis")
	}
}
