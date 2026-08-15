package database

import (
	"errors"
	"fmt"

	sqlmigrations "github.com/LiteyukiStudio/devops/migrations"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"
)

var errUnversionedNonEmptySchema = errors.New("database schema is non-empty but has no migration history")

func runSQLMigrations(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("open sql database for migrations: %w", err)
	}
	if err := ensureVersionedOrEmptySchema(db); err != nil {
		return err
	}
	sourceDriver, err := iofs.New(sqlmigrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	databaseDriver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("open postgres migration driver: %w", err)
	}
	runner, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", databaseDriver)
	if err != nil {
		return fmt.Errorf("create migration runner: %w", err)
	}

	if err := runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run sql migrations: %w", err)
	}
	return nil
}

func ensureVersionedOrEmptySchema(db *gorm.DB) error {
	var state struct {
		HasMigrationTable bool
		HasTables         bool
	}
	if err := db.Raw(`SELECT
  EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = current_schema()
      AND table_name = 'schema_migrations'
      AND table_type = 'BASE TABLE'
  ) AS has_migration_table,
  EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = current_schema()
      AND table_type = 'BASE TABLE'
  ) AS has_tables`).Scan(&state).Error; err != nil {
		return fmt.Errorf("inspect migration history: %w", err)
	}
	if !state.HasMigrationTable && state.HasTables {
		return errUnversionedNonEmptySchema
	}
	return nil
}
