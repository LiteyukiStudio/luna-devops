package projectapi

import (
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestProjectOverviewCountsExcludeDeletingApplicationsAndOldFailures(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "project_overview_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Application{}, &model.Release{})
		},
	})
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	at := func(offset time.Duration) *time.Time {
		value := now.Add(offset)
		return &value
	}
	applications := []model.Application{
		{ID: "app_active", ProjectID: "prj", Name: "Active", Identifier: "active", DeleteStatus: "active"},
		{ID: "app_delete_failed", ProjectID: "prj", Name: "Delete failed", Identifier: "delete-failed", DeleteStatus: "delete_failed"},
		{ID: "app_deleting", ProjectID: "prj", Name: "Deleting", Identifier: "deleting", DeleteStatus: "deleting"},
		{ID: "app_deleted", ProjectID: "prj", Name: "Deleted", Identifier: "deleted", DeleteStatus: "deleted"},
		{ID: "app_other", ProjectID: "other", Name: "Other", Identifier: "other", DeleteStatus: "active"},
	}
	if err := db.Create(&applications).Error; err != nil {
		t.Fatalf("create applications: %v", err)
	}
	releases := []model.Release{
		{ID: "rel_pending", ProjectID: "prj", ApplicationID: "app_active", DeploymentTargetID: "t", Status: "pending", Revision: 1, CreatedAt: now},
		{ID: "rel_running", ProjectID: "prj", ApplicationID: "app_active", DeploymentTargetID: "t", Status: "running", Revision: 2, CreatedAt: now},
		{ID: "rel_failed_recent", ProjectID: "prj", ApplicationID: "app_active", DeploymentTargetID: "t", Status: "failed", Revision: 3, CreatedAt: now.Add(-8 * 24 * time.Hour), FinishedAt: at(-time.Hour)},
		{ID: "rel_failed_old", ProjectID: "prj", ApplicationID: "app_active", DeploymentTargetID: "t", Status: "failed", Revision: 4, CreatedAt: now.Add(-9 * 24 * time.Hour), FinishedAt: at(-8 * 24 * time.Hour)},
		{ID: "rel_failed_other", ProjectID: "other", ApplicationID: "app_other", DeploymentTargetID: "t", Status: "failed", Revision: 1, CreatedAt: now, FinishedAt: at(0)},
		{ID: "rel_succeeded", ProjectID: "prj", ApplicationID: "app_active", DeploymentTargetID: "t", Status: "succeeded", Revision: 5, CreatedAt: now, FinishedAt: at(0)},
	}
	if err := db.Create(&releases).Error; err != nil {
		t.Fatalf("create releases: %v", err)
	}

	got, err := projectOverviewCounts(db, "prj", now)
	if err != nil {
		t.Fatalf("projectOverviewCounts() error = %v", err)
	}
	want := projectOverviewResponse{Applications: 2, ActiveReleases: 2, FailedReleases: 1, FailedReleaseWindowDays: 7}
	if got != want {
		t.Fatalf("projectOverviewCounts() = %#v, want %#v", got, want)
	}
}

func TestSummarizeProjectOverviewAccessRoutesReportsUnavailableInsteadOfZero(t *testing.T) {
	observedAt := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	routes := func(statuses ...string) []model.GatewayRoute {
		result := make([]model.GatewayRoute, 0, len(statuses))
		for _, status := range statuses {
			result = append(result, model.GatewayRoute{Status: status})
		}
		return result
	}
	int64Pointer := func(value int64) *int64 { return &value }
	tests := map[string]struct {
		routes []model.GatewayRoute
		want   projectOverviewAccessRoutes
	}{
		"no enabled routes": {
			routes: nil,
			want:   projectOverviewAccessRoutes{Status: "not-configured", Total: 0, ObservationCode: "project_overview.access_routes_not_configured", ObservedAt: observedAt},
		},
		"all ready": {
			routes: routes("ready", "ready"),
			want:   projectOverviewAccessRoutes{Status: "ready", Ready: int64Pointer(2), Total: 2, ObservedAt: observedAt},
		},
		"some not ready": {
			routes: routes("ready", "progressing", "not-found"),
			want:   projectOverviewAccessRoutes{Status: "degraded", Ready: int64Pointer(1), Total: 3, ObservationCode: "project_overview.access_routes_not_ready", ObservedAt: observedAt},
		},
		"one route unobservable": {
			routes: routes("ready", "unavailable"),
			want:   projectOverviewAccessRoutes{Status: "unavailable", Total: 2, ObservationCode: "project_overview.access_routes_unavailable", ObservedAt: observedAt},
		},
	}
	for name, test := range tests {
		got := summarizeProjectOverviewAccessRoutes(test.routes, observedAt)
		if got.Status != test.want.Status || got.Total != test.want.Total || got.ObservationCode != test.want.ObservationCode || !got.ObservedAt.Equal(test.want.ObservedAt) {
			t.Fatalf("%s: summary = %#v, want %#v", name, got, test.want)
		}
		if (got.Ready == nil) != (test.want.Ready == nil) || (got.Ready != nil && *got.Ready != *test.want.Ready) {
			t.Fatalf("%s: ready = %v, want %v", name, got.Ready, test.want.Ready)
		}
	}
}
