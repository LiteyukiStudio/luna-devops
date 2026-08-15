package volumemigration

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGormRepositoryAppliesBackfillIdempotentlyOnPostgres(t *testing.T) {
	db := openVolumeMigrationTestDB(t)
	ctx := context.Background()
	project := model.Project{
		ID: "prj_volume_migration", Identifier: "volume-migration", KubernetesNamespace: "luna-volume-migration",
		Name: "Volume Migration", NamespaceStrategy: "shared",
	}
	cluster := model.RuntimeCluster{
		ID: "rcl_volume_migration", Name: "Volume Migration", Type: "kubernetes", Scope: "global", IsDefault: true,
	}
	application := model.Application{ID: "app_volume_migration", ProjectID: project.ID, Identifier: "database", Name: "Database"}
	target := model.DeploymentTarget{
		ID: "dplt_volume_migration", ProjectID: project.ID, ApplicationID: application.ID, Name: "Production", Stage: "prod",
		KubernetesName: "database-prod", ClusterID: cluster.ID, DataRetentionEnabled: true,
		DataVolumes: `[{"name":"data","mountPath":"/data","sourceType":"managed","capacity":"1Gi"}]`,
	}
	for _, item := range []any{&project, &cluster, &application, &target} {
		if err := db.WithContext(ctx).Create(item).Error; err != nil {
			t.Fatalf("seed %T: %v", item, err)
		}
	}
	inspector := &inspectorStub{
		claims:    map[string]ClaimObservation{"database-prod-data": managedClaim(project.ID, "1Gi")},
		workloads: map[string]WorkloadAttachment{"data": {ClaimName: "database-prod-data", MountPath: "/data"}},
	}
	service := NewService(NewGormRepository(db), inspector)
	first, err := service.Run(ctx, Options{Apply: true, PageSize: 20, ProjectID: project.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Reconciliation.ReadyForSwitch || first.Reconciliation.AppliedProjectVolumes != 1 || first.Reconciliation.AppliedDeploymentMounts != 1 {
		t.Fatalf("first report = %+v", first)
	}
	second, err := service.Run(ctx, Options{Apply: true, PageSize: 20, ProjectID: project.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !second.Reconciliation.ReadyForSwitch || second.Reconciliation.UnchangedProjectVolumes != 1 || second.Reconciliation.UnchangedDeploymentMounts != 1 {
		t.Fatalf("second report = %+v", second)
	}
	var volumeCount, mountCount int64
	if err := db.WithContext(ctx).Model(&model.ProjectVolume{}).Where("project_id = ?", project.ID).Count(&volumeCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Model(&model.DeploymentVolumeMount{}).Where("project_id = ?", project.ID).Count(&mountCount).Error; err != nil {
		t.Fatal(err)
	}
	if volumeCount != 1 || mountCount != 1 {
		t.Fatalf("database volume/mount count = %d/%d", volumeCount, mountCount)
	}
}

func openVolumeMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}
	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	schema := fmt.Sprintf("volume_migration_test_%d", time.Now().UnixNano())
	if err := adminDB.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
		if sqlDB, dbErr := adminDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsedURL.Query()
	query.Set("search_path", schema)
	parsedURL.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration schema: %v", err)
	}
	if err := database.MigrateContext(context.Background(), db); err != nil {
		t.Fatalf("migrate integration schema: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
