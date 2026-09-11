package deploymentapi

import (
	"context"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

type deploymentTargetPersistenceTestHost struct {
	Host
	db *gorm.DB
}

func (h deploymentTargetPersistenceTestHost) DBWithContext(ctx context.Context) *gorm.DB {
	return h.db.WithContext(ctx)
}

func TestCreateDeploymentTargetPreservesExplicitZeroReplicas(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "deployment_target_zero_replicas",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.DeploymentTarget{},
				&model.DeploymentVolumeMount{},
				&model.DeploymentTargetHookBinding{},
			)
		},
	})
	handler := &Handler{host: deploymentTargetPersistenceTestHost{db: db}}
	target := model.DeploymentTarget{
		ID:                "dplt_zero",
		ProjectID:         "prj_zero",
		ApplicationID:     "app_zero",
		Name:              "Stopped",
		Stage:             "dev",
		KubernetesName:    "stopped-dev",
		Replicas:          0,
		ServicePorts:      "[]",
		RuntimeConfigRefs: "[]",
	}

	if _, err := handler.createDeploymentTarget(target, nil, nil, nil, t.Context()); err != nil {
		t.Fatalf("createDeploymentTarget returned error: %v", err)
	}
	var stored model.DeploymentTarget
	if err := db.First(&stored, "id = ?", target.ID).Error; err != nil {
		t.Fatalf("load deployment target: %v", err)
	}
	if stored.Replicas != 0 {
		t.Fatalf("stored replicas = %d, want 0", stored.Replicas)
	}
}
