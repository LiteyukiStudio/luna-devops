package database

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestOpenRejectsUnsupportedDatabaseURLWithoutRetry(t *testing.T) {
	started := time.Now()
	_, err := OpenContext(context.Background(), "mysql://user:pass@db:3306/app")
	if err == nil {
		t.Fatalf("expected unsupported database URL error")
	}
	if !strings.Contains(err.Error(), "unsupported database url") {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("unsupported database URL took %s, expected no retry sleep", elapsed)
	}
}

func TestDatabaseOptionsDefaultsAndClamp(t *testing.T) {
	options := (Options{
		MaxOpenConns: 2,
		MaxIdleConns: 8,
	}).withDefaults()

	if options.MaxOpenConns != 2 {
		t.Fatalf("MaxOpenConns = %d", options.MaxOpenConns)
	}
	if options.MaxIdleConns != 2 {
		t.Fatalf("MaxIdleConns = %d", options.MaxIdleConns)
	}
	if options.ConnMaxLifetime != defaultConnMaxLifetime {
		t.Fatalf("ConnMaxLifetime = %s", options.ConnMaxLifetime)
	}
	if options.ConnMaxIdleTime != defaultConnMaxIdleTime {
		t.Fatalf("ConnMaxIdleTime = %s", options.ConnMaxIdleTime)
	}
}

func TestDatabaseOptionsAllowZeroIdleConnections(t *testing.T) {
	options := (Options{
		MaxOpenConns: 4,
		MaxIdleConns: 0,
	}).withDefaults()

	if options.MaxIdleConns != 0 {
		t.Fatalf("MaxIdleConns = %d, want 0", options.MaxIdleConns)
	}
}

func TestDefaultDatabaseOptions(t *testing.T) {
	options := defaultOptions()
	if options.MaxOpenConns != defaultMaxOpenConns {
		t.Fatalf("MaxOpenConns = %d", options.MaxOpenConns)
	}
	if options.MaxIdleConns != defaultMaxIdleConns {
		t.Fatalf("MaxIdleConns = %d", options.MaxIdleConns)
	}
}
