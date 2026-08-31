package worker

import (
	"context"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
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
				&model.RuntimeCluster{},
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
	if err := db.Create(&model.RuntimeCluster{
		ID: "rcl_stale", Name: "Stale cluster", Type: "kubernetes", DeleteStatus: "deleting", DeleteStartedAt: &oldStartedAt,
	}).Error; err != nil {
		t.Fatalf("create stale runtime cluster: %v", err)
	}

	payloads, err := (&Runner{db: db}).staleResourceCleanupPayloads(t.Context(), time.Now().Add(-resourceCleanupRecoveryAfter))
	if err != nil {
		t.Fatalf("staleResourceCleanupPayloads() error = %v", err)
	}
	if len(payloads) != 2 || payloads[0].ResourceType != "project" || payloads[0].ResourceID != "prj_stale" || payloads[0].ActorID != "system:cleanup-recovery" || payloads[1].ResourceType != "runtime_cluster" || payloads[1].ResourceID != "rcl_stale" {
		t.Fatalf("recovery payloads = %#v", payloads)
	}
}

func TestResourceCleanupAuditActionUsesStableResourceCategory(t *testing.T) {
	tests := map[string]string{
		"project":           "project.delete",
		"deployment_target": "deployment.delete",
		"gateway_route":     "gateway.delete",
		"runtime_config":    "runtime_config.delete",
		"runtime_cluster":   "runtime_cluster.delete",
		"unknown":           "",
	}
	for resourceType, want := range tests {
		if got := resourceCleanupAuditAction(resourceType); got != want {
			t.Fatalf("resourceCleanupAuditAction(%q) = %q, want %q", resourceType, got, want)
		}
	}
}

func TestResourceCleanupFailureAuditSurvivesCancelledTaskContext(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "resource_cleanup_audit_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.AuditLog{})
		},
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	payload := tasks.ResourceCleanupPayload{
		ResourceType: "deployment_target",
		ResourceID:   "dplt_audit",
		ProjectID:    "prj_audit",
		ActorID:      "usr_audit",
	}
	if err := (&Runner{db: db}).auditResourceCleanupFailure(ctx, payload); err != nil {
		t.Fatalf("auditResourceCleanupFailure() error = %v", err)
	}
	var audit model.AuditLog
	if err := db.First(&audit, "resource = ?", payload.ResourceID).Error; err != nil {
		t.Fatalf("load cleanup failure audit: %v", err)
	}
	if audit.UserID != payload.ActorID || audit.Action != "deployment.delete" || audit.Success || audit.Message != "cleanup_failed" {
		t.Fatalf("cleanup failure audit = %#v", audit)
	}
}

func TestResourceCleanupAttemptWithoutQueueMetadataIsTerminal(t *testing.T) {
	if !resourceCleanupAttemptExhausted(t.Context()) {
		t.Fatal("direct cleanup invocation must fail closed as a terminal attempt")
	}
}
