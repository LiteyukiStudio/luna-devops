package worker

import (
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestStaleResourceCleanupPayloadsOnlyRecoverTimedOutDeletingRows(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "resource_cleanup_recovery_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.Project{},
				&model.DeploymentTarget{},
				&model.GatewayRoute{},
				&model.ProjectRuntimeConfigSet{},
			)
		},
	})
	oldStartedAt := time.Now().Add(-2 * resourceCleanupRecoveryAfter)
	recentStartedAt := time.Now()
	rows := []model.Project{
		{ID: "prj_stale", Identifier: "stale-project", Name: "Stale", DeleteStatus: "deleting", DeleteStartedAt: &oldStartedAt},
		{ID: "prj_recent", Identifier: "recent-project", Name: "Recent", DeleteStatus: "deleting", DeleteStartedAt: &recentStartedAt},
		{ID: "prj_failed", Identifier: "failed-project", Name: "Failed", DeleteStatus: "delete_failed", DeleteStartedAt: &oldStartedAt},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("create cleanup fixtures: %v", err)
	}

	payloads, err := (&Runner{db: db}).staleResourceCleanupPayloads(t.Context(), time.Now().Add(-resourceCleanupRecoveryAfter))
	if err != nil {
		t.Fatalf("staleResourceCleanupPayloads() error = %v", err)
	}
	if len(payloads) != 1 || payloads[0].ResourceType != "project" || payloads[0].ResourceID != "prj_stale" {
		t.Fatalf("recovery payloads = %#v", payloads)
	}
}

func TestResourceCleanupAttemptWithoutQueueMetadataIsTerminal(t *testing.T) {
	if !resourceCleanupAttemptExhausted(t.Context()) {
		t.Fatal("direct cleanup invocation must fail closed as a terminal attempt")
	}
}
