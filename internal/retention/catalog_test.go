package retention

import (
	"reflect"
	"testing"
)

func TestCatalogDefaults(t *testing.T) {
	want := []Dataset{
		{Key: DatasetPlatformEvents, ConfigKey: "retention.platformEventsDays", DefaultDays: 90},
		{Key: DatasetNotificationDeliveries, ConfigKey: "retention.notificationDeliveriesDays", DefaultDays: 90},
		{Key: DatasetWorkerTaskEvents, ConfigKey: "retention.workerTaskEventsDays", DefaultDays: 30},
		{Key: DatasetBuildLogs, ConfigKey: "retention.buildLogsDays", DefaultDays: 30},
		{Key: DatasetReleaseLogs, ConfigKey: "retention.releaseLogsDays", DefaultDays: 90},
		{Key: DatasetHookRunLogs, ConfigKey: "retention.hookRunLogsDays", DefaultDays: 90},
		{Key: DatasetExpiredAuthData, ConfigKey: "retention.expiredAuthDataDays", DefaultDays: 30},
	}

	service := NewService(nil)
	got := service.Catalog()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("catalog = %#v, want %#v", got, want)
	}
	if MinRetentionDays != 0 || MaxRetentionDays != 3650 || CleanupBatchSize != 1000 {
		t.Fatalf("retention limits = %d..%d batch %d", MinRetentionDays, MaxRetentionDays, CleanupBatchSize)
	}

	got[0].DefaultDays = 1
	if service.Catalog()[0].DefaultDays != 90 {
		t.Fatal("Catalog returned mutable package state")
	}
}
