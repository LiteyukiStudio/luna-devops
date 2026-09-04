package volume

import (
	"context"
	"fmt"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func validBlankVolumeInput() CreateProjectVolumeInput {
	return CreateProjectVolumeInput{
		ProjectID: "prj_demo", DisplayName: "postgres-data", ClusterID: "rclu_demo", Namespace: "project-demo",
		OwnershipMode: model.ProjectVolumeOwnershipManaged, SourceKind: model.ProjectVolumeSourceBlank,
		CapacityRequest: "10Gi", CapacityBytes: 10 * 1024 * 1024 * 1024, StorageClassName: "standard",
		AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		ActorID: "usr_demo", IdempotencyKey: "volume-create-demo-0001",
	}
}

func postgresTestProjectVolume(index int) model.ProjectVolume {
	return model.ProjectVolume{
		ID: fmt.Sprintf("pvol_%03d", index), ProjectID: "prj_volume_test", DisplayName: fmt.Sprintf("volume-%03d", index),
		ClusterID: "rclu_volume_test", Namespace: "project-volume-test", ClaimName: fmt.Sprintf("claim-%03d", index),
		OwnershipMode: model.ProjectVolumeOwnershipManaged, SourceKind: model.ProjectVolumeSourceBlank,
		LifecycleState: model.ProjectVolumeLifecycleReady, CapacityRequest: "1Gi", CapacityBytes: 1024 * 1024 * 1024,
		StorageClassName: "standard", AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		CreatedBy: "usr_volume_test", Revision: 1,
	}
}

func installProjectVolumeTestSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := database.MigrateContext(context.Background(), db); err != nil {
		t.Fatalf("install project volume schema: %v", err)
	}
}

func openVolumeTestDB(t *testing.T) *gorm.DB {
	return testdb.OpenDatabase(t, testdb.Options{SchemaPrefix: "volume_test"})
}
