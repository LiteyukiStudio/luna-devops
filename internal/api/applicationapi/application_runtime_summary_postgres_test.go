package applicationapi

import (
	"context"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

type summaryTestHost struct {
	Host
	db *gorm.DB
}

func (host summaryTestHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.db.WithContext(ctx)
}

func TestDeploymentTargetCurrentReleasesUsesFinishedAtPerExistingTarget(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "application_current_release_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Release{})
		},
	})
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	at := func(offset time.Duration) *time.Time {
		value := base.Add(offset)
		return &value
	}
	releases := []model.Release{
		{ID: "rel_prod_created_late", ProjectID: "prj", ApplicationID: "app", DeploymentTargetID: "prod", ImageRef: "repo/app:v1", Status: "succeeded", Revision: 2, CreatedAt: base.Add(3 * time.Hour), FinishedAt: at(time.Hour)},
		{ID: "rel_prod_finished_late", ProjectID: "prj", ApplicationID: "app", DeploymentTargetID: "prod", ImageRef: "repo/app:v2", Status: "succeeded", Revision: 1, CreatedAt: base, FinishedAt: at(2 * time.Hour)},
		{ID: "rel_prod_failed", ProjectID: "prj", ApplicationID: "app", DeploymentTargetID: "prod", ImageRef: "repo/app:v3", Status: "failed", Revision: 3, CreatedAt: base.Add(4 * time.Hour), FinishedAt: at(4 * time.Hour)},
		{ID: "rel_dev", ProjectID: "prj", ApplicationID: "app", DeploymentTargetID: "dev", ImageRef: "repo/app:dev", Status: "succeeded", Revision: 1, CreatedAt: base, FinishedAt: at(30 * time.Minute)},
		{ID: "rel_deleted_target", ProjectID: "prj", ApplicationID: "app", DeploymentTargetID: "deleted", ImageRef: "repo/app:gone", Status: "succeeded", Revision: 1, CreatedAt: base, FinishedAt: at(5 * time.Hour)},
		{ID: "rel_other_project", ProjectID: "other", ApplicationID: "app", DeploymentTargetID: "prod", ImageRef: "repo/app:other", Status: "succeeded", Revision: 1, CreatedAt: base, FinishedAt: at(6 * time.Hour)},
	}
	if err := db.Create(&releases).Error; err != nil {
		t.Fatalf("create releases: %v", err)
	}
	handler := &Handler{host: summaryTestHost{db: db}}
	targets := []model.DeploymentTarget{{ID: "prod", ApplicationID: "app"}, {ID: "dev", ApplicationID: "app"}, {ID: "idle", ApplicationID: "app"}}

	got, err := handler.deploymentTargetCurrentReleases(context.Background(), model.Project{ID: "prj"}, targets)
	if err != nil {
		t.Fatalf("deploymentTargetCurrentReleases() error = %v", err)
	}
	if len(got) != 2 || got["prod"] == nil || got["prod"].ID != "rel_prod_finished_late" || got["dev"] == nil || got["dev"].ID != "rel_dev" {
		t.Fatalf("deploymentTargetCurrentReleases() = %#v, want prod=rel_prod_finished_late dev=rel_dev", got)
	}
	if current := applicationCurrentReleaseOf(targets, got); current == nil || current.ID != "rel_prod_finished_late" {
		t.Fatalf("applicationCurrentReleaseOf() = %#v, want rel_prod_finished_late", current)
	}
}
