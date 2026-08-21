// Package testdb provides small, deterministic helpers for PostgreSQL integration tests.
package testdb

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	identifierPrefixPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	isolationSequence       atomic.Uint64
)

type Options struct {
	Environment        string
	SchemaPrefix       string
	MaxOpenConnections int
	Migrate            func(*gorm.DB) error
}

// Open creates an isolated schema, opens a GORM connection whose search_path
// points at that schema, and registers connection/schema cleanup with the test.
func Open(t testing.TB, options Options) *gorm.DB {
	t.Helper()

	environment := options.Environment
	if environment == "" {
		environment = "AUTH_TEST_DATABASE_URL"
	}
	databaseURL := os.Getenv(environment)
	if databaseURL == "" {
		t.Skipf("%s is not configured", environment)
	}
	if !identifierPrefixPattern.MatchString(options.SchemaPrefix) {
		t.Fatalf("invalid test schema prefix %q", options.SchemaPrefix)
	}

	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open PostgreSQL test database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := adminDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	schemaName := isolatedName(options.SchemaPrefix)
	if err := adminDB.Exec(`CREATE SCHEMA "` + schemaName + `"`).Error; err != nil {
		t.Fatalf("create PostgreSQL test schema: %v", err)
	}
	t.Cleanup(func() {
		if err := adminDB.Exec(`DROP SCHEMA IF EXISTS "` + schemaName + `" CASCADE`).Error; err != nil {
			t.Errorf("drop PostgreSQL test schema %s: %v", schemaName, err)
		}
	})

	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse PostgreSQL test database URL: %v", err)
	}
	query := parsedURL.Query()
	query.Set("search_path", schemaName)
	parsedURL.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open isolated PostgreSQL test schema: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	if options.MaxOpenConnections > 0 {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			sqlDB.SetMaxOpenConns(options.MaxOpenConnections)
		}
	}
	if options.Migrate != nil {
		if err := options.Migrate(db); err != nil {
			t.Fatalf("prepare isolated PostgreSQL test schema: %v", err)
		}
	}
	return db
}

// OpenDatabase creates a disposable database for tests that reference explicit
// schemas and therefore cannot be isolated with search_path alone.
func OpenDatabase(t testing.TB, options Options) *gorm.DB {
	t.Helper()

	environment := options.Environment
	if environment == "" {
		environment = "AUTH_TEST_DATABASE_URL"
	}
	databaseURL := os.Getenv(environment)
	if databaseURL == "" {
		t.Skipf("%s is not configured", environment)
	}
	if !identifierPrefixPattern.MatchString(options.SchemaPrefix) {
		t.Fatalf("invalid test database prefix %q", options.SchemaPrefix)
	}

	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open PostgreSQL test database: %v", err)
	}
	databaseName := isolatedName(options.SchemaPrefix)
	if err := adminDB.Exec(`CREATE DATABASE "` + databaseName + `"`).Error; err != nil {
		t.Fatalf("create isolated PostgreSQL test database: %v", err)
	}
	t.Cleanup(func() {
		if err := adminDB.Exec(`DROP DATABASE IF EXISTS "` + databaseName + `" WITH (FORCE)`).Error; err != nil {
			t.Errorf("drop PostgreSQL test database %s: %v", databaseName, err)
		}
		if sqlDB, dbErr := adminDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse PostgreSQL test database URL: %v", err)
	}
	parsedURL.Path = "/" + databaseName
	parsedURL.RawPath = ""
	query := parsedURL.Query()
	query.Del("search_path")
	parsedURL.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open isolated PostgreSQL test database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	if options.MaxOpenConnections > 0 {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			sqlDB.SetMaxOpenConns(options.MaxOpenConnections)
		}
	}
	if options.Migrate != nil {
		if err := options.Migrate(db); err != nil {
			t.Fatalf("prepare isolated PostgreSQL test database: %v", err)
		}
	}
	return db
}

func isolatedName(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), isolationSequence.Add(1))
}
