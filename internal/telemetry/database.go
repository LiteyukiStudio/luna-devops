package telemetry

import (
	"context"
	"database/sql"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func RegisterDBPoolMetrics(db *sql.DB, poolName string) (metric.Registration, error) {
	if db == nil {
		return nil, nil
	}
	meter := otel.Meter("github.com/LiteyukiStudio/devops/internal/telemetry/database")
	connections, err := meter.Int64ObservableGauge("luna_devops_db_client_connections",
		metric.WithDescription("Current database connections by state."))
	if err != nil {
		return nil, err
	}
	maxConnections, err := meter.Int64ObservableGauge("luna_devops_db_client_connections_max",
		metric.WithDescription("Configured maximum database connections."))
	if err != nil {
		return nil, err
	}
	waits, err := meter.Int64ObservableCounter("luna_devops_db_client_connection_wait_total",
		metric.WithDescription("Total database connection pool waits."))
	if err != nil {
		return nil, err
	}
	waitDuration, err := meter.Float64ObservableCounter("luna_devops_db_client_connection_wait_seconds_total",
		metric.WithDescription("Total time waiting for a database connection."), metric.WithUnit("s"))
	if err != nil {
		return nil, err
	}
	closed, err := meter.Int64ObservableCounter("luna_devops_db_client_connections_closed_total",
		metric.WithDescription("Total database connections closed by pool policy."))
	if err != nil {
		return nil, err
	}
	poolAttr := attribute.String("db.client.connection.pool.name", poolName)
	return meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		stats := db.Stats()
		observer.ObserveInt64(connections, int64(stats.OpenConnections), metric.WithAttributes(poolAttr, attribute.String("state", "open")))
		observer.ObserveInt64(connections, int64(stats.InUse), metric.WithAttributes(poolAttr, attribute.String("state", "in_use")))
		observer.ObserveInt64(connections, int64(stats.Idle), metric.WithAttributes(poolAttr, attribute.String("state", "idle")))
		observer.ObserveInt64(maxConnections, int64(stats.MaxOpenConnections), metric.WithAttributes(poolAttr))
		observer.ObserveInt64(waits, stats.WaitCount, metric.WithAttributes(poolAttr))
		observer.ObserveFloat64(waitDuration, stats.WaitDuration.Seconds(), metric.WithAttributes(poolAttr))
		observer.ObserveInt64(closed, stats.MaxIdleClosed, metric.WithAttributes(poolAttr, attribute.String("reason", "max_idle")))
		observer.ObserveInt64(closed, stats.MaxIdleTimeClosed, metric.WithAttributes(poolAttr, attribute.String("reason", "max_idle_time")))
		observer.ObserveInt64(closed, stats.MaxLifetimeClosed, metric.WithAttributes(poolAttr, attribute.String("reason", "max_lifetime")))
		return nil
	}, connections, maxConnections, waits, waitDuration, closed)
}
