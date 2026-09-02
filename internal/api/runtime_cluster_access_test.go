package api

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestRuntimeClusterExplicitZeroPolicyPersistsOnCreate(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "runtime_cluster_zero_policy_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.RuntimeCluster{})
		},
	})
	cluster := model.RuntimeCluster{
		ID: "clu_zero_policy", Name: "Zero Policy", Type: "kubernetes", Scope: "global",
		CPURequestPercent: 0, MemoryRequestPercent: 0, CPULimitPercent: 0, MemoryLimitPercent: 0,
	}
	if err := db.Create(&cluster).Error; err != nil {
		t.Fatalf("create runtime cluster: %v", err)
	}
	var persisted model.RuntimeCluster
	if err := db.First(&persisted, "id = ?", cluster.ID).Error; err != nil {
		t.Fatalf("read runtime cluster: %v", err)
	}
	if persisted.CPURequestPercent != 0 || persisted.MemoryRequestPercent != 0 || persisted.CPULimitPercent != 0 || persisted.MemoryLimitPercent != 0 {
		t.Fatalf("persisted runtime cluster policy = %#v", persisted)
	}
}
