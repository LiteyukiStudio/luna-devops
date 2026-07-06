package database

import (
	"strings"
	"testing"
	"time"
)

func TestOpenRejectsUnsupportedDatabaseURLWithoutRetry(t *testing.T) {
	started := time.Now()
	_, err := Open("mysql://user:pass@db:3306/app", Options{
		ConnectRetryAttempts: 3,
		ConnectRetryInterval: time.Second,
	})
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
		MaxOpenConns:         2,
		MaxIdleConns:         8,
		ConnectRetryAttempts: 1,
		ConnectRetryInterval: time.Millisecond,
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
	if options.ConnectRetryAttempts != 1 {
		t.Fatalf("ConnectRetryAttempts = %d", options.ConnectRetryAttempts)
	}
	if options.ConnectRetryInterval != time.Millisecond {
		t.Fatalf("ConnectRetryInterval = %s", options.ConnectRetryInterval)
	}
}

func TestDatabaseOptionsAllowZeroIdleConnections(t *testing.T) {
	options := (Options{
		MaxOpenConns:         4,
		MaxIdleConns:         0,
		ConnectRetryAttempts: 1,
		ConnectRetryInterval: time.Millisecond,
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
	if options.ConnectRetryAttempts != defaultConnectRetryAttempts {
		t.Fatalf("ConnectRetryAttempts = %d", options.ConnectRetryAttempts)
	}
}

func TestCleanupApplicationDeliveryStatementsDropLegacyServicePort(t *testing.T) {
	statements := strings.Join(cleanupApplicationDeliveryStatements(), "\n")
	if !strings.Contains(statements, "applications DROP COLUMN IF EXISTS service_port") {
		t.Fatalf("cleanup statements do not drop legacy application service_port: %s", statements)
	}
}

func TestShouldAdoptLegacyMigrationState(t *testing.T) {
	state := legacyMigrationState{
		HasUsers:                true,
		HasProjects:             true,
		HasDeploymentTargets:    true,
		HasBillingLedgerEntries: true,
	}
	if !shouldAdoptLegacyMigrationState(state) {
		t.Fatalf("expected complete legacy schema to be adopted")
	}
}

func TestShouldNotAdoptWhenMigrationTableExists(t *testing.T) {
	state := legacyMigrationState{
		HasMigrationTable:       true,
		HasUsers:                true,
		HasProjects:             true,
		HasDeploymentTargets:    true,
		HasBillingLedgerEntries: true,
	}
	if shouldAdoptLegacyMigrationState(state) {
		t.Fatalf("expected schema with migration table to keep normal migration flow")
	}
}

func TestShouldNotAdoptIncompleteSchema(t *testing.T) {
	state := legacyMigrationState{
		HasUsers:          true,
		HasProjects:       true,
		HasMigrationTable: false,
	}
	if shouldAdoptLegacyMigrationState(state) {
		t.Fatalf("expected incomplete schema to run migrations from the beginning")
	}
}
