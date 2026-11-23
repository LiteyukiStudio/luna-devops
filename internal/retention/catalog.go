package retention

const (
	DatasetPlatformEvents         = "platform_events"
	DatasetNotificationDeliveries = "notification_deliveries"
	DatasetBuildLogs              = "build_logs"
	DatasetReleaseLogs            = "release_logs"
	DatasetHookRunLogs            = "hook_run_logs"
	DatasetExpiredAuthData        = "expired_auth_data"
	DatasetRuntimeObservations    = "runtime_observations"
	DatasetBillingRecords         = "billing_records"

	MinRetentionDays = 0
	MaxRetentionDays = 3650
	CleanupBatchSize = 1000
	cleanupBatchSize = CleanupBatchSize
)

// Dataset describes one retention-controlled data class.
type Dataset struct {
	Key         string `json:"key"`
	DefaultDays int    `json:"defaultDays"`
	ConfigKey   string `json:"configKey"`
}

var catalog = []Dataset{
	{Key: DatasetPlatformEvents, DefaultDays: 90, ConfigKey: "retention.platformEventsDays"},
	{Key: DatasetNotificationDeliveries, DefaultDays: 90, ConfigKey: "retention.notificationDeliveriesDays"},
	{Key: DatasetBuildLogs, DefaultDays: 30, ConfigKey: "retention.buildLogsDays"},
	{Key: DatasetReleaseLogs, DefaultDays: 90, ConfigKey: "retention.releaseLogsDays"},
	{Key: DatasetHookRunLogs, DefaultDays: 90, ConfigKey: "retention.hookRunLogsDays"},
	{Key: DatasetExpiredAuthData, DefaultDays: 30, ConfigKey: "retention.expiredAuthDataDays"},
	{Key: DatasetRuntimeObservations, DefaultDays: 7, ConfigKey: "retention.runtimeObservationsDays"},
	{Key: DatasetBillingRecords, DefaultDays: 90, ConfigKey: "retention.billingRecordsDays"},
}

// Catalog returns a copy so callers cannot alter the retention whitelist.
func Catalog() []Dataset {
	result := make([]Dataset, len(catalog))
	copy(result, catalog)
	return result
}

// Catalog returns the service's immutable dataset catalog.
func (s *Service) Catalog() []Dataset {
	return Catalog()
}
