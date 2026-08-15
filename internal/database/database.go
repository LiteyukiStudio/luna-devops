package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
)

const (
	defaultMaxOpenConns       = 20
	defaultMaxIdleConns       = 5
	defaultConnMaxLifetime    = 30 * time.Minute
	defaultConnMaxIdleTime    = 5 * time.Minute
	defaultConnectPingTimeout = 5 * time.Second
)

type Options struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func (options Options) withDefaults() Options {
	if options.MaxOpenConns <= 0 {
		options.MaxOpenConns = defaultMaxOpenConns
	}
	if options.MaxIdleConns < 0 {
		options.MaxIdleConns = 0
	}
	if options.MaxIdleConns > options.MaxOpenConns {
		options.MaxIdleConns = options.MaxOpenConns
	}
	if options.ConnMaxLifetime <= 0 {
		options.ConnMaxLifetime = defaultConnMaxLifetime
	}
	if options.ConnMaxIdleTime <= 0 {
		options.ConnMaxIdleTime = defaultConnMaxIdleTime
	}
	return options
}

func OpenContext(ctx context.Context, databaseURL string, optionList ...Options) (*gorm.DB, error) {
	if !isPostgresURL(databaseURL) {
		return nil, fmt.Errorf("unsupported database url: %s", databaseURL)
	}

	options := defaultOptions()
	if len(optionList) > 0 {
		options = optionList[0].withDefaults()
	}

	db, err := openPostgres(ctx, databaseURL, options)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	return db, nil
}

func defaultOptions() Options {
	return Options{
		MaxOpenConns:    defaultMaxOpenConns,
		MaxIdleConns:    defaultMaxIdleConns,
		ConnMaxLifetime: defaultConnMaxLifetime,
		ConnMaxIdleTime: defaultConnMaxIdleTime,
	}
}

func isPostgresURL(databaseURL string) bool {
	return strings.HasPrefix(databaseURL, "postgres://") || strings.HasPrefix(databaseURL, "postgresql://")
}

func openPostgres(ctx context.Context, databaseURL string, options Options) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		DisableAutomaticPing: true,
		// SQL results are represented by OTel spans and structured boundary
		// logs. GORM's text logger can interpolate secrets into raw SQL.
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}
	if err := db.Use(tracing.NewPlugin(
		tracing.WithDBSystem("postgresql"),
		tracing.WithoutQueryVariables(),
		tracing.WithoutServerAddress(),
	)); err != nil {
		return nil, fmt.Errorf("instrument database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(options.MaxOpenConns)
	sqlDB.SetMaxIdleConns(options.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(options.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(options.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(ctx, defaultConnectPingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	return db, nil
}

func MigrateContext(ctx context.Context, db *gorm.DB) (err error) {
	ctx, end := telemetry.StartOperation(ctx, "database", "migrate")
	defer func() { end(err) }()
	db = db.WithContext(ctx)
	return runSQLMigrations(db)
}
