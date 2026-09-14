package runtimeapi

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestRuntimeClusterReferenceCountsCoverImplicitTargetsAndProjectVolumes(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "runtime_cluster_delete_refs_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{}, &model.RuntimeCluster{}, &model.DeploymentTarget{}, &model.ProjectVolume{})
		},
	})
	projects := []model.Project{
		{ID: "prj_one", Identifier: "one", Name: "One"},
		{ID: "prj_two", Identifier: "two", Name: "Two"},
		{ID: "prj_three", Identifier: "three", Name: "Three"},
	}
	if err := db.Create(&projects).Error; err != nil {
		t.Fatalf("create projects: %v", err)
	}
	defaultCluster := model.RuntimeCluster{ID: "clu_default", Name: "Default", Scope: "global", IsDefault: true, DeleteStatus: "active"}
	otherCluster := model.RuntimeCluster{ID: "clu_other", Name: "Other", Scope: "global", DeleteStatus: "active"}
	if err := db.Create(&[]model.RuntimeCluster{defaultCluster, otherCluster}).Error; err != nil {
		t.Fatalf("create runtime clusters: %v", err)
	}
	targets := []model.DeploymentTarget{
		{ID: "dplt_implicit", ProjectID: "prj_one", ApplicationID: "app_one", Name: "Implicit", Stage: "prod"},
		{ID: "dplt_default", ProjectID: "prj_two", ApplicationID: "app_two", Name: "Default", Stage: "prod", ClusterID: defaultCluster.ID},
		{ID: "dplt_other", ProjectID: "prj_three", ApplicationID: "app_three", Name: "Other", Stage: "prod", ClusterID: otherCluster.ID},
	}
	if err := db.Create(&targets).Error; err != nil {
		t.Fatalf("create deployment targets: %v", err)
	}
	volumes := []model.ProjectVolume{
		projectVolumeReferenceFixture("pvol_default", "prj_one", defaultCluster.ID),
		projectVolumeReferenceFixture("pvol_other", "prj_two", otherCluster.ID),
	}
	if err := db.Create(&volumes).Error; err != nil {
		t.Fatalf("create project volumes: %v", err)
	}

	targetCount, volumeCount, err := runtimeClusterReferenceCounts(db, defaultCluster)
	if err != nil {
		t.Fatalf("default runtime cluster reference counts: %v", err)
	}
	if targetCount != 2 || volumeCount != 1 {
		t.Fatalf("default cluster references = targets:%d volumes:%d, want 2 and 1", targetCount, volumeCount)
	}
	targetCount, volumeCount, err = runtimeClusterReferenceCounts(db, otherCluster)
	if err != nil {
		t.Fatalf("other runtime cluster reference counts: %v", err)
	}
	if targetCount != 1 || volumeCount != 1 {
		t.Fatalf("other cluster references = targets:%d volumes:%d, want 1 and 1", targetCount, volumeCount)
	}
}

func projectVolumeReferenceFixture(id, projectID, clusterID string) model.ProjectVolume {
	return model.ProjectVolume{
		ID: id, ProjectID: projectID, DisplayName: id, ClusterID: clusterID,
		Namespace: "ns-" + projectID, ClaimName: id, OwnershipMode: model.ProjectVolumeOwnershipManaged,
		SourceKind: model.ProjectVolumeSourceBlank, LifecycleState: model.ProjectVolumeLifecycleReady,
		CapacityRequest: "1Gi", CapacityBytes: 1024 * 1024 * 1024,
		AccessMode: model.ProjectVolumeAccessReadWriteOnce, VolumeMode: model.ProjectVolumeModeFilesystem,
		CreatedBy: "usr_test",
	}
}
